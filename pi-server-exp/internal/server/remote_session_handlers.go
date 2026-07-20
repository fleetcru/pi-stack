package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) listRemoteSessions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sessions": s.remoteSessions.List()})
}
func (s *Server) createRemoteSession(w http.ResponseWriter, r *http.Request, worker Worker) {
	base, err := url.Parse(strings.TrimRight(worker.URL, "/"))
	if err != nil {
		writeError(w, 502, err)
		return
	}
	base.Path = "/v1/sessions"
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, 413, err)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, base.String(), bytes.NewReader(body))
	if err != nil {
		writeError(w, 502, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if string(worker.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+string(worker.Token))
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		writeError(w, 502, err)
		return
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if len(raw) > 1<<20 {
		writeErrorText(w, http.StatusBadGateway, "worker response exceeds 1MB limit")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(raw)
		return
	}
	var remote struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &remote); err != nil || remote.ID == "" {
		writeErrorText(w, 502, "worker returned invalid session response")
		return
	}
	global := NewSessionID()
	record := RemoteSession{ID: global, WorkerID: worker.ID, WorkerSessionID: remote.ID, CreatedAt: time.Now().UTC()}
	if err := s.remoteSessions.Add(record); err != nil {
		writeError(w, 500, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": global, "workerId": worker.ID, "workerSessionId": remote.ID, "workerWs": "/v1/workers/" + worker.ID + "/sessions/" + remote.ID + "/ws"})
}
