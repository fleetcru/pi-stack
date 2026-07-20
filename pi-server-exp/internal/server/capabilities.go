package server

import "net/http"

const APIVersion = "0.2.0"

// Required desktop capability flags for pi-desk Phase 0 negotiation.
var serverCapabilities = map[string]bool{
	"workerCrud":       true,
	"sessionInventory": true,
	"fileContent":      true,
	"webSocketTickets": true,
}

func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"apiVersion":   APIVersion,
		"capabilities": serverCapabilities,
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"apiVersion":   APIVersion,
		"capabilities": serverCapabilities,
		"sessions":     s.sessions.List(),
		"capacity":     s.workerCapacity(),
	})
}
