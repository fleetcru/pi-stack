package server

import (
	"context"
	"net/http"
	"os/exec"
	"strings"
)

func (s *Server) gitHandler(w http.ResponseWriter, r *http.Request) {
	id, action := splitSessionPath(r.URL.Path)
	if !strings.HasPrefix(action, "git/") {
		http.NotFound(w, r)
		return
	}
	if s.proxyRemoteSession(w, r, id, action) {
		return
	}
	spec, ok := s.sessions.GetSpec(id)
	if !ok {
		writeErrorText(w, http.StatusNotFound, "session not found")
		return
	}
	var args []string
	switch strings.TrimPrefix(action, "git/") {
	case "status":
		args = []string{"status", "--short", "--branch"}
	case "diff":
		args = []string{"diff", "--no-ext-diff"}
	case "log":
		args = []string{"log", "--oneline", "-n", "20"}
	case "head":
		args = []string{"log", "-1", "--format=%H%n%h%n%s%n%an%n%cI"}
	default:
		http.NotFound(w, r)
		return
	}
	output, err := s.runGit(r.Context(), spec.CWD, args...)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cwd": spec.CWD, "command": "git " + strings.Join(args, " "), "output": output})
}

func (s *Server) runGit(parent context.Context, cwd string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, s.cfg.RequestTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}
