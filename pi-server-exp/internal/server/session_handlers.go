package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type createSessionRequest struct {
	ID           string            `json:"id"`
	CWD          string            `json:"cwd"`
	Args         []string          `json:"args"`
	Env          map[string]string `json:"env"`
	SessionPath  string            `json:"sessionPath"`
	Start        bool              `json:"start"`
	Restart      bool              `json:"restart"`
	Project      string            `json:"project"`
	Title        string            `json:"title"`
	TaskType     string            `json:"taskType"`
	Owner        string            `json:"owner"`
	Labels       []string          `json:"labels"`
	Metadata     map[string]string `json:"metadata"`
	WorktreePath string            `json:"worktreePath"`
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && r.Body != http.NoBody {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	spec, err := s.buildSessionSpec(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	p := NewPiProcess(spec, s.cfg, s.logger)
	p.onMessageEnd = func() { s.invalidateHistoryCache(spec.ID) }
	// AddIfCapacity atomically checks the session limit and adds under one
	// lock, eliminating the TOCTOU race between ActiveCount() and Add().
	maxSessions := 0
	if req.Start {
		maxSessions = s.cfg.MaxSessions
	}
	if err := s.sessions.AddIfCapacity(p, spec, maxSessions); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if req.Start {
		ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
		defer cancel()
		if err := p.Start(ctx); err != nil {
			_ = s.sessions.Delete(spec.ID)
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	// Bridge managed session to Pi's native session store so pi -r can discover it.
	if s.sessionBridge != nil && spec.ManagedSessionDir != "" {
		go func() {
			if err := s.sessionBridge.LinkManagedSession(spec.ID, spec.ManagedSessionDir); err != nil {
				s.logger.Warn("failed to bridge session to pi -r", "session", spec.ID, "error", err)
			}
		}()
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": spec.ID, "cwd": spec.CWD, "args": spec.Args, "managed": true, "status": "running", "ws": "/v1/sessions/" + spec.ID + "/ws"})
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	id, rest := splitSessionPath(r.URL.Path)
	if strings.HasPrefix(rest, "git/") {
		s.gitHandler(w, r)
		return
	}
	if rest != "" {
		http.NotFound(w, r)
		return
	}
	if record, remote := s.remoteSessions.Get(id); remote {
		worker, ok := s.workers.Get(record.WorkerID)
		if !ok {
			writeErrorText(w, http.StatusBadGateway, "mapped worker no longer exists")
			return
		}
		s.proxyWorker(w, r, worker, "/v1/sessions/"+record.WorkerSessionID)
		if r.Context().Err() == nil {
			_ = s.remoteSessions.Delete(id)
		}
		return
	}
	spec, _ := s.sessions.GetSpec(id)
	p, ok := s.getSession(id)
	if !ok {
		writeErrorText(w, http.StatusNotFound, "session not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.ShutdownTimeout)
	defer cancel()
	_ = p.Close(ctx)
	s.stopWatcher(id)
	if err := s.sessions.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// Remove session bridge symlink from Pi's native session store.
	if s.sessionBridge != nil {
		s.sessionBridge.UnlinkManagedSession(id)
	}
	if err := s.removeManagedSessionDir(spec.ManagedSessionDir); err != nil {
		// The daemon record is already gone. Keep deletion successful while
		// reporting filesystem cleanup failure for operational follow-up.
		s.logger.Warn("failed to remove managed Pi session directory", "session", id, "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (s *Server) removeManagedSessionDir(dir string) error {
	if dir == "" {
		return nil
	}
	root, err := filepath.Abs(filepath.Join(s.cfg.DataDir, "pi-sessions"))
	if err != nil {
		return err
	}
	target, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to remove unmanaged session directory")
	}
	return os.RemoveAll(target)
}

func (s *Server) buildSessionSpec(req createSessionRequest) (SessionSpec, error) {
	cwd, err := s.allowedCWD(req.CWD)
	if err != nil {
		return SessionSpec{}, err
	}
	worktreePath := ""
	if req.WorktreePath != "" {
		worktreePath, err = s.validateWorktreePath(cwd, req.WorktreePath, true)
		if err != nil {
			return SessionSpec{}, err
		}
		cwd = worktreePath
	}
	id := req.ID
	if id == "" {
		id = NewSessionID()
	} else if !validSessionID(id) {
		return SessionSpec{}, fmt.Errorf("session id may contain only letters, numbers, hyphens, and underscores")
	}
	args := append([]string{}, req.Args...)
	managedSessionDir := ""
	if req.SessionPath != "" {
		args = append(args, "--session", req.SessionPath)
	} else {
		// Each daemon session needs an isolated Pi session store. Track this
		// directory explicitly so DELETE can safely erase only history created
		// and owned by this server, never a user-supplied --session file.
		managedSessionDir = filepath.Join(s.cfg.DataDir, "pi-sessions", id)
		args = append(args, "--session-dir", managedSessionDir)
	}
	return SessionSpec{ID: id, CWD: cwd, Args: args, Env: req.Env, SessionPath: req.SessionPath, ManagedSessionDir: managedSessionDir, WorktreePath: worktreePath, Managed: true, Transport: "rpc", Restart: req.Restart, Status: "created", Project: req.Project, Title: req.Title, TaskType: req.TaskType, Owner: req.Owner, Labels: req.Labels, Metadata: req.Metadata}, nil
}

func (s *Server) getSession(id string) (*PiProcess, bool) {
	if p, ok := s.sessions.Get(id); ok {
		return p, true
	}
	spec, ok := s.sessions.GetSpec(id)
	if !ok || spec.Transport == "relay" {
		return nil, false
	}
	p := NewPiProcess(spec, s.cfg, s.logger)
	p.onMessageEnd = func() { s.invalidateHistoryCache(id) }
	registered, isNew := s.sessions.AttachIfAbsent(p)
	if !isNew {
		// Another goroutine registered a process between our Get miss and
		// AttachIfAbsent. Close the redundant one to avoid leaking the
		// underlying OS process and goroutines.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Close(ctx)
	}
	return registered, true
}
