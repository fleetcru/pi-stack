package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SensitiveString redacts its value on String()/GoString() to prevent
// accidental token leakage in logs, fmt.Sprintf, or debug output.
type SensitiveString string

func (s SensitiveString) String() string  { return "***" }
func (s SensitiveString) GoString() string { return "***" }

type Worker struct {
	ID             string    `json:"id"`
	URL            string    `json:"url"`
	Token          SensitiveString `json:"-"`
	Tags           []string  `json:"tags,omitempty"`
	Status         string    `json:"status,omitempty"`
	LastHeartbeat  time.Time `json:"lastHeartbeat,omitempty"`
	ActiveSessions int       `json:"activeSessions,omitempty"`
	MaxSessions    int       `json:"maxSessions,omitempty"`
}

type persistedWorker struct {
	ID    string   `json:"id"`
	URL   string   `json:"url"`
	Token string   `json:"token,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}
type WorkerRegistry struct {
	mu      sync.RWMutex
	path    string
	workers map[string]Worker
}

func NewWorkerRegistry(path string) *WorkerRegistry {
	return &WorkerRegistry{path: path, workers: map[string]Worker{"local": {ID: "local", URL: "local", Tags: []string{"local"}}}}
}
func (r *WorkerRegistry) Load() error {
	if r.path == "" {
		return nil
	}
	b, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var workers []persistedWorker
	if err := json.Unmarshal(b, &workers); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, w := range workers {
		if w.ID != "" {
			r.workers[w.ID] = Worker{ID: w.ID, URL: w.URL, Token: SensitiveString(w.Token), Tags: w.Tags}
		}
	}
	return nil
}
func (r *WorkerRegistry) Save() error {
	if r.path == "" {
		return nil
	}
	r.mu.RLock()
	workers := make([]persistedWorker, 0, len(r.workers))
	for _, w := range r.workers {
		if w.ID != "local" {
			workers = append(workers, persistedWorker{ID: w.ID, URL: w.URL, Token: string(w.Token), Tags: w.Tags})
		}
	}
	r.mu.RUnlock()
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(workers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, b, 0o600)
}
func (r *WorkerRegistry) Add(w Worker) error {
	r.mu.Lock()
	r.workers[w.ID] = w
	r.mu.Unlock()
	return r.Save()
}
func (r *WorkerRegistry) List() []Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Worker, 0, len(r.workers))
	for _, w := range r.workers {
		out = append(out, w)
	}
	return out
}
func (r *WorkerRegistry) UpdateCapacity(id string, active, max int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workers[id]
	if ok {
		w.ActiveSessions = active
		w.MaxSessions = max
		r.workers[id] = w
	}
}

func (r *WorkerRegistry) Heartbeat(id string, healthy bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.workers[id]
	if ok {
		w.LastHeartbeat = time.Now().UTC()
		if healthy {
			w.Status = "healthy"
		} else {
			w.Status = "unhealthy"
		}
		r.workers[id] = w
	}
}

func (r *WorkerRegistry) Get(id string) (Worker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.workers[id]
	return w, ok
}

func (r *WorkerRegistry) Update(w Worker) error {
	r.mu.Lock()
	r.workers[w.ID] = w
	r.mu.Unlock()
	return r.Save()
}

func (r *WorkerRegistry) Delete(id string) error {
	r.mu.Lock()
	delete(r.workers, id)
	r.mu.Unlock()
	return r.Save()
}

func publicWorker(w Worker) map[string]any {
	return map[string]any{
		"id":             w.ID,
		"url":            w.URL,
		"tags":           w.Tags,
		"status":         w.Status,
		"lastHeartbeat":  w.LastHeartbeat,
		"activeSessions": w.ActiveSessions,
		"maxSessions":    w.MaxSessions,
	}
}

func (s *Server) workerCapacity() map[string]any {
	return map[string]any{"activeSessions": s.sessions.ActiveCount(), "maxSessions": s.cfg.MaxSessions}
}

func (s *Server) updateCapacity(w http.ResponseWriter, r *http.Request) {
	var input struct {
		MaxSessions int `json:"maxSessions"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeErrorText(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if input.MaxSessions < 0 {
		writeErrorText(w, http.StatusBadRequest, "maxSessions must be >= 0 (0 = unlimited)")
		return
	}
	s.cfg.MaxSessions = input.MaxSessions
	s.sessions.mu.Lock()
	s.sessions.maxSessions = input.MaxSessions
	s.sessions.mu.Unlock()
	writeJSON(w, http.StatusOK, s.workerCapacity())
}

func (s *Server) validateWorkerURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("worker url must be absolute http(s)")
	}
	if len(s.cfg.AllowedWorkerHosts) > 0 && !hostAllowed(u.Hostname(), s.cfg.AllowedWorkerHosts) {
		return nil, errors.New("worker host not allowed")
	}
	return u, nil
}

func (s *Server) listWorkers(w http.ResponseWriter, _ *http.Request) {
	workers := s.workers.List()
	out := make([]map[string]any, 0, len(workers))
	for _, worker := range workers {
		out = append(out, publicWorker(worker))
	}
	writeJSON(w, http.StatusOK, map[string]any{"workers": out})
}

func (s *Server) addWorker(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID    string   `json:"id"`
		URL   string   `json:"url"`
		Token string   `json:"token"`
		Tags  []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "invalid request body")
		return
	}
	if input.ID == "" {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "worker id required")
		return
	}
	if input.ID == "local" {
		writeErrorCode(w, r, http.StatusForbidden, CodeLocalWorkerImmutable, "local worker cannot be modified")
		return
	}
	if _, err := s.validateWorkerURL(input.URL); err != nil {
		code := CodeBadRequest
		if err.Error() == "worker host not allowed" {
			code = CodeWorkerHostNotAllowed
			writeErrorCode(w, r, http.StatusForbidden, code, err.Error())
			return
		}
		writeErrorCode(w, r, http.StatusBadRequest, code, err.Error())
		return
	}
	worker := Worker{ID: input.ID, URL: input.URL, Token: SensitiveString(input.Token), Tags: input.Tags}
	if err := s.workers.Add(worker); err != nil {
		writeErrorCode(w, r, http.StatusInternalServerError, CodeInternal, "failed to save worker")
		return
	}
	writeJSON(w, http.StatusCreated, publicWorker(worker))
}

func (s *Server) getWorkerByID(w http.ResponseWriter, r *http.Request, id string) {
	worker, ok := s.workers.Get(id)
	if !ok {
		writeErrorCode(w, r, http.StatusNotFound, CodeWorkerNotFound, "worker not found")
		return
	}
	writeJSON(w, http.StatusOK, publicWorker(worker))
}

func (s *Server) workerPut(w http.ResponseWriter, r *http.Request) {
	wp := splitWorkerPath(r.URL.Path)
	if wp.rest != "" {
		http.NotFound(w, r)
		return
	}
	if wp.id == "local" {
		writeErrorCode(w, r, http.StatusForbidden, CodeLocalWorkerImmutable, "local worker cannot be modified")
		return
	}
	existing, ok := s.workers.Get(wp.id)
	if !ok {
		writeErrorCode(w, r, http.StatusNotFound, CodeWorkerNotFound, "worker not found")
		return
	}
	var input struct {
		ID    string          `json:"id"`
		URL   string          `json:"url"`
		Token json.RawMessage `json:"token"`
		Tags  []string        `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "invalid request body")
		return
	}
	if input.ID != "" && input.ID != wp.id {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "worker id path/body mismatch")
		return
	}
	if input.URL == "" {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "worker url required")
		return
	}
	if _, err := s.validateWorkerURL(input.URL); err != nil {
		if err.Error() == "worker host not allowed" {
			writeErrorCode(w, r, http.StatusForbidden, CodeWorkerHostNotAllowed, err.Error())
			return
		}
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, err.Error())
		return
	}
	updated := existing
	updated.URL = input.URL
	updated.Tags = input.Tags
	// token omitted => preserve; token: null => clear; token: "..." => set
	if len(input.Token) > 0 {
		if string(input.Token) == "null" {
			updated.Token = ""
		} else {
			var token string
			if err := json.Unmarshal(input.Token, &token); err != nil {
				writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "token must be string or null")
				return
			}
			updated.Token = SensitiveString(token)
		}
	}
	if err := s.workers.Update(updated); err != nil {
		writeErrorCode(w, r, http.StatusInternalServerError, CodeInternal, "failed to save worker")
		return
	}
	writeJSON(w, http.StatusOK, publicWorker(updated))
}

func (s *Server) workerDelete(w http.ResponseWriter, r *http.Request) {
	wp := splitWorkerPath(r.URL.Path)
	if wp.rest != "" {
		http.NotFound(w, r)
		return
	}
	if wp.id == "local" {
		writeErrorCode(w, r, http.StatusForbidden, CodeLocalWorkerImmutable, "local worker cannot be deleted")
		return
	}
	if _, ok := s.workers.Get(wp.id); !ok {
		writeErrorCode(w, r, http.StatusNotFound, CodeWorkerNotFound, "worker not found")
		return
	}
	force := r.URL.Query().Get("force") == "true"
	mapped := s.remoteSessionsForWorker(wp.id)
	if len(mapped) > 0 && !force {
		writeErrorCode(w, r, http.StatusConflict, CodeWorkerHasActive, "worker has active session mappings")
		return
	}
	failedCleanup := []string{}
	if force && len(mapped) > 0 {
		worker, _ := s.workers.Get(wp.id)
		for _, rec := range mapped {
			if err := s.cleanupRemoteSession(r.Context(), worker, rec); err != nil {
				failedCleanup = append(failedCleanup, rec.ID)
			}
			_ = s.remoteSessions.Delete(rec.ID)
		}
	}
	if err := s.workers.Delete(wp.id); err != nil {
		writeErrorCode(w, r, http.StatusInternalServerError, CodeInternal, "failed to delete worker")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deleted":          wp.id,
		"failedCleanupIds": failedCleanup,
		"removedMappings":  len(mapped),
	})
}

func (s *Server) remoteSessionsForWorker(workerID string) []RemoteSession {
	all := s.remoteSessions.List()
	out := make([]RemoteSession, 0)
	for _, rec := range all {
		if rec.WorkerID == workerID {
			out = append(out, rec)
		}
	}
	return out
}

func (s *Server) cleanupRemoteSession(ctx context.Context, worker Worker, rec RemoteSession) error {
	base, err := url.Parse(strings.TrimRight(worker.URL, "/"))
	if err != nil {
		return err
	}
	if base.Path == "" {
		base.Path = "/v1/sessions/" + rec.WorkerSessionID
	} else {
		base.Path = strings.TrimRight(base.Path, "/") + "/v1/sessions/" + rec.WorkerSessionID
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base.String(), nil)
	if err != nil {
		return err
	}
	if string(worker.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+string(worker.Token))
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode != http.StatusNotFound {
		return errors.New("remote delete failed")
	}
	return nil
}

func (s *Server) probeWorkerHealth(w http.ResponseWriter, r *http.Request, worker Worker) {
	if worker.ID == "local" {
		s.workers.Heartbeat("local", true)
		s.workers.UpdateCapacity("local", s.sessions.ActiveCount(), s.cfg.MaxSessions)
		worker, _ = s.workers.Get("local")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "worker": publicWorker(worker), "capacity": s.workerCapacity()})
		return
	}
	base, err := url.Parse(strings.TrimRight(worker.URL, "/"))
	if err != nil {
		s.workers.Heartbeat(worker.ID, false)
		writeErrorCode(w, r, http.StatusBadGateway, CodeBadGateway, "invalid worker url")
		return
	}
	if base.Path == "" {
		base.Path = "/healthz"
	} else {
		base.Path = strings.TrimRight(base.Path, "/") + "/healthz"
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, base.String(), nil)
	if err != nil {
		s.workers.Heartbeat(worker.ID, false)
		writeErrorCode(w, r, http.StatusBadGateway, CodeBadGateway, "failed to probe worker")
		return
	}
	if string(worker.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+string(worker.Token))
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.workers.Heartbeat(worker.ID, false)
		writeErrorCode(w, r, http.StatusBadGateway, CodeBadGateway, "worker health probe failed")
		return
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	healthy := resp.StatusCode >= 200 && resp.StatusCode < 300
	s.workers.Heartbeat(worker.ID, healthy)
	if healthy {
		if capRaw, ok := body["capacity"].(map[string]any); ok {
			active, _ := capRaw["activeSessions"].(float64)
			max, _ := capRaw["maxSessions"].(float64)
			s.workers.UpdateCapacity(worker.ID, int(active), int(max))
		}
	}
	worker, _ = s.workers.Get(worker.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     healthy,
		"worker": publicWorker(worker),
		"status": resp.StatusCode,
	})
}

func (s *Server) workerGet(w http.ResponseWriter, r *http.Request) {
	wp := splitWorkerPath(r.URL.Path)
	worker, ok := s.workers.Get(wp.id)
	if !ok {
		writeErrorCode(w, r, http.StatusNotFound, CodeWorkerNotFound, "worker not found")
		return
	}
	if wp.rest == "" {
		s.getWorkerByID(w, r, wp.id)
		return
	}
	if wp.rest == "health" {
		s.probeWorkerHealth(w, r, worker)
		return
	}
	if strings.HasPrefix(wp.rest, "sessions/") {
		if worker.ID == "local" {
			http.NotFound(w, r)
			return
		}
		remotePath := "/v1/" + wp.rest
		if strings.HasSuffix(wp.rest, "/ws") {
			s.proxyWorkerWebSocket(w, r, worker, remotePath)
			return
		}
		s.proxyWorker(w, r, worker, remotePath)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) workerPost(w http.ResponseWriter, r *http.Request) {
	wp := splitWorkerPath(r.URL.Path)
	worker, ok := s.workers.Get(wp.id)
	if !ok {
		writeErrorCode(w, r, http.StatusNotFound, CodeWorkerNotFound, "worker not found")
		return
	}
	if wp.rest == "health" {
		s.probeWorkerHealth(w, r, worker)
		return
	}
	if wp.rest == "sessions" {
		if worker.ID == "local" {
			s.createSession(w, r)
			return
		}
		s.createRemoteSession(w, r, worker)
		return
	}
	if strings.HasPrefix(wp.rest, "sessions/") {
		if worker.ID == "local" {
			http.NotFound(w, r)
			return
		}
		s.proxyWorker(w, r, worker, "/v1/"+wp.rest)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) proxyWorker(w http.ResponseWriter, r *http.Request, worker Worker, path string) {
	base, err := url.Parse(strings.TrimRight(worker.URL, "/"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	// Preserve any path prefix in the worker URL and append the requested path
	if base.Path == "" {
		base.Path = path
	} else {
		base.Path = strings.TrimRight(base.Path, "/") + path
	}
	// Forward client query (e.g. file path, since cursor) without leaking Authorization.
	base.RawQuery = r.URL.RawQuery
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, err)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, base.String(), bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	if string(worker.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+string(worker.Token))
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	defer resp.Body.Close()
	// Only forward safe response headers from remote workers. Internal
	// headers like Set-Cookie, Server, or trace IDs should not leak to clients.
	forwardedHeaders := []string{"Content-Type", "Content-Length", "Content-Disposition", "X-Request-Id"}
	for _, k := range forwardedHeaders {
		for _, v := range resp.Header.Values(k) {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
