package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// MachineSession is a persisted Pi session found in Pi's default user session
// store. Discovery is intentionally independent from AllowedRoots: it reads
// only ~/.pi/agent/sessions, never arbitrary workspace files.
type MachineSession struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	CWD       string    `json:"cwd"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Size      int64     `json:"size"`
}

type machineSessionHeader struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
}

func defaultMachineSessionRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pi", "agent", "sessions"), nil
}

var (
	machineSessionCache     []MachineSession
	machineSessionCacheRoot string
	machineSessionCacheTime time.Time
	machineSessionCacheMu   sync.Mutex
)

const machineSessionCacheTTL = 10 * time.Second

func listMachineSessions(root string) ([]MachineSession, error) {
	machineSessionCacheMu.Lock()
	if root == machineSessionCacheRoot && time.Since(machineSessionCacheTime) < machineSessionCacheTTL && machineSessionCache != nil {
		result := machineSessionCache
		machineSessionCacheMu.Unlock()
		return result, nil
	}
	machineSessionCacheMu.Unlock()

	items := make([]MachineSession, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return walkErr
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		line, err := bufio.NewReaderSize(f, 4096).ReadBytes('\n')
		if err != nil {
			return nil
		}
		var header machineSessionHeader
		if err := json.Unmarshal(line, &header); err != nil || header.Type != "session" || header.ID == "" || header.CWD == "" {
			return nil
		}
		// Machine discovery should expose sessions that can actually be resumed
		// on this server. Old sessions often point at deleted or moved projects.
		cwdInfo, err := os.Stat(header.CWD)
		if err != nil || !cwdInfo.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		created, _ := time.Parse(time.RFC3339Nano, header.Timestamp)
		items = append(items, MachineSession{ID: header.ID, Path: path, CWD: header.CWD, CreatedAt: created, UpdatedAt: info.ModTime().UTC(), Size: info.Size()})
		return nil
	})
	if os.IsNotExist(err) {
		return items, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	machineSessionCacheMu.Lock()
	machineSessionCache = items
	machineSessionCacheRoot = root
	machineSessionCacheTime = time.Now()
	machineSessionCacheMu.Unlock()
	return items, nil
}

func (s *Server) listMachineSessions(w http.ResponseWriter, r *http.Request) {
	root, err := defaultMachineSessionRoot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, err := listMachineSessions(root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// Also include sessions from the server's managed session directory.
	// Sessions created via the web/companion API use --session-dir pointing
	// to {DataDir}/pi-sessions/{id}/ instead of ~/.pi/agent/sessions/.
	// Without this, those sessions are invisible to pi -r and this API.
	managedRoot := filepath.Join(s.cfg.DataDir, "pi-sessions")
	if managedRoot != root {
		managed, err := listMachineSessions(managedRoot)
		if err == nil {
			// Deduplicate: prefer the Pi-native entry if both exist.
			seen := make(map[string]bool, len(items))
			for _, it := range items {
				seen[it.ID] = true
			}
			for _, it := range managed {
				if !seen[it.ID] {
					items = append(items, it)
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"root": root, "managedRoot": managedRoot, "sessions": items})
}

func (s *Server) openMachineSession(w http.ResponseWriter, r *http.Request, machineID string) {
	if !validSessionID(machineID) {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "invalid machine session id")
		return
	}
	root, err := defaultMachineSessionRoot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, err := listMachineSessions(root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var found *MachineSession
	for i := range items {
		if items[i].ID == machineID {
			found = &items[i]
			break
		}
	}
	if found == nil {
		writeErrorCode(w, r, http.StatusNotFound, CodeSessionNotFound, "machine session not found")
		return
	}
	// Reopening from Webby or Companion must not start another Pi process for
	// the same persisted JSONL session. Return the live server session instead.
	for _, spec := range s.sessions.ListSpecs() {
		// Match by explicit --session path or by managed session directory.
		// Managed sessions use --session-dir instead of --session, so check both.
		matched := spec.SessionPath == found.Path
		if !matched && spec.ManagedSessionDir != "" {
			// The found.Path is a JSONL file inside the managed directory.
			managedDir := filepath.Dir(found.Path)
			matched = spec.ManagedSessionDir == managedDir || spec.ManagedSessionDir == found.Path
		}
		if !matched {
			continue
		}
		if process, ok := s.sessions.Get(spec.ID); ok {
			if running, _ := process.Status()["running"].(bool); running {
				writeJSON(w, http.StatusOK, map[string]any{"id": spec.ID, "machineSessionId": found.ID, "cwd": spec.CWD, "ws": "/v1/sessions/" + spec.ID + "/ws"})
				return
			}
		}
	}
	if info, err := os.Stat(found.CWD); err != nil || !info.IsDir() {
		writeErrorText(w, http.StatusBadRequest, "machine session working directory is unavailable")
		return
	}
	// This is a deliberate trusted-machine feature: session discovery and resume
	// are not constrained by workspace file roots. File APIs remain protected by
	// AllowedRoots and are not widened by this endpoint.
	spec := SessionSpec{ID: NewSessionID(), CWD: found.CWD, Args: []string{"--session", found.Path}, SessionPath: found.Path, Managed: true, Transport: "rpc", Status: "created", Title: filepath.Base(found.CWD)}
	p := NewPiProcess(spec, s.cfg, s.logger)
	p.onMessageEnd = func() { s.invalidateHistoryCache(spec.ID) }
	if err := s.sessions.AddIfCapacity(p, spec, s.cfg.MaxSessions); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()
	if err := p.Start(ctx); err != nil {
		_ = s.sessions.Delete(spec.ID)
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": spec.ID, "machineSessionId": found.ID, "cwd": spec.CWD, "ws": "/v1/sessions/" + spec.ID + "/ws"})
}

func (s *Server) machineSessionPost(w http.ResponseWriter, r *http.Request) {
	prefix := "/v1/machine-sessions/"
	tail := r.URL.Path[len(prefix):]
	const suffix = "/open"
	if len(tail) <= len(suffix) || tail[len(tail)-len(suffix):] != suffix {
		http.NotFound(w, r)
		return
	}
	s.openMachineSession(w, r, tail[:len(tail)-len(suffix)])
}
