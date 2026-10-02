package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Continuation is explicit and only supported after a locally verified TUI exits.
// Registration and continuation share this mutex, fencing late reconnects.
func (s *Server) continueExternalSession(w http.ResponseWriter, r *http.Request, id string) {
	if !validSessionID(id) {
		writeErrorText(w, http.StatusBadRequest, "invalid session id")
		return
	}
	s.relayOwnershipMu.Lock()
	defer s.relayOwnershipMu.Unlock()
	oldSpec, exists := s.sessions.GetSpec(id)
	if exists && oldSpec.Transport == "rpc" && oldSpec.Metadata["continuedFromRelay"] == "true" {
		writeJSON(w, http.StatusOK, map[string]any{"sessionId": id, "transport": "rpc", "continued": true})
		return
	}
	s.external.mu.Lock()
	session := s.external.sessions[id]
	reason := externalContinueBlockedReason(session)
	if reason != "" {
		s.external.mu.Unlock()
		writeErrorText(w, http.StatusConflict, reason)
		return
	}
	if !exists || oldSpec.Transport != "relay" {
		s.external.mu.Unlock()
		writeErrorText(w, http.StatusConflict, "session is not registered as a relay")
		return
	}
	session.transferring = true
	historyPath, cwd, title, next := session.SessionPath, session.CWD, session.Title, session.next
	s.external.mu.Unlock()
	transferred := false
	defer func() {
		if !transferred {
			s.external.mu.Lock()
			session.transferring = false
			s.external.mu.Unlock()
		}
	}()
	fail := func(code int, err error) { writeErrorText(w, code, err.Error()) }
	// Like machine-session resume, this is a trusted local-history operation.
	// A project's original CWD need not sit below the server launch directory.
	// File APIs still enforce AllowedRoots independently.
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		fail(http.StatusConflict, fmt.Errorf("session working directory is invalid"))
		return
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		fail(http.StatusConflict, fmt.Errorf("session working directory is unavailable"))
		return
	}
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		fail(http.StatusConflict, fmt.Errorf("session working directory is unavailable"))
		return
	}
	historyPath, err = s.validateContinuationHistoryPath(historyPath)
	if err != nil {
		fail(http.StatusConflict, fmt.Errorf("session history is unavailable on this server"))
		return
	}
	if err := validatePiHistoryFile(historyPath, id, cwd); err != nil {
		fail(http.StatusConflict, err)
		return
	}
	spec := oldSpec
	spec.CWD, spec.Title, spec.SessionPath = cwd, title, historyPath
	spec.Transport, spec.Managed, spec.Status = "rpc", true, "created"
	spec.BridgePID, spec.BridgeStartedAt = 0, ""
	spec.Args = []string{"--session", historyPath}
	spec.UpdatedAt = time.Now().UTC()
	spec.Metadata = make(map[string]string, len(oldSpec.Metadata)+1)
	for key, value := range oldSpec.Metadata {
		spec.Metadata[key] = value
	}
	spec.Metadata["continuedFromRelay"] = "true"
	if err := s.reserveHistoryOwner(spec); err != nil {
		fail(http.StatusConflict, err)
		return
	}
	// The relay already owns the same claim. Keep it on failure so another
	// session cannot open the file while the user retries this handoff.
	process := NewPiProcess(spec, s.cfg, s.logger)
	process.onMessageEnd = func() { s.invalidateHistoryCache(id) }
	process.mu.Lock()
	if process.eventSeq < next {
		process.eventSeq = next
	}
	process.mu.Unlock()
	committed := false
	defer func() {
		if !committed {
			ctx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
			defer cancel()
			_ = process.Close(ctx)
		}
	}()
	if err := s.ensureSessionCapacity(process); err != nil {
		fail(http.StatusServiceUnavailable, err)
		return
	}
	timeout := s.cfg.RequestTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	// Start alone only proves exec succeeded. A successful RPC state response
	// confirms Pi loaded the exact history before publishing the new owner.
	state, err := process.Request(ctx, RPCCommand{"type": "get_state"})
	if err != nil {
		fail(http.StatusBadGateway, fmt.Errorf("could not resume Pi: %w", err))
		return
	}
	data, _ := state["data"].(map[string]any)
	loadedPath, _ := data["sessionFile"].(string)
	loadedID, _ := data["sessionId"].(string)
	if loadedPath == "" || loadedID != id || canonicalPath(loadedPath) != canonicalPath(historyPath) {
		fail(http.StatusBadGateway, fmt.Errorf("Pi did not resume the requested session history"))
		return
	}
	if err := s.sessions.replaceRelayProcess(process, spec); err != nil {
		fail(http.StatusInternalServerError, err)
		return
	}
	s.external.mu.Lock()
	delete(s.external.sessions, id)
	// Wake existing viewers so they reconnect to the newly selected RPC transport.
	for channel := range session.subs {
		close(channel)
		delete(session.subs, channel)
	}
	session.leaseToken = ""
	session.relayGeneration++
	s.external.mu.Unlock()
	s.releaseDistributedRun(id)
	s.invalidateHistoryCache(id)
	s.stateCacheMu.Lock()
	delete(s.stateCache, id)
	s.stateCacheMu.Unlock()
	committed, transferred = true, true
	writeJSON(w, http.StatusOK, map[string]any{"sessionId": id, "transport": "rpc", "continued": true})
}

func (s *Server) validateContinuationHistoryPath(candidate string) (string, error) {
	if candidate == "" {
		return "", os.ErrNotExist
	}
	absolute, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	roots := []string{filepath.Join(s.cfg.DataDir, "pi-sessions")}
	if root, err := defaultMachineSessionRoot(); err == nil {
		roots = append(roots, root)
	}
	for _, root := range roots {
		root, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if actual, err := filepath.EvalSymlinks(root); err == nil {
			root = actual
		}
		relative, err := filepath.Rel(root, resolved)
		if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && strings.EqualFold(filepath.Ext(resolved), ".jsonl") {
			return resolved, nil
		}
	}
	return "", os.ErrPermission
}

func validatePiHistoryFile(path, id, cwd string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("session history is not a regular file")
	}
	var header struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		CWD  string `json:"cwd"`
	}
	if json.NewDecoder(io.LimitReader(file, 64<<10)).Decode(&header) != nil || header.Type != "session" || header.ID != id || canonicalPath(header.CWD) != canonicalPath(cwd) {
		return fmt.Errorf("session history header does not match this relay")
	}
	return nil
}
