package server

import (
	"net/http"
	"strings"
)

func (s *Server) proxyRemoteSession(w http.ResponseWriter, r *http.Request, id, action string) bool {
	record, ok := s.remoteSessions.Get(id)
	if !ok {
		return false
	}
	worker, ok := s.workers.Get(record.WorkerID)
	if !ok {
		writeErrorText(w, http.StatusBadGateway, "mapped worker no longer exists")
		return true
	}
	// Prevent path traversal: reject actions containing ".." so the proxy
	// cannot be used to reach arbitrary endpoints on the remote worker.
	if strings.Contains(action, "..") {
		http.NotFound(w, r)
		return true
	}
	path := "/v1/sessions/" + record.WorkerSessionID
	if action != "" {
		path += "/" + action
	}
	if action == "ws" && r.Method == http.MethodGet {
		s.proxyWorkerWebSocket(w, r, worker, path)
		return true
	}
	s.proxyWorker(w, r, worker, path)
	return true
}
