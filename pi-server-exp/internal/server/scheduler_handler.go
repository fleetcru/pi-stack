package server

import "net/http"

// schedulerStatus exposes admission pressure for queue indicators and capacity
// diagnostics without leaking prompt contents.
func (s *Server) schedulerStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"admission": s.admission.Snapshot(),
		"workers":   s.workers.List(),
	})
}
