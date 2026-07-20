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

	seq           uint64
	waiters       map[string]responseWaiter
	subs          map[chan RPCEvent]struct{}
	events        []EventRecord
	eventMax      int
	eventMaxBytes int
	eventBytes    int
	eventSeq      uint64
	restarts      int
	runtimeState  string
	runtimeReason string
	runtimeDetail string
	runtimeSince  time.Time
	runtimeError  string
	// onMessageEnd is called when a message_end event is dispatched.
	// Used to invalidate the history cache so REST requests return fresh data.
	onMessageEnd func()
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
	return &PiProcess{id: spec.ID, cfg: cfg, spec: spec, logger: logger.With("session", spec.ID, "cwd", spec.CWD), waiters: map[string]responseWaiter{}, subs: map[chan RPCEvent]struct{}{}, eventMax: eventMax, eventMaxBytes: eventMaxBytes, runtimeState: "created"}
}

func (p *PiProcess) Start(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running {
		return nil
	}
	if p.closed {
		return errors.New("session closed")
	}
	p.setRuntimeLocked("starting", "process", "Starting Pi")
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
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	p.cmd, p.stdin, p.running, p.done = cmd, stdin, true, make(chan struct{})
	p.setRuntimeLocked("idle", "process", "Ready")
	go p.readStdout(stdout)
	go p.readStderr(stderr)
	go p.wait(cmd)
	p.logger.Info("pi rpc process started", "pid", cmd.Process.Pid, "args", args)
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
	defer p.mu.Unlock()
	if p.closed || p.stdin == nil {
		return errors.New("pi process closed")
	}
	_, err = p.stdin.Write(append(b, '\n'))
	return err
}

func (p *PiProcess) SubscribeSince(since uint64) (<-chan RPCEvent, []EventRecord, func()) {
	ch := make(chan RPCEvent, 256)
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

func (p *PiProcess) Subscribe() (<-chan RPCEvent, func()) {
	ch, _, close := p.SubscribeSince(0)
	return ch, close
}

func (p *PiProcess) Close(ctx context.Context) error {
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
	if p.cmd == cmd {
		p.running = false
		if p.spec.Restart && !p.closed && p.restarts < p.cfg.RestartMax {
			p.setRuntimeLocked("reconnecting", "process", "Restarting Pi")
		} else {
			p.setRuntimeLocked("stopped", "process", "Pi process stopped")
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
	p.updateRuntimeFromEventLocked(ev)
	p.eventSeq++
	id := p.eventSeq
	// Retaining full Pi events is optional replay convenience, so account for
	// serialized payload bytes and keep the per-session history on a hard budget.
	if encoded, err := json.Marshal(ev); err != nil {
		p.logger.Warn("not retaining event that cannot be encoded", "error", err)
	} else if len(encoded) <= p.eventMaxBytes {
		record := EventRecord{ID: id, Timestamp: time.Now().UTC(), Event: cloneEvent(ev), size: len(encoded)}
		p.events = append(p.events, record)
		p.eventBytes += record.size
		for len(p.events) > p.eventMax || p.eventBytes > p.eventMaxBytes {
			p.eventBytes -= p.events[0].size
			p.events = p.events[1:]
		}
	} else {
		p.logger.Warn("not retaining oversized event", "bytes", len(encoded), "limit", p.eventMaxBytes)
	}
	out := eventWithID(ev, id)
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
			p.logger.Warn("dropping event for slow subscriber")
		}
	}
	// Invalidate history cache when a message ends so REST requests return
	// fresh data instead of stale cached responses.
	if ev["type"] == "message_end" && p.onMessageEnd != nil {
		p.onMessageEnd()
	}
}

func (p *PiProcess) setRuntimeLocked(state, reason, detail string) {
	if p.runtimeState != state || p.runtimeReason != reason || p.runtimeDetail != detail {
		p.runtimeState, p.runtimeReason, p.runtimeDetail = state, reason, detail
		p.runtimeSince = time.Now().UTC()
	}
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
			p.setRuntimeLocked("waiting_for_input", "extension", stringValue(ev["message"], "Waiting for input"))
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
	return map[string]any{"id": p.id, "cwd": p.spec.CWD, "args": p.spec.Args, "sessionPath": p.spec.SessionPath, "running": p.running, "status": status, "runtimeStatus": map[string]any{"state": p.runtimeState, "reason": p.runtimeReason, "detail": p.runtimeDetail, "since": p.runtimeSince, "lastError": p.runtimeError}, "restart": p.spec.Restart, "pid": pid, "wsSubscribers": len(p.subs), "eventCount": len(p.events)}
}

func (p *PiProcess) Emit(event RPCEvent) { p.dispatch(event) }
func (p *PiProcess) CWD() string         { return p.spec.CWD }

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
	out := make(RPCEvent, len(event))
	for k, v := range event {
		out[k] = v
	}
	return out
}
func extensionUIRequiresResponse(event RPCEvent) bool {
	method, _ := event["method"].(string)
	switch method {
	case "select", "confirm", "input", "editor":
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
