package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const remoteStatusTimeout = 5 * time.Second

// distributedReservation is the hub's durable record that a remote or relay
// run occupies admission capacity. The timer is a local recovery guard for
// missed lifecycle events; done lets the subscriber stop promptly on release.
type distributedReservation struct {
	WorkerID        string    `json:"workerId"`
	Kind            string    `json:"kind"`
	RemoteSessionID string    `json:"remoteSessionId,omitempty"`
	ExpiresAt       time.Time `json:"expiresAt,omitempty"`
	timer           *time.Timer
	done            chan struct{}
}

// acquireDistributedRun reserves capacity before dispatching a remote prompt.
// The reservation is recorded before the worker is contacted so a very short
// run cannot finish before its lifecycle subscriber is attached.
func (s *Server) acquireDistributedRun(ctx context.Context, sessionID, workerID string) bool {
	if !s.admission.Acquire(ctx, sessionID, workerID) {
		return false
	}
	s.distributedMu.Lock()
	if _, exists := s.distributedRuns[sessionID]; exists {
		s.distributedMu.Unlock()
		s.admission.Release(sessionID, workerID)
		return false
	}
	reservation := distributedReservation{WorkerID: workerID, done: make(chan struct{})}
	if s.cfg.DistributedRunTimeout > 0 {
		reservation.ExpiresAt = time.Now().Add(s.cfg.DistributedRunTimeout)
		reservation.timer = time.AfterFunc(s.cfg.DistributedRunTimeout, func() { s.releaseDistributedRun(sessionID) })
	}
	s.distributedRuns[sessionID] = reservation
	s.distributedMu.Unlock()
	s.persistDistributedRuns()
	return true
}

// observeDistributedRun adopts a run started by another process (for example,
// a relay). TryAcquire applies the same global/session/worker limits as local
// prompts without double-counting an already tracked reservation.
func (s *Server) observeDistributedRun(sessionID, workerID, kind string) {
	if !s.admission.TryAcquire(sessionID, workerID) {
		return
	}
	s.distributedMu.Lock()
	if _, exists := s.distributedRuns[sessionID]; exists {
		s.distributedMu.Unlock()
		s.admission.Release(sessionID, workerID)
		return
	}
	reservation := distributedReservation{WorkerID: workerID, Kind: kind, done: make(chan struct{})}
	if s.cfg.DistributedRunTimeout > 0 {
		reservation.ExpiresAt = time.Now().Add(s.cfg.DistributedRunTimeout)
		reservation.timer = time.AfterFunc(s.cfg.DistributedRunTimeout, func() { s.releaseDistributedRun(sessionID) })
	}
	s.distributedRuns[sessionID] = reservation
	s.distributedMu.Unlock()
	s.persistDistributedRuns()
}

func (s *Server) distributedRunDone(sessionID string) <-chan struct{} {
	s.distributedMu.Lock()
	defer s.distributedMu.Unlock()
	if reservation, ok := s.distributedRuns[sessionID]; ok && reservation.done != nil {
		return reservation.done
	}
	closed := make(chan struct{})
	close(closed)
	return closed
}

func (s *Server) distributedRunActive(sessionID string) bool {
	s.distributedMu.Lock()
	defer s.distributedMu.Unlock()
	_, ok := s.distributedRuns[sessionID]
	return ok
}

// releaseDistributedRun is idempotent: timeouts, lifecycle events, and
// shutdown cleanup may race to release the same reservation.
func (s *Server) releaseDistributedRun(sessionID string) {
	s.distributedMu.Lock()
	reservation, ok := s.distributedRuns[sessionID]
	if ok {
		delete(s.distributedRuns, sessionID)
		if reservation.timer != nil {
			reservation.timer.Stop()
		}
		if reservation.done != nil {
			close(reservation.done)
		}
	}
	s.distributedMu.Unlock()
	if ok {
		s.admission.Release(sessionID, reservation.WorkerID)
		s.persistDistributedRuns()
	}
}

// setDistributedRunMetadata enriches a persisted reservation after the worker
// has assigned its remote session identifier.
func (s *Server) setDistributedRunMetadata(sessionID, kind, remoteSessionID string) {
	s.distributedMu.Lock()
	reservation, ok := s.distributedRuns[sessionID]
	if ok {
		reservation.Kind = kind
		reservation.RemoteSessionID = remoteSessionID
		s.distributedRuns[sessionID] = reservation
	}
	s.distributedMu.Unlock()
	if ok {
		s.persistDistributedRuns()
	}
}

// remoteEventCursor captures the worker cursor before prompt dispatch. The
// subsequent WebSocket replay therefore includes even runs that finish before
// the hub finishes connecting.
func (s *Server) remoteEventCursor(worker Worker, sessionID string) (uint64, bool) {
	base, err := url.Parse(strings.TrimRight(worker.URL, "/"))
	if err != nil {
		return 0, false
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/v1/sessions/" + url.PathEscape(sessionID) + "/daemon-status"
	ctx, cancel := context.WithTimeout(context.Background(), remoteStatusTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return 0, false
	}
	if string(worker.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+string(worker.Token))
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, false
	}
	var payload struct {
		LatestEventID *uint64 `json:"latestEventId"`
	}
	if json.NewDecoder(resp.Body).Decode(&payload) != nil || payload.LatestEventID == nil {
		return 0, false
	}
	return *payload.LatestEventID, true
}

func (s *Server) remoteRuntimeState(worker Worker, sessionID string) (string, bool, bool) {
	base, err := url.Parse(strings.TrimRight(worker.URL, "/"))
	if err != nil {
		return "", false, false
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/v1/sessions/" + url.PathEscape(sessionID) + "/daemon-status"
	ctx, cancel := context.WithTimeout(context.Background(), remoteStatusTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return "", false, false
	}
	if string(worker.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+string(worker.Token))
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", false, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false, false
	}
	var payload struct {
		Running       bool `json:"running"`
		RuntimeStatus struct {
			State string `json:"state"`
		} `json:"runtimeStatus"`
	}
	if json.NewDecoder(resp.Body).Decode(&payload) != nil {
		return "", false, false
	}
	return payload.RuntimeStatus.State, payload.Running, true
}

// subscribeRemoteRun consumes the worker's authoritative lifecycle stream.
// It reconnects with the last cursor during outages and relies on replay to
// avoid missing short runs.
func (s *Server) subscribeRemoteRun(worker Worker, remoteSessionID, hubSessionID string, since uint64, requireStart bool) {
	go func() {
		cursor := since
		seenStart := !requireStart
		backoff := 250 * time.Millisecond
		for s.distributedRunActive(hubSessionID) {
			path := "/v1/sessions/" + url.PathEscape(remoteSessionID) + "/ws"
			remoteURL, err := workerWebSocketURL(worker.URL, path, "since="+strconv.FormatUint(cursor, 10))
			if err != nil {
				s.logger.Warn("remote lifecycle URL failed", "session", hubSessionID, "error", err)
				return
			}
			header := http.Header{}
			if string(worker.Token) != "" {
				header.Set("Authorization", "Bearer "+string(worker.Token))
			}
			conn, _, err := websocket.DefaultDialer.Dial(remoteURL, header)
			if err != nil {
				time.Sleep(backoff)
				if backoff < remoteStatusTimeout {
					backoff *= 2
				}
				continue
			}
			backoff = 250 * time.Millisecond
			connectionDone := make(chan struct{})
			go func(done <-chan struct{}) {
				select {
				case <-done:
					_ = conn.Close()
				case <-connectionDone:
				}
			}(s.distributedRunDone(hubSessionID))
			for s.distributedRunActive(hubSessionID) {
				_, data, readErr := conn.ReadMessage()
				if readErr != nil {
					break
				}
				var event RPCEvent
				if json.Unmarshal(data, &event) != nil {
					continue
				}
				if id, ok := event["_daemonEventId"].(float64); ok && id > float64(cursor) {
					cursor = uint64(id)
				}
				eventType, _ := event["type"].(string)
				if eventType == "agent_start" {
					seenStart = true
				}
				if seenStart && (eventType == "agent_end" || eventType == "agent_settled") {
					_ = conn.Close()
					s.releaseDistributedRun(hubSessionID)
					return
				}
			}
			close(connectionDone)
			_ = conn.Close()
		}
	}()
}
