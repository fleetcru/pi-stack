package server

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// External sessions are interactive Pi TUI processes that opt into relaying.
// pi-server never starts or restarts them; it only fans out their events and
// queues remote controls for the bridge extension to acknowledge.
type ExternalCommand struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Message  string `json:"message,omitempty"`
	Delivery string `json:"delivery,omitempty"`
}

type ExternalSession struct {
	ID              string
	CWD             string
	Title           string
	SessionPath     string
	Model           map[string]any
	ThinkingLevel   string
	LastUsage       map[string]any
	TotalCost       float64
	MessageCount    int
	Status          string
	UpdatedAt       time.Time
	next            uint64
	eventBytes      int
	events          []EventRecord
	subs            map[chan RPCEvent]struct{}
	commands        []ExternalCommand
	relay           chan ExternalCommand
	relayGeneration uint64
	RelayConnected  bool
	RelayLatencyMS  int64
	// leaseID identifies the bridge instance that owns this session; leaseToken
	// must be presented on HTTP command polling and ack. A different bridge
	// re-registering rotates the lease and detaches any stale relay.
	leaseID    string
	leaseToken string
}

type ExternalRegistry struct {
	mu          sync.RWMutex
	sessions    map[string]*ExternalSession
	commandPath string
	persisted   persistedRelayCommands
}

func newExternalRegistry(commandPath string) *ExternalRegistry {
	persisted, _ := loadRelayCommands(commandPath)
	return &ExternalRegistry{sessions: map[string]*ExternalSession{}, commandPath: commandPath, persisted: persisted}
}

func (r *ExternalRegistry) saveCommandsLocked() {
	for id, session := range r.sessions {
		if len(session.commands) == 0 {
			delete(r.persisted, id)
		} else {
			r.persisted[id] = append([]ExternalCommand(nil), session.commands...)
		}
	}
	// Persist synchronously to ensure commands survive a server restart.
	// The atomic rename is fast and the lock is held for a very short time.
	if err := saveRelayCommands(r.commandPath, r.persisted); err != nil {
		slog.Error("failed to persist relay commands", "error", err, "path", r.commandPath)
	}
}



// register upserts the session and returns the command-queue lease. The same
// bridge (identified by leaseID) re-registering keeps its lease; a different
// bridge rotates it, invalidating the previous holder and its relay.
func (r *ExternalRegistry) register(id, cwd, title, sessionPath, leaseID string) (*ExternalSession, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sessions[id]
	if s == nil {
		s = &ExternalSession{ID: id, CWD: cwd, Title: title, SessionPath: sessionPath, Status: "idle", subs: map[chan RPCEvent]struct{}{}}
		if pending := r.persisted[id]; len(pending) > 0 {
			s.commands = append([]ExternalCommand(nil), pending...)
		}
		r.sessions[id] = s
	}
	if s.leaseToken == "" || (leaseID != "" && s.leaseID != leaseID) {
		// New owner: rotate the lease and detach any relay held by the previous
		// bridge so its read loop observes isCurrentRelay == false and exits.
		s.leaseID = leaseID
		s.leaseToken = newRequestID()
		s.relay = nil
		s.RelayConnected = false
	}
	s.CWD = cwd
	if title != "" {
		s.Title = title
	}
	if sessionPath != "" {
		s.SessionPath = sessionPath
	}
	if s.Status == "stale" || s.Status == "stopped" {
		s.Status = "idle"
	}
	s.UpdatedAt = time.Now().UTC()
	return s, s.leaseToken
}

// get returns a value copy of the session's read-only fields. Callers never
// receive a pointer into the registry, so concurrent publish() writes cannot
// race against field reads after the lock is released.
func (r *ExternalRegistry) get(id string) (ExternalSession, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[id]
	if !ok {
		return ExternalSession{}, false
	}
	return ExternalSession{
		ID:              s.ID,
		CWD:             s.CWD,
		Title:           s.Title,
		SessionPath:     s.SessionPath,
		Model:           s.Model,
		ThinkingLevel:   s.ThinkingLevel,
		LastUsage:       s.LastUsage,
		TotalCost:       s.TotalCost,
		MessageCount:    s.MessageCount,
		Status:          s.Status,
		UpdatedAt:       s.UpdatedAt,
		RelayConnected:  s.RelayConnected,
		RelayLatencyMS:  s.RelayLatencyMS,
	}, true
}

// eventCount returns the next event ID (number of published events) under
// the registry lock. Used by tests that previously read s.next from the
// pointer returned by get().
func (r *ExternalRegistry) eventCount(id string) uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s := r.sessions[id]; s != nil {
		return s.next
	}
	return 0
}

// stateSnapshot returns a map suitable for API "state" responses. All reads
// happen under the registry lock so callers see a consistent snapshot even
// while publish() is concurrently writing.
func (r *ExternalRegistry) stateSnapshot(id string) map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[id]
	if !ok {
		return nil
	}
	running := s.Status != "stale" && s.Status != "stopped"
	return map[string]any{
		"external":        true,
		"running":         running,
		"status":          s.Status,
		"model":           s.Model,
		"thinkingLevel":   s.ThinkingLevel,
		"transport":       "relay",
		"relayConnected":  s.RelayConnected,
		"relayLatencyMs":  s.RelayLatencyMS,
	}
}

func (r *ExternalRegistry) publish(id string, ev RPCEvent) bool {
	r.mu.Lock()
	s := r.sessions[id]
	if s == nil {
		r.mu.Unlock()
		return false
	}
	s.next++
	encoded, err := json.Marshal(ev)
	if err != nil {
		slog.Warn("failed to marshal relay event", "error", err, "eventType", ev["type"])
		r.mu.Unlock()
		return false
	}
	record := EventRecord{ID: s.next, Timestamp: time.Now().UTC(), Event: ev, size: len(encoded)}
	s.events = append(s.events, record)
	s.eventBytes += record.size
	// Evict oldest events when either the count or byte limit is exceeded.
	// This matches PiProcess's dual-bound event ring.
	const maxRelayEventBytes = 8 << 20 // 8 MB
	for len(s.events) > 200 || s.eventBytes > maxRelayEventBytes {
		s.eventBytes -= s.events[0].size
		s.events = s.events[1:]
	}
	switch ev["type"] {
	case "model_select":
		if model, ok := ev["model"].(map[string]any); ok {
			s.Model = model
		}
	case "thinking_level_select":
		if level, ok := ev["level"].(string); ok {
			s.ThinkingLevel = level
		}
	case "message_start", "message_update", "tool_execution_start", "tool_execution_update":
		s.Status = "working"
	case "extension_ui_request":
		if extensionUIRequiresResponse(ev) {
			s.Status = "waiting_for_input"
		}
	case "message_end":
		if message, _ := ev["message"].(map[string]any); message != nil {
			if role, _ := message["role"].(string); role == "assistant" {
				s.MessageCount++
				if usage, _ := message["usage"].(map[string]any); usage != nil {
					s.LastUsage = usage
					if cost, _ := usage["cost"].(map[string]any); cost != nil {
						s.TotalCost += relayNumber(cost["total"])
					}
				}
			}
		}
		if s.Status != "waiting_for_input" {
			s.Status = "idle"
		}
	}
	s.UpdatedAt = record.Timestamp
	out := eventWithID(ev, record.ID)
	subs := make([]chan RPCEvent, 0, len(s.subs))
	for ch := range s.subs {
		subs = append(subs, ch)
	}
	r.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- out:
		default: // live viewers can reconnect and replay the bounded event ring
		}
	}
	return true
}

func (r *ExternalRegistry) subscribe(id string, since uint64) (chan RPCEvent, []RPCEvent, func(), bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sessions[id]
	if s == nil {
		return nil, nil, nil, false
	}
	ch := make(chan RPCEvent, 256)
	s.subs[ch] = struct{}{}
	replay := make([]RPCEvent, 0)
	if since > 0 && len(s.events) > 0 && s.events[0].ID > since+1 {
		// The requested cursor predates the bounded relay ring. Tell clients to
		// reconcile durable history before replaying the retained tail.
		replay = append(replay, eventWithID(RPCEvent{"type": "events_lost", "expectedAfter": since, "received": s.events[0].ID}, 0))
	}
	for _, event := range s.events {
		if event.ID > since {
			replay = append(replay, eventWithID(event.Event, event.ID))
		}
	}
	return ch, replay, func() {
		r.mu.Lock()
		delete(s.subs, ch)
		r.mu.Unlock()
	}, true
}

func (r *ExternalRegistry) enqueue(id string, command ExternalCommand) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sessions[id]
	if s == nil || s.Status == "stale" || s.Status == "stopped" {
		return false
	}
	if len(s.commands) >= 100 {
		if command.Type != "abort" {
			// Non-abort commands are rejected when the queue is full so the
			// caller can surface the failure.
			return false
		}
		// Abort always succeeds: evict the oldest command regardless of type.
		// An abort is idempotent — delivering a duplicate is harmless, but
		// failing to deliver one leaves a stuck session.
		s.commands = s.commands[1:]
	}
	s.commands = append(s.commands, command)
	r.saveCommandsLocked()
	s.UpdatedAt = time.Now().UTC()
	if s.relay != nil {
		select {
		case s.relay <- command:
		default:
		}
	}
	return true
}

// commandsFor serves the pending queue to the current leaseholder only. The
// leaseholder's poll doubles as the relay heartbeat (a stale poller must not
// keep a dead session looking alive). Both validation and copying occur under
// one lock so a lease cannot rotate between them.
func (r *ExternalRegistry) commandsFor(id, lease string) (commands []ExternalCommand, exists, authorized bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sessions[id]
	if s == nil {
		return nil, false, false
	}
	if s.leaseToken == "" || subtle.ConstantTimeCompare([]byte(s.leaseToken), []byte(lease)) != 1 {
		return nil, true, false
	}
	// Command polling doubles as a relay heartbeat.
	s.UpdatedAt = time.Now().UTC()
	if s.Status == "stale" {
		s.Status = "idle"
	}
	return append(make([]ExternalCommand, 0, len(s.commands)), s.commands...), true, true
}

// attachRelay attaches a WS relay only when it presents the current HTTP
// command lease. This prevents a stale bridge from bypassing the polling lease
// and reclaiming delivery through the WebSocket path.
func (r *ExternalRegistry) attachRelay(id, lease string) (<-chan ExternalCommand, []ExternalCommand, uint64, func(), bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sessions[id]
	if s == nil {
		return nil, nil, 0, nil, false, false
	}
	if s.leaseToken == "" || subtle.ConstantTimeCompare([]byte(s.leaseToken), []byte(lease)) != 1 {
		return nil, nil, 0, nil, true, false
	}
	channel := make(chan ExternalCommand, 32)
	s.relayGeneration++
	generation := s.relayGeneration
	s.relay = channel
	s.RelayConnected = true
	s.UpdatedAt = time.Now().UTC()
	pending := append(make([]ExternalCommand, 0, len(s.commands)), s.commands...)
	return channel, pending, generation, func() {
		r.mu.Lock()
		if s.relay == channel && s.relayGeneration == generation {
			s.relay = nil
			s.RelayConnected = false
		}
		r.mu.Unlock()
	}, true, true
}

func (r *ExternalRegistry) isCurrentRelay(id string, generation uint64) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := r.sessions[id]
	return s != nil && s.relay != nil && s.relayGeneration == generation
}

func (r *ExternalRegistry) setRelayLatency(id string, generation uint64, latency time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s := r.sessions[id]; s != nil && s.relayGeneration == generation {
		s.RelayLatencyMS = latency.Milliseconds()
		s.RelayConnected = true
		s.UpdatedAt = time.Now().UTC()
	}
}

// heartbeatRelay updates liveness only for the generation that owns the relay
// WebSocket; HTTP poll leases use commandsFor instead.
func (r *ExternalRegistry) heartbeatRelay(id string, generation uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s := r.sessions[id]; s != nil && s.relay != nil && s.relayGeneration == generation {
		s.UpdatedAt = time.Now().UTC()
	}
}

// acknowledge removes commands after a generation-checked relay WS receipt.
func (r *ExternalRegistry) acknowledge(id string, ids []string) bool {
	exists, authorized := r.acknowledgeLocked(id, ids, "", false)
	return exists && authorized
}

// acknowledgeForLease removes commands after an HTTP polling receipt, but only
// if the caller still owns the current lease.
func (r *ExternalRegistry) acknowledgeForLease(id string, ids []string, lease string) (exists, authorized bool) {
	return r.acknowledgeLocked(id, ids, lease, true)
}

func (r *ExternalRegistry) acknowledgeLocked(id string, ids []string, lease string, requireLease bool) (exists, authorized bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.sessions[id]
	if s == nil {
		return false, false
	}
	if requireLease && (s.leaseToken == "" || subtle.ConstantTimeCompare([]byte(s.leaseToken), []byte(lease)) != 1) {
		return true, false
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		seen[id] = struct{}{}
	}
	kept := s.commands[:0]
	for _, command := range s.commands {
		if _, ok := seen[command.ID]; !ok {
			kept = append(kept, command)
		}
	}
	s.commands = kept
	r.saveCommandsLocked()
	s.UpdatedAt = time.Now().UTC()
	return true, true
}

func (s *Server) externalRegister(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID, CWD, Title, SessionPath string
		BridgeID                    string `json:"bridgeId"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || !validSessionID(input.ID) || input.BridgeID == "" {
		writeErrorText(w, http.StatusBadRequest, "valid id and bridgeId required")
		return
	}
	external, lease := s.external.register(input.ID, input.CWD, input.Title, input.SessionPath, input.BridgeID)
	_, err := s.sessions.RegisterSpec(SessionSpec{ID: external.ID, CWD: external.CWD, Title: external.Title, Status: external.Status, Managed: false, Transport: "relay", SessionPath: external.SessionPath})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": external.ID, "sessionId": external.ID, "registered": true, "lease": lease})
}

func (s *Server) externalPost(w http.ResponseWriter, r *http.Request) {
	id, action := splitExternalPath(r.URL.Path)
	switch action {
	case "events":
		var event RPCEvent
		if json.NewDecoder(r.Body).Decode(&event) != nil || !s.external.publish(id, event) {
			writeErrorText(w, http.StatusNotFound, "external session not found")
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
	case "ack":
		var input struct {
			IDs   []string `json:"ids"`
			Lease string   `json:"lease"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			writeErrorText(w, http.StatusBadRequest, "invalid acknowledgement")
			return
		}
		exists, authorized := s.external.acknowledgeForLease(id, input.IDs, input.Lease)
		if !exists {
			writeErrorText(w, http.StatusNotFound, "external session not found")
			return
		}
		if !authorized {
			writeErrorText(w, http.StatusConflict, "external relay lease is no longer current")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"acknowledged": len(input.IDs)})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) externalGet(w http.ResponseWriter, r *http.Request) {
	id, action := splitExternalPath(r.URL.Path)
	if action != "commands" {
		http.NotFound(w, r)
		return
	}
	commands, exists, authorized := s.external.commandsFor(id, r.URL.Query().Get("lease"))
	if !exists {
		writeErrorText(w, http.StatusNotFound, "external session not found")
		return
	}
	if !authorized {
		writeErrorText(w, http.StatusConflict, "external relay lease is no longer current")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"commands": commands})
}

func splitExternalPath(path string) (string, string) {
	const prefix = "/v1/external-sessions/"
	tail := path[len(prefix):]
	for i, char := range tail {
		if char == '/' {
			return tail[:i], tail[i+1:]
		}
	}
	return tail, ""
}
