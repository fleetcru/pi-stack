package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

type RPCCommand map[string]any
type RPCEvent map[string]any

var errExtensionUIRequestMismatch = errors.New("extension UI request mismatch")

type EventRecord struct {
	ID        uint64    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Event     RPCEvent  `json:"event"`
	size      int
}

type responseWaiter struct {
	command string
	ch      chan RPCEvent
}

type PiProcess struct {
	id     string
	cfg    Config
	spec   SessionSpec
	logger *slog.Logger

	mu      sync.RWMutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	running bool
	closed  bool
	done    chan struct{}
	readers sync.WaitGroup

	seq              uint64
	waiters          map[string]responseWaiter
	subs             map[chan RPCEvent]struct{}
	events           []EventRecord
	eventMax         int
	eventMaxBytes    int
	eventBytes       int
	journal          *eventJournal
	eventSeq         uint64
	restarts         int
	runtimeState     string
	runtimeReason    string
	runtimeDetail    string
	runtimeSince     time.Time
	runtimeError     string
	pendingUIRequest RPCEvent
	lastEventAt      time.Time
	droppedEvents    uint64
	// taskID is stable for the session; runID changes for each agent turn.
	taskID string
	runID  string
	// onMessageEnd is called when a message_end event is dispatched.
	// Used to invalidate the history cache so REST requests return fresh data.
	onMessageEnd func()
	// onAgentSettled releases scheduler admission exactly once after a run.
	onAgentSettled func()
	admissionHeld  bool
}

func NewPiProcess(spec SessionSpec, cfg Config, logger *slog.Logger) *PiProcess {
	eventMax := cfg.EventHistoryMax
	if eventMax <= 0 {
		eventMax = 200
	}
	eventMaxBytes := cfg.EventHistoryBytes
	if eventMaxBytes <= 0 {
		eventMaxBytes = 8 << 20
	}
	journal, restored, lastID, err := openEventJournal(cfg.DataDir, spec.ID, cfg.EventJournalSyncInterval)
	if err != nil {
		logger.Warn("durable event journal unavailable", "session", spec.ID, "error", err)
	}
	p := &PiProcess{id: spec.ID, cfg: cfg, spec: spec, logger: logger.With("session", spec.ID, "cwd", spec.CWD), waiters: map[string]responseWaiter{}, subs: map[chan RPCEvent]struct{}{}, eventMax: eventMax, eventMaxBytes: eventMaxBytes, journal: journal, events: restored, eventSeq: lastID, runtimeState: "created", taskID: spec.ID, runID: newRequestID()}
	for _, record := range p.events {
		p.eventBytes += record.size
	}
	trimmed := false
	for len(p.events) > p.eventMax || p.eventBytes > p.eventMaxBytes {
		p.eventBytes -= p.events[0].size
		p.events = p.events[1:]
		trimmed = true
	}
	if trimmed {
		if err := p.journal.compact(p.events); err != nil {
			p.logger.Warn("failed to compact restored event journal", "error", err)
		}
	}
	return p
}

func (p *PiProcess) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return nil
	}
	if p.closed {
		p.mu.Unlock()
		return errors.New("session closed")
	}
	p.setRuntimeLocked("starting", "process", "Starting Pi")
	startingEvent := p.runtimeStateEventLocked()
	args := append([]string{"--mode", "rpc"}, p.spec.Args...)
	for _, extension := range p.cfg.Extensions {
		if extension != "" {
			args = append(args, "--extension", extension)
		}
	}
	cmd := exec.CommandContext(context.Background(), p.cfg.PiBinary, args...)
	cmd.Dir = p.spec.CWD
	if len(p.spec.Env) > 0 {
		cmd.Env = append(cmd.Environ(), envMapToList(p.spec.Env)...)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		p.mu.Unlock()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		p.mu.Unlock()
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		p.mu.Unlock()
		return err
	}
	applyProcessAttrs(cmd)
	if err := ctx.Err(); err != nil {
		p.mu.Unlock()
		return err
	}
	if err := cmd.Start(); err != nil {
		p.mu.Unlock()
		return err
	}
	p.cmd, p.stdin, p.running, p.done = cmd, stdin, true, make(chan struct{})
	p.setRuntimeLocked("idle", "process", "Ready")
	idleEvent := p.runtimeStateEventLocked()
	p.readers.Add(2)
	go func() { defer p.readers.Done(); p.readStdout(stdout) }()
	go func() { defer p.readers.Done(); p.readStderr(stderr) }()
	p.logger.Info("pi rpc process started", "pid", cmd.Process.Pid, "args", args)
	p.mu.Unlock()
	p.dispatch(startingEvent)
	p.dispatch(idleEvent)
	go p.wait(cmd)
	return nil
}

func (p *PiProcess) Request(ctx context.Context, command RPCCommand) (RPCEvent, error) {
	if err := p.Start(ctx); err != nil {
		return nil, err
	}
	id := fmt.Sprintf("%s-%d", p.id, atomic.AddUint64(&p.seq, 1))
	command["id"] = id
	b, err := json.Marshal(command)
	if err != nil {
		return nil, err
	}
	waiter := responseWaiter{command: fmt.Sprint(command["type"]), ch: make(chan RPCEvent, 1)}
	p.mu.Lock()
	if p.closed || !p.running || p.stdin == nil {
		p.mu.Unlock()
		return nil, errors.New("pi process not running")
	}
	p.waiters[id] = waiter
	_, err = p.stdin.Write(append(b, '\n'))
	p.mu.Unlock()
	if err != nil {
		p.removeWaiter(id)
		return nil, err
	}
	select {
	case resp := <-waiter.ch:
		if ok, _ := resp["success"].(bool); !ok {
			return resp, fmt.Errorf("rpc command failed: %v", resp["error"])
		}
		return resp, nil
	case <-ctx.Done():
		p.removeWaiter(id)
		return nil, ctx.Err()
	}
}

func (p *PiProcess) Send(command RPCCommand) error {
	if command["type"] == "extension_ui_response" {
		if err := validateExtensionUIResponseCommand(command); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.RequestTimeout)
	defer cancel()
	if err := p.Start(ctx); err != nil {
		return err
	}
	b, err := json.Marshal(command)
	if err != nil {
		return err
	}
	p.mu.Lock()
	if p.closed || p.stdin == nil {
		p.mu.Unlock()
		return errors.New("pi process closed")
	}
	requestID, _ := command["id"].(string)
	isUIResponse := command["type"] == "extension_ui_response"
	if isUIResponse {
		pendingID, _ := p.pendingUIRequest["id"].(string)
		if requestID == "" {
			p.mu.Unlock()
			return fmt.Errorf("%w: id is required", errExtensionUIRequestMismatch)
		}
		if pendingID == "" || requestID != pendingID {
			p.mu.Unlock()
			return fmt.Errorf("%w: request %q is no longer pending", errExtensionUIRequestMismatch, requestID)
		}
	}
	_, err = p.stdin.Write(append(b, '\n'))
	if err == nil && isUIResponse {
		p.pendingUIRequest = nil
	}
	p.mu.Unlock()
	if err == nil && isUIResponse {
		// Pi's RPC protocol does not emit a close event after a response. Emit a
		// daemon-side close so every connected client dismisses the same dialog.
		p.dispatch(RPCEvent{"type": "extension_ui_closed", "id": requestID})
	}
	return err
}

func (p *PiProcess) SubscribeSince(since uint64) (<-chan RPCEvent, []EventRecord, func()) {
	// Streaming responses can emit many small text_delta events per second.
	// Keep enough headroom for brief client/UI stalls without dropping a
	// character-sized event immediately.
	ch := make(chan RPCEvent, 2048)
	p.mu.Lock()
	p.subs[ch] = struct{}{}
	replay := make([]EventRecord, 0)
	if since > 0 && len(p.events) > 0 && p.events[0].ID > since+1 {
		// The requested cursor predates the bounded RPC ring. ID 0 denotes an
		// ID-less control frame; ws_handler sends it without re-stamping.
		replay = append(replay, EventRecord{Event: RPCEvent{"type": "events_lost", "expectedAfter": since, "received": p.events[0].ID}})
	}
	for _, record := range p.events {
		if record.ID > since {
			replay = append(replay, record)
		}
	}
	p.mu.Unlock()
	return ch, replay, func() {
		p.mu.Lock()
		if _, ok := p.subs[ch]; ok {
			delete(p.subs, ch)
			close(ch)
		}
		p.mu.Unlock()
	}
}

// EventMetrics reports bounded replay pressure without exposing event content.
func (p *PiProcess) EventMetrics() (retained int, retainedBytes int, dropped uint64) {
	p.mu.RLock()
	retained, retainedBytes = len(p.events), p.eventBytes
	p.mu.RUnlock()
	return retained, retainedBytes, atomic.LoadUint64(&p.droppedEvents)
}

func (p *PiProcess) Subscribe() (<-chan RPCEvent, func()) {
	ch, _, close := p.SubscribeSince(0)
	return ch, close
}

func (p *PiProcess) Close(ctx context.Context) error {
	defer p.releaseAdmission()
	defer func() {
		p.readers.Wait()
		p.mu.Lock()
		journal := p.journal
		p.journal = nil
		p.mu.Unlock()
		_ = journal.close()
	}()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	cmd := p.cmd
	done := p.done
	stdin := p.stdin
	// Send abort and close stdin while holding the lock to prevent racing
	// with Request() or Send() which also write to stdin under the same lock.
	if stdin != nil && cmd != nil && cmd.Process != nil {
		b, _ := json.Marshal(RPCCommand{"type": "abort"})
		_, _ = stdin.Write(append(b, '\n'))
		_ = stdin.Close()
	}
	p.stdin = nil
	p.mu.Unlock()
	if cmd == nil || cmd.Process == nil || done == nil {
		return nil
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return ctx.Err()
	}
	// The deferred reader wait ensures cmd.Wait/process-exit paths also finish
	// unwinding before the durable journal is closed.
	return nil
}

// Pi RPC is strict LF-delimited JSONL. Do not use bufio.Scanner here: its
// token limit can terminate the complete event stream when Pi emits large but
// valid tool output. ReadSlice keeps LF framing exact while retaining a hard
// server-side record cap.
const maxRPCJSONLRecordBytes = 16 << 20 // 16 MiB

func (p *PiProcess) readStdout(r io.Reader) {
	reader := bufio.NewReaderSize(r, 64*1024)
	var record []byte
	discarding := false
	for {
		fragment, err := reader.ReadSlice('\n')
		if !discarding {
			if len(record)+len(fragment) > maxRPCJSONLRecordBytes {
				discarding = true
				record = nil
			} else {
				record = append(record, fragment...)
			}
		}

		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			p.logger.Warn("rpc stdout read error", "error", err)
			return
		}
		if discarding {
			p.logger.Warn("discarded oversized rpc jsonl record", "limitBytes", maxRPCJSONLRecordBytes)
			discarding = false
			record = nil
			// Emit a sentinel so the event-ID sequence stays unbroken. Without
			// this, the gap triggers an events_lost reconnect loop in clients.
			p.dispatch(RPCEvent{"type": "daemon_warning", "warning": "discarded oversized rpc record", "limitBytes": maxRPCJSONLRecordBytes})
			if errors.Is(err, io.EOF) {
				return
			}
			continue
		}
		if len(record) > 0 {
			// ReadSlice retains the LF delimiter; tolerate CRLF input only by
			// removing the CR immediately before its required LF.
			if record[len(record)-1] == '\n' {
				record = record[:len(record)-1]
			}
			if len(record) > 0 && record[len(record)-1] == '\r' {
				record = record[:len(record)-1]
			}
			var ev RPCEvent
			if decodeErr := json.Unmarshal(record, &ev); decodeErr != nil {
				p.logger.Warn("invalid rpc json", "bytes", len(record), "error", decodeErr)
			} else {
				p.dispatch(ev)
			}
		}
		record = nil
		if errors.Is(err, io.EOF) {
			return
		}
	}
}

func (p *PiProcess) readStderr(r io.Reader) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for s.Scan() {
		p.logger.Warn("pi stderr", "line", s.Text())
	}
	if err := s.Err(); err != nil {
		p.logger.Warn("pi stderr scanner error", "error", err)
	}
}

func (p *PiProcess) wait(cmd *exec.Cmd) {
	err := cmd.Wait()
	p.mu.Lock()
	var stateEvent RPCEvent
	if p.cmd == cmd {
		p.running = false
		if p.spec.Restart && !p.closed && p.restarts < p.cfg.RestartMax {
			p.setRuntimeLocked("reconnecting", "process", "Restarting Pi")
			stateEvent = p.runtimeStateEventLocked()
		} else {
			p.setRuntimeLocked("stopped", "process", "Pi process stopped")
			stateEvent = p.runtimeStateEventLocked()
		}
		p.stdin = nil
		p.cmd = nil
		if p.done != nil {
			close(p.done)
			p.done = nil
		}
	}
	for id, w := range p.waiters {
		delete(p.waiters, id)
		w.ch <- RPCEvent{"type": "response", "success": false, "error": "pi process exited"}
	}
	restart := p.spec.Restart && !p.closed && p.restarts < p.cfg.RestartMax
	if restart {
		p.restarts++
	}
	attempt := p.restarts
	p.mu.Unlock()
	if stateEvent != nil {
		p.dispatch(stateEvent)
	}
	// A process exit terminates the admitted run even if Pi could not emit its
	// normal agent_end/agent_settled event.
	p.releaseAdmission()
	p.logger.Info("pi rpc process exited", "error", err, "restartAttempt", attempt)
	if restart {
		delay := p.cfg.RestartBackoff * time.Duration(1<<min(attempt-1, 5))
		time.Sleep(delay)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if startErr := p.Start(ctx); startErr != nil {
			p.logger.Warn("pi rpc restart failed", "attempt", attempt, "error", startErr)
		}
		cancel()
	}
}

func (p *PiProcess) dispatch(ev RPCEvent) {
	// Close() marks the process closed before stopping auxiliary producers such
	// as filesystem watchers. Ignore late events rather than touching a closed
	// journal or publishing to closed subscriber channels.
	p.mu.RLock()
	closed := p.closed
	p.mu.RUnlock()
	if closed {
		return
	}
	// Pi uses extension_ui_request for both blocking dialog methods and
	// fire-and-forget status/notification events. Preserve the raw protocol
	// fields and add a daemon classification so clients never mistake verbose
	// extension output for a request that must block the workspace.
	if ev["type"] == "extension_ui_request" {
		ev["_daemonExtensionUiRequiresResponse"] = extensionUIRequiresResponse(ev)
	}
	if ev["type"] == "response" {
		if id, ok := ev["id"].(string); ok {
			p.mu.Lock()
			w, found := p.waiters[id]
			if found {
				delete(p.waiters, id)
			}
			p.mu.Unlock()
			if found {
				w.ch <- ev
			}
		}
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	if ev["type"] == "extension_ui_request" && extensionUIRequiresResponse(ev) {
		p.pendingUIRequest = cloneEvent(ev)
	}
	if ev["type"] == "extension_ui_closed" {
		requestID, _ := ev["id"].(string)
		pendingID, _ := p.pendingUIRequest["id"].(string)
		matched := requestID != "" && (pendingID == "" || requestID == pendingID)
		ev["_daemonExtensionUiCloseMatched"] = matched
		if matched {
			p.pendingUIRequest = nil
		}
	}
	if ev["type"] == "agent_start" {
		p.runID = newRequestID()
	}
	oldState := p.runtimeState
	oldReason := p.runtimeReason
	p.updateRuntimeFromEventLocked(ev)
	stateChanged := p.runtimeState != oldState || p.runtimeReason != oldReason
	p.eventSeq++
	id := p.eventSeq
	p.lastEventAt = time.Now().UTC()
	// Retaining full Pi events is optional replay convenience, so account for
	// serialized payload bytes and keep the per-session history on a hard budget.
	if encoded, err := json.Marshal(ev); err != nil {
		p.logger.Warn("not retaining event that cannot be encoded", "error", err)
	} else if len(encoded) <= p.eventMaxBytes {
		record := EventRecord{ID: id, Timestamp: time.Now().UTC(), Event: cloneEvent(ev), size: len(encoded)}
		p.events = append(p.events, record)
		p.eventBytes += record.size
		if p.journal != nil {
			if err := p.journal.append(record); err != nil {
				p.logger.Warn("failed to persist daemon event", "error", err)
			}
		}
		for len(p.events) > p.eventMax || p.eventBytes > p.eventMaxBytes {
			p.eventBytes -= p.events[0].size
			p.events = p.events[1:]
		}
		if p.journal != nil && p.journal.shouldCompact(p.eventMax, p.eventMaxBytes) {
			if err := p.journal.compact(p.events); err != nil {
				p.logger.Warn("failed to compact event journal", "error", err)
			}
		}
	} else {
		p.logger.Warn("not retaining oversized event", "bytes", len(encoded), "limit", p.eventMaxBytes)
	}
	out := eventWithID(ev, id)
	out["_daemonTaskId"] = p.taskID
	out["_daemonRunId"] = p.runID
	// Copy subscriber set under the write lock to prevent a subscriber from
	// being added between the write unlock and read lock, which could cause
	// it to miss the event in its replay window.
	subs := make([]chan RPCEvent, 0, len(p.subs))
	for ch := range p.subs {
		subs = append(subs, ch)
	}
	p.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- out:
		default:
			atomic.AddUint64(&p.droppedEvents, 1)
			p.logger.Warn("dropping event for slow subscriber")
		}
	}
	// Emit a synthetic runtime_state event when the process state transitions.
	// This lets clients derive live status from WS events without HTTP polling.
	if stateChanged {
		p.mu.RLock()
		rtState := p.runtimeState
		rtReason := p.runtimeReason
		rtDetail := p.runtimeDetail
		rtSince := p.runtimeSince
		rtError := p.runtimeError
		p.mu.RUnlock()
		rtEvent := RPCEvent{
			"type":          "runtime_state",
			"runtimeState":  rtState,
			"runtimeReason": rtReason,
			"runtimeDetail": rtDetail,
			"runtimeSince":  rtSince,
			"_daemonTaskId": p.taskID,
			"_daemonRunId":  p.runID,
		}
		if rtError != "" {
			rtEvent["runtimeError"] = rtError
		}
		p.dispatchRuntimeState(rtEvent)
	}
	// Invalidate history cache when a message ends so REST requests return
	// fresh data instead of stale cached responses.
	if ev["type"] == "message_end" && p.onMessageEnd != nil {
		p.onMessageEnd()
	}
	if ev["type"] == "agent_end" || ev["type"] == "agent_settled" {
		p.releaseAdmission()
	}
}

// dispatchRuntimeState appends and fans out the synthetic state event without
// re-entering dispatch(). The runtime snapshot was already computed by the
// triggering event, so response handling and state-transition detection would
// be redundant work here.
func (p *PiProcess) dispatchRuntimeState(ev RPCEvent) {
	p.mu.Lock()
	p.eventSeq++
	id := p.eventSeq
	p.lastEventAt = time.Now().UTC()
	if encoded, err := json.Marshal(ev); err != nil {
		p.logger.Warn("not retaining runtime state event", "error", err)
	} else if len(encoded) <= p.eventMaxBytes {
		record := EventRecord{ID: id, Timestamp: time.Now().UTC(), Event: cloneEvent(ev), size: len(encoded)}
		p.events = append(p.events, record)
		p.eventBytes += record.size
		for len(p.events) > p.eventMax || p.eventBytes > p.eventMaxBytes {
			p.eventBytes -= p.events[0].size
			p.events = p.events[1:]
		}
	}
	out := eventWithID(ev, id)
	subs := make([]chan RPCEvent, 0, len(p.subs))
	for ch := range p.subs {
		subs = append(subs, ch)
	}
	p.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- out:
		default:
			atomic.AddUint64(&p.droppedEvents, 1)
			p.logger.Warn("dropping runtime state event for slow subscriber")
		}
	}
}

func (p *PiProcess) setRuntimeLocked(state, reason, detail string) {
	if p.runtimeState != state || p.runtimeReason != reason || p.runtimeDetail != detail {
		p.runtimeState, p.runtimeReason, p.runtimeDetail = state, reason, detail
		p.runtimeSince = time.Now().UTC()
	}
}

// emitRuntimeStateLocked broadcasts the current runtime state to WS subscribers
// and the event ring. Called from Start()/wait() which bypass dispatch().
// Caller must hold p.mu (write lock). Reads state under the held lock and
// returns a detached event for dispatch after the caller releases the lock.
func (p *PiProcess) runtimeStateEventLocked() RPCEvent {
	// Capture state while the caller still holds the write lock.
	event := RPCEvent{
		"type":          "runtime_state",
		"runtimeState":  p.runtimeState,
		"runtimeReason": p.runtimeReason,
		"runtimeDetail": p.runtimeDetail,
		"runtimeSince":  p.runtimeSince,
		"_daemonTaskId": p.taskID,
		"_daemonRunId":  p.runID,
	}
	if p.runtimeError != "" {
		event["runtimeError"] = p.runtimeError
	}
	return event
}

func (p *PiProcess) updateRuntimeFromEventLocked(ev RPCEvent) {
	switch ev["type"] {
	case "message_start":
		if message, _ := ev["message"].(map[string]any); message != nil {
			if role, _ := message["role"].(string); role == "user" || role == "assistant" {
				p.setRuntimeLocked("working", role, "Generating response")
			}
		}
	case "message_update":
		p.setRuntimeLocked("working", "assistant", "Generating response")
	case "tool_execution_start":
		p.setRuntimeLocked("working", "tool", stringValue(ev["toolName"], "Running tool"))
	case "tool_execution_update":
		p.setRuntimeLocked("working", "tool", stringValue(ev["toolName"], "Running tool"))
	case "tool_execution_end":
		p.setRuntimeLocked("working", "assistant", "Processing tool result")
	case "extension_ui_request":
		if extensionUIRequiresResponse(ev) {
			p.setRuntimeLocked("waiting_for_input", "extension", stringValue(ev["question"], stringValue(ev["message"], "Waiting for input")))
		}
	case "extension_ui_closed":
		if matched, _ := ev["_daemonExtensionUiCloseMatched"].(bool); matched && p.runtimeState == "waiting_for_input" {
			p.setRuntimeLocked("working", "extension", "Processing response")
		}
	case "message_end":
		if p.runtimeState != "waiting_for_input" {
			p.setRuntimeLocked("idle", "", "Ready")
		}
	case "error":
		p.runtimeError = stringValue(ev["error"], "Pi reported an error")
		p.setRuntimeLocked("failed", "error", p.runtimeError)
	}
}

func stringValue(value any, fallback string) string {
	if text, ok := value.(string); ok && text != "" {
		return text
	}
	return fallback
}

func (p *PiProcess) holdAdmission(release func()) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.admissionHeld {
		return false
	}
	p.admissionHeld = true
	p.onAgentSettled = release
	return true
}

func (p *PiProcess) releaseAdmission() {
	p.mu.Lock()
	if !p.admissionHeld {
		p.mu.Unlock()
		return
	}
	release := p.onAgentSettled
	p.admissionHeld = false
	p.onAgentSettled = nil
	p.mu.Unlock()
	if release != nil {
		release()
	}
}

func (p *PiProcess) removeWaiter(id string) { p.mu.Lock(); delete(p.waiters, id); p.mu.Unlock() }

func (p *PiProcess) Status() map[string]any {
	p.mu.RLock()
	defer p.mu.RUnlock()
	pid := 0
	if p.cmd != nil && p.cmd.Process != nil {
		pid = p.cmd.Process.Pid
	}
	status := "exited"
	if p.running {
		status = "running"
	}
	return map[string]any{"id": p.id, "cwd": p.spec.CWD, "args": p.spec.Args, "sessionPath": p.spec.SessionPath, "running": p.running, "status": status, "taskId": p.taskID, "runId": p.runID, "runtimeStatus": map[string]any{"state": p.runtimeState, "reason": p.runtimeReason, "detail": p.runtimeDetail, "since": p.runtimeSince, "lastError": p.runtimeError}, "pendingExtensionUiRequest": cloneEvent(p.pendingUIRequest), "restart": p.spec.Restart, "pid": pid, "wsSubscribers": len(p.subs), "eventCount": len(p.events), "latestEventId": p.eventSeq, "lastEventAt": p.lastEventAt, "droppedEvents": atomic.LoadUint64(&p.droppedEvents)}
}

func (p *PiProcess) Emit(event RPCEvent) { p.dispatch(event) }
func (p *PiProcess) CWD() string         { return p.spec.CWD }

// SubscriberCount returns the number of active WS subscribers.
func (p *PiProcess) SubscriberCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.subs)
}

func (p *PiProcess) Events(limit int, since uint64) []EventRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]EventRecord, 0)
	for _, record := range p.events {
		if record.ID > since {
			out = append(out, record)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func cloneEvent(event RPCEvent) RPCEvent {
	return cloneJSONValue(event).(RPCEvent)
}

// cloneJSONValue makes an ownership-safe copy of values carried by Pi's JSON
// protocol. Nested content arrays and objects may otherwise be mutated while
// a WebSocket writer is encoding an event.
func cloneJSONValue(value any) any {
	switch value := value.(type) {
	case RPCEvent:
		out := make(RPCEvent, len(value))
		for key, item := range value {
			out[key] = cloneJSONValue(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			out[key] = cloneJSONValue(item)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = cloneJSONValue(item)
		}
		return out
	case []string:
		return append([]string(nil), value...)
	default:
		return value
	}
}
func extensionUIRequiresResponse(event RPCEvent) bool {
	method, _ := event["method"].(string)
	switch method {
	case "select", "confirm", "input", "editor", "ask_user":
		return true
	default:
		return false
	}
}

func eventWithID(event RPCEvent, id uint64) RPCEvent {
	out := cloneEvent(event)
	out["_daemonEventId"] = id
	return out
}

var sessionSeq uint64

// NewSessionID generates a unique session/command ID using a timestamp plus a
// monotonic counter. The counter prevents collisions when multiple goroutines
// call this within the same nanosecond tick.
func NewSessionID() string {
	return fmt.Sprintf("s-%d-%d", time.Now().UnixNano(), atomic.AddUint64(&sessionSeq, 1))
}

func envMapToList(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}
