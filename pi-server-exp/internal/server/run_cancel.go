package server

import (
	"net/http"
	"strings"
)

// cancelRun removes queued scheduler work or aborts the active target session.
// Queued runs are dropped from admission; active runs are aborted in the
// owning session so the normal agent_settled admission release path runs.
func (s *Server) cancelRun(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/runs/"), "/cancel")
	if runID == "" || strings.Contains(runID, "/") {
		http.NotFound(w, r)
		return
	}
	if s.admission.CancelQueued(runID) {
		writeJSON(w, http.StatusOK, map[string]any{"runId": runID, "result": "cancelled"})
		return
	}
	sessionID, workerID, ok := s.admission.ActiveRunTarget(runID)
	if !ok {
		writeErrorCode(w, r, http.StatusNotFound, CodeNotFound, "unknown run")
		return
	}
	// Only locally managed sessions can be aborted directly from the hub.
	p, exists := s.sessions.Get(sessionID)
	if !exists {
		writeErrorCode(w, r, http.StatusConflict, CodeConflict, "active run is not locally abortable")
		return
	}
	if err := p.Send(RPCCommand{"type": "abort"}); err != nil {
		writeErrorCode(w, r, http.StatusBadGateway, CodeBadGateway, "abort failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runId": runID, "sessionId": sessionID, "workerId": workerID, "result": "aborting"})
}
