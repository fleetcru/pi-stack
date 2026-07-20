package server

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// GlobalSessionSummary identifies the origin of every session visible to a
// coordinator. Its ID is stable for inventory purposes; attach returns the
// normal coordinator session ID used by existing REST/WS routes.
type GlobalSessionSummary struct {
	ID        string         `json:"id"`
	OriginID  string         `json:"originId"`
	WorkerID  string         `json:"workerId"`
	Session   SessionSummary `json:"session"`
	Reachable bool           `json:"reachable"`
}

func globalSessionID(workerID, sessionID string) string { return workerID + ":" + sessionID }

func (s *Server) listGlobalSessions(w http.ResponseWriter, r *http.Request) {
	items := make([]GlobalSessionSummary, 0)
	for _, spec := range s.sessions.ListSpecs() {
		sum := localSummaryFromSpec(spec)
		items = append(items, GlobalSessionSummary{ID: globalSessionID("local", spec.ID), OriginID: spec.ID, WorkerID: "local", Session: sum, Reachable: true})
	}
	workers := s.workers.List()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	failures := make([]partialFailure, 0)
	for _, worker := range workers {
		if worker.ID == "local" {
			continue
		}
		wg.Add(1)
		go func(worker Worker) {
			defer wg.Done()
			summaries, err := s.fetchRemoteWorkerSessionSummaries(ctx, worker)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, partialFailure{WorkerID: worker.ID, Error: "failed to load worker session inventory", Code: CodeBadGateway})
				return
			}
			for _, sum := range summaries {
				sum.WorkerID = worker.ID
				items = append(items, GlobalSessionSummary{ID: globalSessionID(worker.ID, sum.ID), OriginID: sum.ID, WorkerID: worker.ID, Session: sum, Reachable: true})
			}
		}(worker)
	}
	wg.Wait()
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	writeJSON(w, http.StatusOK, map[string]any{"sessions": items, "partialFailures": failures})
}

// attachGlobalSession creates (or reuses) a coordinator remote mapping so all
// existing /v1/sessions/{id} endpoints and ticketed WS streams work unchanged.
func (s *Server) attachGlobalSession(w http.ResponseWriter, r *http.Request, globalID string) {
	parts := strings.SplitN(globalID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "invalid global session id")
		return
	}
	if parts[0] == "local" {
		if _, ok := s.sessions.GetSpec(parts[1]); !ok {
			writeErrorCode(w, r, http.StatusNotFound, CodeSessionNotFound, "session not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": parts[1], "attached": false})
		return
	}
	worker, ok := s.workers.Get(parts[0])
	if !ok {
		writeErrorCode(w, r, http.StatusNotFound, CodeWorkerNotFound, "worker not found")
		return
	}
	for _, rec := range s.remoteSessions.List() {
		if rec.WorkerID == worker.ID && rec.WorkerSessionID == parts[1] {
			writeJSON(w, http.StatusOK, map[string]any{"id": rec.ID, "attached": false})
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if _, err := s.fetchRemoteSessionSummary(ctx, worker, parts[1]); err != nil {
		writeErrorCode(w, r, http.StatusBadGateway, CodeBadGateway, "failed to resolve global session")
		return
	}
	rec := RemoteSession{ID: NewSessionID(), WorkerID: worker.ID, WorkerSessionID: parts[1], CreatedAt: time.Now().UTC()}
	if err := s.remoteSessions.Add(rec); err != nil {
		writeErrorCode(w, r, http.StatusInternalServerError, CodeInternal, "failed to save global session mapping")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": rec.ID, "attached": true})
}
