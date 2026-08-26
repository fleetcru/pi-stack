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
	// CreateWorktree auto-creates an isolated git worktree (with its own feature
	// branch) for this session and sets cwd to it. Cannot be combined with an
	// explicit WorktreePath.
	CreateWorktree *createWorktreeOptions `json:"createWorktree"`
}

// createWorktreeOptions controls automatic per-session worktree isolation. When
// Enabled, the session runs in a fresh feature branch in an isolated directory,
// so agent edits never touch the working tree the developer is standing in.
type createWorktreeOptions struct {
	Enabled bool `json:"enabled"`
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
	p.onMessageEnd = func() {
		s.invalidateHistoryCache(spec.ID)
		s.linkManagedSession(spec)
	}
	// AddIfCapacity atomically checks the session limit and adds under one
	// lock, eliminating the TOCTOU race between ActiveCount() and Add().
	maxSessions := 0
	if req.Start {
		maxSessions = int(s.maxSessionsAtomicValue())
	}
	if err := s.sessions.AddIfCapacity(p, spec, maxSessions); err != nil {
		s.rollbackCreatedSession(spec)
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if req.Start {
		ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
		defer cancel()
		if err := p.Start(ctx); err != nil {
			_ = s.sessions.Delete(spec.ID)
			s.rollbackCreatedSession(spec)
			writeError(w, http.StatusBadGateway, err)
			return
		}
	}
	// Link immediately, then the message-end callback refreshes the link once
	// Pi creates its first JSONL file.
	go s.linkManagedSession(spec)
	status := "created"
	if req.Start {
		status = "running"
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": spec.ID, "cwd": spec.CWD, "args": spec.Args, "managed": true, "status": status, "ws": "/v1/sessions/" + spec.ID + "/ws"})
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
		if s.proxyWorker(w, r, worker, "/v1/sessions/"+record.WorkerSessionID) {
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
	if err := p.Close(ctx); err != nil {
		writeError(w, http.StatusGatewayTimeout, fmt.Errorf("stop session: %w", err))
		return
	}
	s.stopWatcher(id)
	if err := s.sessions.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := removeEventJournal(s.cfg.DataDir, id); err != nil {
		s.logger.Warn("failed to remove session event journal", "session", id, "error", err)
	}
	// Remove session bridge symlink from Pi's native session store.
	if s.sessionBridge != nil {
		s.sessionBridge.UnlinkManagedSession(id, spec.CWD)
	}
	if err := s.removeManagedSessionDir(spec.ManagedSessionDir); err != nil {
		// The daemon record is already gone. Keep deletion successful while
		// reporting filesystem cleanup failure for operational follow-up.
		s.logger.Warn("failed to remove managed Pi session directory", "session", id, "error", err)
	}
	if spec.WorktreePath != "" {
		if err := s.removeOwnedWorktree(spec); err != nil {
			s.logger.Warn("failed to remove session worktree", "session", id, "worktree", spec.WorktreePath, "error", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// removeOwnedWorktree removes a linked worktree recorded on a SessionSpec. It
// only proceeds when the worktree is genuinely registered with the session's
// repository, so we never delete a directory the server does not own.
func (s *Server) rollbackCreatedSession(spec SessionSpec) {
	if err := s.removeManagedSessionDir(spec.ManagedSessionDir); err != nil {
		s.logger.Warn("failed to roll back managed session directory", "session", spec.ID, "error", err)
	}
	if spec.WorktreePath != "" {
		if err := s.removeOwnedWorktree(spec); err != nil {
			s.logger.Warn("failed to roll back session worktree", "session", spec.ID, "error", err)
		}
	}
}

func (s *Server) removeOwnedWorktree(spec SessionSpec) error {
	known, err := s.gitWorktrees(context.Background(), spec.CWD)
	if err != nil {
		return err
	}
	match := ""
	for _, item := range known {
		itemAbs, _ := filepath.Abs(filepath.Clean(item.Path))
		if filepath.Clean(itemAbs) == filepath.Clean(spec.WorktreePath) {
			match = itemAbs
			break
		}
	}
	if match == "" {
		return nil // not registered as a worktree we own; leave it alone
	}
	if _, err := os.Stat(match); err != nil {
		// Directory already gone; just prune the git bookkeeping.
		_, _ = s.runGit(context.Background(), spec.CWD, "worktree", "prune")
		return nil
	}
	if _, err := s.runGit(context.Background(), spec.CWD, "worktree", "remove", match); err != nil {
		// On Git for Windows, worktree metadata files carry read-only attributes and
		// `git worktree remove` can fail with "Permission denied" even on a clean
		// checkout. Fall back to a forced filesystem cleanup, then prune + delete
		// the now-orphaned branch.
		s.clearReadOnlyTree(match)
		if rmErr := os.RemoveAll(match); rmErr != nil {
			return rmErr
		}
		_, _ = s.runGit(context.Background(), spec.CWD, "worktree", "prune")
		if branch := spec.Metadata["worktreeBranch"]; branch != "" {
			_, _ = s.runGit(context.Background(), spec.CWD, "branch", "-D", branch)
		}
	}
	_, _ = s.runGit(context.Background(), spec.CWD, "worktree", "prune")
	return nil
}

// clearReadOnlyTree recursively clears the read-only attribute so os.RemoveAll
// can delete a worktree's metadata on Windows.
func (s *Server) clearReadOnlyTree(root string) {
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type().IsRegular() {
			_ = os.Chmod(path, 0o644)
		}
		return nil
	})
	_ = err
}

func (s *Server) linkManagedSession(spec SessionSpec) {
	if s.sessionBridge == nil || spec.ManagedSessionDir == "" {
		return
	}
	if err := s.sessionBridge.LinkManagedSession(spec.ID, spec.ManagedSessionDir, spec.CWD); err != nil {
		s.logger.Warn("failed to bridge session to pi -r", "session", spec.ID, "error", err)
	}
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
	id := req.ID
	if id == "" {
		id = NewSessionID()
	} else if !validSessionID(id) {
		return SessionSpec{}, fmt.Errorf("session id may contain only letters, numbers, hyphens, and underscores")
	}
	worktreePath := ""
	if req.CreateWorktree != nil && req.CreateWorktree.Enabled {
		if req.WorktreePath != "" {
			return SessionSpec{}, fmt.Errorf("createWorktree cannot be combined with an explicit worktreePath")
		}
		// Auto-create an isolated worktree + feature branch so agent edits never
		// touch the primary working tree. Title (or session id) seeds the branch.
		createdPath, branchName, err := s.createAutoWorktree(context.Background(), cwd, req.Title)
		if err != nil {
			return SessionSpec{}, fmt.Errorf("createWorktree: %w", err)
		}
		worktreePath = createdPath
		cwd = createdPath
		if req.Metadata == nil {
			req.Metadata = map[string]string{}
		}
		req.Metadata["worktreeBranch"] = branchName
	} else if req.WorktreePath != "" {
		worktreePath, err = s.validateWorktreePath(cwd, req.WorktreePath, true)
		if err != nil {
			return SessionSpec{}, err
		}
		cwd = worktreePath
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
	p.onMessageEnd = func() {
		s.invalidateHistoryCache(id)
		s.linkManagedSession(spec)
	}
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
