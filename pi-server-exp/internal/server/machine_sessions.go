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
	ID              string    `json:"id"`
	Path            string    `json:"path"`
	CWD             string    `json:"cwd"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	Size            int64     `json:"size"`
	ServerSessionID string    `json:"serverSessionId,omitempty"`
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

type machineSessionCacheEntry struct {
	items    []MachineSession
	cachedAt time.Time
}

var (
	// Keep an entry per root: listing native and managed sessions in one
	// request must not evict the other root's cache.
	machineSessionCaches  = map[string]machineSessionCacheEntry{}
	machineSessionCacheMu sync.Mutex
)

const machineSessionCacheTTL = 10 * time.Second

func listMachineSessions(root string) ([]MachineSession, error) {
	machineSessionCacheMu.Lock()
	if cached, ok := machineSessionCaches[root]; ok && time.Since(cached.cachedAt) < machineSessionCacheTTL {
		result := cached.items
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
	machineSessionCaches[root] = machineSessionCacheEntry{items: items, cachedAt: time.Now()}
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
	specs := s.sessions.ListSpecs()
	resolve := memoizedCanonicalPath()
	for i := range items {
		if spec := s.preferredServerSessionWithResolver(items[i], specs, resolve); spec != nil {
			items[i].ServerSessionID = spec.ID
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"root": root, "managedRoot": managedRoot, "sessions": items})
}

func canonicalPath(path string) string {
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	return filepath.Clean(path)
}

type canonicalPathResolver func(string) string

func memoizedCanonicalPath() canonicalPathResolver {
	cache := make(map[string]string)
	return func(path string) string {
		if resolved, ok := cache[path]; ok {
			return resolved
		}
		resolved := canonicalPath(path)
		cache[path] = resolved
		return resolved
	}
}

func matchingServerSession(machine MachineSession, specs []SessionSpec) *SessionSpec {
	return matchingServerSessionWithResolver(machine, specs, canonicalPath)
}

func matchingServerSessionWithResolver(machine MachineSession, specs []SessionSpec, resolve canonicalPathResolver) *SessionSpec {
	machinePath := resolve(machine.Path)
	machineDir := filepath.Dir(machinePath)
	for i := range specs {
		spec := &specs[i]
		if spec.Transport == "relay" {
			continue
		}
		if spec.SessionPath != "" && resolve(spec.SessionPath) == machinePath {
			return spec
		}
		if spec.ManagedSessionDir != "" && resolve(spec.ManagedSessionDir) == machineDir {
			return spec
		}
	}
	return nil
}

func duplicateRelaySpec(spec SessionSpec, specs []SessionSpec) bool {
	return duplicateRelaySpecWithResolver(spec, specs, canonicalPath)
}

func duplicateRelaySpecWithResolver(spec SessionSpec, specs []SessionSpec, resolve canonicalPathResolver) bool {
	if spec.Transport != "relay" || spec.SessionPath == "" {
		return false
	}
	return matchingServerSessionWithResolver(MachineSession{Path: spec.SessionPath}, specs, resolve) != nil
}

func sessionSpecsShareHistory(a, b SessionSpec) bool {
	return sessionSpecsShareHistoryWithResolver(a, b, canonicalPath)
}

func sessionSpecsShareHistoryWithResolver(a, b SessionSpec, resolve canonicalPathResolver) bool {
	if a.SessionPath == "" && b.SessionPath == "" {
		return false
	}
	if a.SessionPath != "" && b.SessionPath != "" && resolve(a.SessionPath) == resolve(b.SessionPath) {
		return true
	}
	if a.SessionPath != "" && b.ManagedSessionDir != "" && resolve(filepath.Dir(a.SessionPath)) == resolve(b.ManagedSessionDir) {
		return true
	}
	if b.SessionPath != "" && a.ManagedSessionDir != "" && resolve(filepath.Dir(b.SessionPath)) == resolve(a.ManagedSessionDir) {
		return true
	}
	return false
}

func (s *Server) liveRelaySpecForHistory(spec SessionSpec, specs []SessionSpec) *SessionSpec {
	return s.liveRelaySpecForHistoryWithResolver(spec, specs, canonicalPath)
}

func (s *Server) liveRelaySpecForHistoryWithResolver(spec SessionSpec, specs []SessionSpec, resolve canonicalPathResolver) *SessionSpec {
	const relayFreshness = 90 * time.Second
	for i := range specs {
		candidate := &specs[i]
		if candidate.Transport != "relay" || !sessionSpecsShareHistoryWithResolver(*candidate, spec, resolve) {
			continue
		}
		relay, ok := s.external.get(candidate.ID)
		if ok && (relay.RelayConnected || time.Since(relay.UpdatedAt) <= relayFreshness) {
			return candidate
		}
	}
	return nil
}

// preferredServerSession prevents Machine Session Discovery from starting a
// second RPC process for a JSONL file already owned by a live bridged TUI.
func (s *Server) preferredServerSession(machine MachineSession, specs []SessionSpec) *SessionSpec {
	return s.preferredServerSessionWithResolver(machine, specs, canonicalPath)
}

func (s *Server) preferredServerSessionWithResolver(machine MachineSession, specs []SessionSpec, resolve canonicalPathResolver) *SessionSpec {
	probe := SessionSpec{SessionPath: machine.Path}
	if relay := s.liveRelaySpecForHistoryWithResolver(probe, specs, resolve); relay != nil {
		return relay
	}
	return matchingServerSessionWithResolver(machine, specs, resolve)
}

// hideDuplicateSessionSpec presents the live TUI relay as the canonical owner
// when an unsafe legacy RPC+relay duplicate already exists for one JSONL file.
func (s *Server) hideDuplicateSessionSpec(spec SessionSpec, specs []SessionSpec) bool {
	return s.hideDuplicateSessionSpecWithResolver(spec, specs, canonicalPath)
}

func (s *Server) hideDuplicateSessionSpecWithResolver(spec SessionSpec, specs []SessionSpec, resolve canonicalPathResolver) bool {
	if relay := s.liveRelaySpecForHistoryWithResolver(spec, specs, resolve); relay != nil {
		return spec.ID != relay.ID
	}
	return duplicateRelaySpecWithResolver(spec, specs, resolve)
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
	// Also search the server's managed session directory. The list endpoint
	// merges both roots, so the open endpoint must accept IDs from either.
	managedRoot := filepath.Join(s.cfg.DataDir, "pi-sessions")
	if managedRoot != root {
		if managed, merr := listMachineSessions(managedRoot); merr == nil {
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
	// the same persisted JSONL session. Resolve bridge symlinks before matching:
	// native discovery prefers the symlink while managed specs store its target
	// directory, and comparing the unresolved paths created duplicate processes.
	if spec := s.preferredServerSession(*found, s.sessions.ListSpecs()); spec != nil {
		writeJSON(w, http.StatusOK, map[string]any{"id": spec.ID, "machineSessionId": found.ID, "cwd": spec.CWD, "ws": "/v1/sessions/" + spec.ID + "/ws"})
		return
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
	s.wirePiProcess(p)
	p.onMessageEnd = func() { s.invalidateHistoryCache(spec.ID) }
	if err := s.sessions.AddIfCapacity(p, spec, int(s.maxSessionsAtomicValue())); err != nil {
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
