package server

import (
	"net/http"
	"strconv"
)

func (s *Server) daemonStatus(w http.ResponseWriter, r *http.Request) {
	id, rest := splitSessionPath(r.URL.Path)
	if rest != "daemon-status" {
		http.NotFound(w, r)
		return
	}
	p, ok := s.getSession(id)
	if !ok {
		writeErrorText(w, http.StatusNotFound, "session not found")
		return
	}
	status := p.Status()
	if spec, ok := s.sessions.GetSpec(id); ok {
		status["createdAt"] = spec.CreatedAt
		status["updatedAt"] = spec.UpdatedAt
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) eventHistory(w http.ResponseWriter, r *http.Request) {
	id, rest := splitSessionPath(r.URL.Path)
	if rest != "events" {
		http.NotFound(w, r)
		return
	}
	p, ok := s.getSession(id)
	if !ok {
		writeErrorText(w, http.StatusNotFound, "session not found")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 200
	}
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	writeJSON(w, http.StatusOK, map[string]any{"events": p.Events(limit, since), "since": since})
}
