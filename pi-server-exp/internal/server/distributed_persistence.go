package server

import (
	"context"
	"encoding/json"
	"os"
	"time"
)

func (s *Server) persistDistributedRuns() {
	if s.distributedPath == "" {
		return
	}
	s.distributedPersistMu.Lock()
	defer s.distributedPersistMu.Unlock()
	s.distributedMu.Lock()
	snapshot := make(map[string]distributedReservation, len(s.distributedRuns))
	for id, reservation := range s.distributedRuns {
		copy := reservation
		copy.timer = nil
		snapshot[id] = copy
	}
	s.distributedMu.Unlock()
	if err := writeJSONAtomic(s.distributedPath, snapshot); err != nil {
		s.logger.Warn("failed to persist distributed reservations", "error", err)
	}
}

func (s *Server) restoreDistributedRuns() {
	data, err := os.ReadFile(s.distributedPath)
	if err != nil && !os.IsNotExist(err) {
		s.logger.Warn("failed to read distributed reservations", "error", err)
		return
	}
	persisted := map[string]distributedReservation{}
	if len(data) > 0 && json.Unmarshal(data, &persisted) != nil {
		s.logger.Warn("failed to decode distributed reservations")
		return
	}
	now := time.Now()
	for sessionID, reservation := range persisted {
		if !reservation.ExpiresAt.IsZero() && !reservation.ExpiresAt.After(now) {
			continue
		}
		if reservation.Kind == "remote" {
			if _, ok := s.workers.Get(reservation.WorkerID); !ok {
				continue
			}
		}
		if !s.admission.TryAcquire(sessionID, reservation.WorkerID) {
			continue
		}
		reservation.done = make(chan struct{})
		if !reservation.ExpiresAt.IsZero() {
			remaining := time.Until(reservation.ExpiresAt)
			reservation.timer = time.AfterFunc(remaining, func(id string) func() { return func() { s.releaseDistributedRun(id) } }(sessionID))
		}
		s.distributedMu.Lock()
		s.distributedRuns[sessionID] = reservation
		s.distributedMu.Unlock()
		if reservation.Kind == "remote" {
			if worker, ok := s.workers.Get(reservation.WorkerID); ok {
				go func(worker Worker, remoteID, hubID string) {
					state, running, reachable := s.remoteRuntimeState(worker, remoteID)
					if reachable && (!running || state == "idle" || state == "stopped" || state == "failed") {
						s.releaseDistributedRun(hubID)
						return
					}
					s.subscribeRemoteRunWhenAvailable(worker, remoteID, hubID, false)
				}(worker, reservation.RemoteSessionID, sessionID)
			}
		}
	}
	s.persistDistributedRuns()
	go s.reconstructMappedRemoteRuns()
}

func (s *Server) subscribeRemoteRunWhenAvailable(worker Worker, remoteID, hubID string, requireStart bool) {
	for s.distributedRunActive(hubID) {
		if cursor, ok := s.remoteEventCursor(worker, remoteID); ok {
			s.subscribeRemoteRun(worker, remoteID, hubID, cursor, requireStart)
			return
		}
		time.Sleep(time.Second)
	}
}

func (s *Server) reconstructMappedRemoteRuns() {
	s.distributedMu.Lock()
	if s.distributedReconstructing {
		s.distributedMu.Unlock()
		return
	}
	s.distributedReconstructing = true
	s.distributedMu.Unlock()
	defer func() { s.distributedMu.Lock(); s.distributedReconstructing = false; s.distributedMu.Unlock() }()
	// Reconstruct mapped worker runs that started outside this hub or were not
	// persisted before a crash. Worker runtime state is authoritative.
	for _, mapped := range s.remoteSessions.List() {
		if s.distributedRunActive(mapped.ID) {
			continue
		}
		worker, ok := s.workers.Get(mapped.WorkerID)
		if !ok {
			continue
		}
		state, running, reachable := s.remoteRuntimeState(worker, mapped.WorkerSessionID)
		if !reachable || !running || (state != "working" && state != "waiting_for_input" && state != "starting") {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		acquired := s.acquireDistributedRun(ctx, mapped.ID, mapped.WorkerID)
		cancel()
		if !acquired {
			continue
		}
		s.setDistributedRunMetadata(mapped.ID, "remote", mapped.WorkerSessionID)
		s.subscribeRemoteRunWhenAvailable(worker, mapped.WorkerSessionID, mapped.ID, false)
	}
}
