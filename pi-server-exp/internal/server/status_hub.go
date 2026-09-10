package server

import (
	"sync"
	"time"
)

// StatusEvent is the lightweight server-wide live status delta. It carries
// only routing and lifecycle fields — never prompt contents.
type StatusEvent struct {
	Type      string    `json:"type"`
	SessionID string    `json:"sessionId,omitempty"`
	WorkerID  string    `json:"workerId,omitempty"`
	State     string    `json:"state"`
	Reason    string    `json:"reason,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	RunID     string    `json:"runId,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
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
	mu     sync.Mutex
	seq    uint64
	ring   []statusRingEntry
	last   map[string]StatusEvent
	subs   map[chan StatusEvent]struct{}
}

func newStatusHub() *statusHub {
	return &statusHub{
		last: map[string]StatusEvent{},
		subs: map[chan StatusEvent]struct{}{},
	}
}

func statusEventKey(ev StatusEvent) string {
	if ev.SessionID != "" {
		return "session:" + ev.SessionID
	}
	return "worker:" + ev.WorkerID
}

// publish records a delta and fans it out. Identical consecutive states for
// the same session/worker are suppressed to keep the stream cheap.
func (h *statusHub) publish(sessionID, workerID, state, reason, detail, runID string) {
	if state == "" {
		return
	}
	ev := StatusEvent{
		Type:      "session_status",
		SessionID: sessionID,
		WorkerID:  workerID,
		State:     state,
		Reason:    reason,
		Detail:    detail,
		RunID:     runID,
		UpdatedAt: time.Now().UTC(),
	}
	h.mu.Lock()
	key := statusEventKey(ev)
	if prev, ok := h.last[key]; ok && prev.State == ev.State && prev.Reason == ev.Reason && prev.RunID == ev.RunID {
		h.mu.Unlock()
		return
	}
	h.seq++
	h.last[key] = ev
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
	ch := make(chan StatusEvent, 128)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
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
