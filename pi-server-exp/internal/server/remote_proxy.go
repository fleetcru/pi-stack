package server

import (
	"bytes"
	"encoding/json"
	"io"
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
	admitted := r.Method == http.MethodPost && (action == "prompt" || ((action == "command" || action == "send") && remoteBodyStartsPrompt(r)))
	cursor := uint64(0)
	if admitted {
		var cursorOK bool
		cursor, cursorOK = s.remoteEventCursor(worker, record.WorkerSessionID)
		if !cursorOK {
			writeErrorText(w, http.StatusBadGateway, "worker lifecycle cursor is unavailable")
			return true
		}
	}
	if admitted && !s.acquireDistributedRun(r.Context(), id, worker.ID) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "hub run capacity is busy or this session already has an active run", "scheduler": s.admission.Snapshot()})
		return true
	}
	if admitted {
		s.setDistributedRunMetadata(id, "remote", record.WorkerSessionID)
	}
	ok = s.proxyWorker(w, r, worker, path)
	if admitted {
		if ok {
			s.subscribeRemoteRun(worker, record.WorkerSessionID, id, cursor, true)
		} else {
			s.releaseDistributedRun(id)
		}
	}
	return true
}

func remoteBodyStartsPrompt(r *http.Request) bool {
	body, err := io.ReadAll(io.LimitReader(r.Body, (8<<20)+1))
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || len(body) > 1<<20 {
		return false
	}
	var command RPCCommand
	return json.Unmarshal(body, &command) == nil && command["type"] == "prompt"
}
