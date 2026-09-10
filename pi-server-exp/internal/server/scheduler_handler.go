package server

import "net/http"

// schedulerStatus exposes admission pressure for queue indicators and capacity
// diagnostics without leaking prompt contents. Detailed run records let
// clients observe queued position and active occupancy.
func (s *Server) schedulerStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"admission": s.admission.Snapshot(),
		"runs":      s.admission.DetailedRuns(),
		"workers":   s.workers.List(),
	})
}
