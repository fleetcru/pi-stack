package server

import (
	"crypto/rand"
	"encoding/binary"
	"strconv"
	"sync"
	"time"
)

// StatusEvent is the lightweight server-wide live status delta. It carries
// only routing and lifecycle fields — never prompt contents.
type StatusEvent struct {
	sequence  uint64
	Type      string `json:"type"`
	SessionID string `json:"sessionId,omitempty"`
	WorkerID  string `json:"workerId,omitempty"`
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
	Detail    string `json:"detail,omitempty"`
	RunID     string `json:"runId,omitempty"`
	// Position and QueuedAt are set on admission queue events (state
	// "queued"); Position is 1-based in admission order.
	Position  int        `json:"position,omitempty"`
	QueuedAt  *time.Time `json:"queuedAt,omitempty"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// statusHubStatusCursor is the bounded replay window for status subscribers.
const statusHubReplayLimit = 512

// statusHub fans out server-wide session/worker status deltas to lightweight
// subscribers. It keeps a bounded replay ring so reconnecting clients can
// catch up, and a current-state snapshot for fresh connections. Publishing is
// best-effort: slow subscribers drop live deltas and rely on replay.
type statusRingEntry struct {
	seq uint64
	ev  StatusEvent
}

type statusHub struct {
	mu sync.Mutex
	// generation is a per-process random epoch. Clients that observe a
	// different generation after a reconnect know the server restarted and
	// their cursor is meaningless — they must take a full snapshot.
	generation uint64
	seq        uint64
	ring       []statusRingEntry
	last       map[string]StatusEvent
	subs       map[chan StatusEvent]struct{}
}

func newStatusHub() *statusHub {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// Fall back to nanosecond time; uniqueness across restarts is what matters.
		binary.BigEndian.PutUint64(raw[:], uint64(time.Now().UnixNano()))
	}
	return &statusHub{
		generation: binary.BigEndian.Uint64(raw[:]),
		last:       map[string]StatusEvent{},
		subs:       map[chan StatusEvent]struct{}{},
	}
}

// Generation returns the hub's restart epoch as a decimal string.
func (h *statusHub) Generation() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strconv.FormatUint(h.generation, 10)
}

func statusEventKey(ev StatusEvent) string {
	// Admission records describe a run, not the session's runtime. A session
	// may be working while its next prompt is queued, so keep both states.
	if ev.Reason == "admission" && ev.RunID != "" {
		return "run:" + ev.RunID
	}
	if ev.SessionID != "" {
		return "session:" + ev.SessionID
	}
	return "worker:" + ev.WorkerID
}

// publish records a delta and fans it out. Identical consecutive states for
// the same session/worker are suppressed to keep the stream cheap.
func (h *statusHub) publish(sessionID, workerID, state, reason, detail, runID string) {
	h.publishEvent(StatusEvent{
		Type:      "session_status",
		SessionID: sessionID,
		WorkerID:  workerID,
		State:     state,
		Reason:    reason,
		Detail:    detail,
		RunID:     runID,
		UpdatedAt: time.Now().UTC(),
	})
}

// publishAdmission reports an admission queue transition ("queued",
// "starting", "cancelled") including the run's 1-based queue position and
// enqueue timestamp so clients can render live queue state.
func (h *statusHub) publishAdmission(sessionID, workerID, state, reason, detail, runID string, position int, queuedAt time.Time) {
	ev := StatusEvent{
		Type:      "session_status",
		SessionID: sessionID,
		WorkerID:  workerID,
		State:     state,
		Reason:    reason,
		Detail:    detail,
		RunID:     runID,
		Position:  position,
		UpdatedAt: time.Now().UTC(),
	}
	if !queuedAt.IsZero() {
		ts := queuedAt
		ev.QueuedAt = &ts
	}
	h.publishEvent(ev)
}

// publishEvent records a delta and fans it out. Identical consecutive states
// for the same session/worker are suppressed — including detail, runId, and
// admission position changes, which are significant for queue events.
func (h *statusHub) publishEvent(ev StatusEvent) {
	if ev.State == "" {
		return
	}
	h.mu.Lock()
	key := statusEventKey(ev)
	if prev, ok := h.last[key]; ok && prev.State == ev.State && prev.Reason == ev.Reason && prev.Detail == ev.Detail && prev.RunID == ev.RunID && prev.Position == ev.Position {
		h.mu.Unlock()
		return
	}
	h.seq++
	ev.sequence = h.seq
	if ev.Reason == "admission" && ev.State == "cancelled" {
		delete(h.last, key)
	} else {
		h.last[key] = ev
	}
	if ev.Reason != "admission" && ev.RunID != "" {
		delete(h.last, "run:"+ev.RunID)
	}
	h.ring = append(h.ring, statusRingEntry{seq: h.seq, ev: ev})
	for len(h.ring) > statusHubReplayLimit {
		h.ring = h.ring[1:]
	}
	subs := make([]chan StatusEvent, 0, len(h.subs))
	for ch := range h.subs {
		subs = append(subs, ch)
	}
	h.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default: // slow subscribers reconnect and replay; never block publishers
		}
	}
}

// snapshot returns the current status of every known session and worker.
func (h *statusHub) snapshot() ([]StatusEvent, uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]StatusEvent, 0, len(h.last))
	for _, ev := range h.last {
		out = append(out, ev)
	}
	return out, h.seq
}

// replaySince returns events after the given cursor. When the cursor predates
// the ring, ok is false and the caller must send a full snapshot instead.
func (h *statusHub) replaySince(cursor uint64) ([]StatusEvent, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.ring) == 0 {
		return nil, cursor < h.seq
	}
	if cursor < h.ring[0].seq-1 {
		return nil, false // gap: caller must fall back to a full snapshot
	}
	out := make([]StatusEvent, 0, len(h.ring))
	for _, entry := range h.ring {
		if entry.seq > cursor {
			out = append(out, entry.ev)
		}
	}
	return out, true
}

// currentCursor returns the latest assigned sequence number.
func (h *statusHub) currentCursor() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.seq
}

func (h *statusHub) subscribe() (<-chan StatusEvent, func()) {
	ch, _, _, _, cancel := h.subscribeWithSnapshot()
	return ch, cancel
}

// subscribeWithSnapshot registers a subscriber and captures the current
// snapshot atomically under the hub lock. Events published after the snapshot
// are delivered on the channel, so no delta can fall between snapshot and
// subscription. Callers must still treat snapshot events as the baseline.
func (h *statusHub) subscribeWithSnapshot() (ch <-chan StatusEvent, events []StatusEvent, seq uint64, generation string, cancel func()) {
	live := make(chan StatusEvent, 128)
	h.mu.Lock()
	h.subs[live] = struct{}{}
	events = make([]StatusEvent, 0, len(h.last))
	for _, ev := range h.last {
		events = append(events, ev)
	}
	seq = h.seq
	generation = strconv.FormatUint(h.generation, 10)
	h.mu.Unlock()
	return live, events, seq, generation, func() {
		h.mu.Lock()
		delete(h.subs, live)
		h.mu.Unlock()
	}
}

// publishSessionStatus is the nil-safe entry point for status instrumentation;
// tests may construct bare Server values without a hub.
func (s *Server) publishSessionStatus(sessionID, workerID, state, reason, detail, runID string) {
	if s.status != nil {
		s.status.publish(sessionID, workerID, state, reason, detail, runID)
	}
}

// publishAdmissionStatus is the nil-safe admission queue status publisher.
func (s *Server) publishAdmissionStatus(sessionID, workerID, state, reason, detail, runID string, position int, queuedAt time.Time) {
	if s.status != nil {
		s.status.publishAdmission(sessionID, workerID, state, reason, detail, runID, position, queuedAt)
	}
}

// wireAdmissionStatus forwards admission queue transitions to the status hub.
// TaskAdmission invokes the observer outside its mutex, so publishing here may
// freely take the hub lock. Queued runs publish state "queued" with their
// 1-based position, grants publish "starting", and removals publish
// "cancelled" — all carrying the admission runId.
func (s *Server) wireAdmissionStatus() {
	s.admission.SetOnChange(func(change admissionChange) {
		run := change.Run
		switch change.Kind {
		case "queued":
			s.publishAdmissionStatus(run.SessionID, run.WorkerID, "queued", "admission", "Run waiting in admission queue", run.RunID, run.Position, run.QueuedAt)
		case "granted":
			s.publishAdmissionStatus(run.SessionID, run.WorkerID, "starting", "admission", "Run admitted from queue", run.RunID, 0, run.QueuedAt)
		case "cancelled":
			s.publishAdmissionStatus(run.SessionID, run.WorkerID, "cancelled", "admission", "Queued run cancelled", run.RunID, 0, run.QueuedAt)
		}
	})
}

// wirePiProcess attaches live-status instrumentation to a locally managed Pi
// process. Local sessions always report workerId "local".
func (s *Server) wirePiProcess(p *PiProcess) {
	p.onRuntimeState = func(state, reason, detail, runID string) {
		s.publishSessionStatus(p.id, "local", state, reason, detail, runID)
	}
}

// publishRelayStatus reports relay session lifecycle transitions derived from
// the bridge event stream.
func (s *Server) publishRelayStatus(sessionID, state, reason, detail, runID string) {
	s.publishSessionStatus(sessionID, "relay", state, reason, detail, runID)
}

// publishWorkerStatus reports worker health transitions. A failed heartbeat
// is surfaced as "offline" so clients can stop dispatching work to it.
func (s *Server) publishWorkerStatus(workerID string, healthy bool) {
	state := "healthy"
	if !healthy {
		state = "offline"
	}
	s.publishSessionStatus("", workerID, state, "heartbeat", "", "")
}
