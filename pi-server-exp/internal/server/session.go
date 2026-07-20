package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

type SessionSpec struct {
	ID                string            `json:"id"`
	CWD               string            `json:"cwd"`
	Args              []string          `json:"args,omitempty"`
	Env               map[string]string `json:"env,omitempty"`
	SessionPath       string            `json:"sessionPath,omitempty"`
	ManagedSessionDir string            `json:"managedSessionDir,omitempty"`
	Managed           bool              `json:"managed"`
	Transport         string            `json:"transport,omitempty"` // rpc or relay
	Restart           bool              `json:"restart,omitempty"`
	Status            string            `json:"status,omitempty"`
	LastExit          string            `json:"lastExit,omitempty"`
	Project           string            `json:"project,omitempty"`
	Title             string            `json:"title,omitempty"`
	TaskType          string            `json:"taskType,omitempty"`
	Owner             string            `json:"owner,omitempty"`
	Labels            []string          `json:"labels,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	CreatedAt         time.Time         `json:"createdAt"`
	UpdatedAt         time.Time         `json:"updatedAt"`
}

type SessionRegistry struct {
	mu         sync.RWMutex
	path       string
	sessions   map[string]*PiProcess
	specs      map[string]SessionSpec
	maxSessions int
}

func NewSessionRegistry(path string, maxSessions int) *SessionRegistry {
	return &SessionRegistry{path: path, sessions: map[string]*PiProcess{}, specs: map[string]SessionSpec{}, maxSessions: maxSessions}
}

func (r *SessionRegistry) Load() error {
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
	var specs []SessionSpec
	if err := json.Unmarshal(b, &specs); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, spec := range specs {
		r.specs[spec.ID] = spec
	}
	return nil
}

func (r *SessionRegistry) Save() error {
	if r.path == "" {
		return nil
	}
	r.mu.RLock()
	specs := make([]SessionSpec, 0, len(r.specs))
	for _, spec := range r.specs {
		specs = append(specs, spec)
	}
	r.mu.RUnlock()
	return writeJSONAtomic(r.path, specs)
}

// RegisterSpec persists a session owned by a non-RPC transport (currently the
// external Pi TUI relay) without creating a PiProcess.
func (r *SessionRegistry) RegisterSpec(spec SessionSpec) (SessionSpec, error) {
	r.mu.Lock()
	current, exists := r.specs[spec.ID]
	now := time.Now().UTC()
	if exists {
		if spec.CWD != "" {
			current.CWD = spec.CWD
		}
		if spec.Title != "" {
			current.Title = spec.Title
		}
		if spec.Transport != "" {
			current.Transport = spec.Transport
		}
		current.Status = spec.Status
		current.UpdatedAt = now
		r.specs[spec.ID] = current
		r.mu.Unlock()
		return current, r.Save()
	}
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = now
	}
	spec.UpdatedAt = now
	r.specs[spec.ID] = spec
	r.mu.Unlock()
	return spec, r.Save()
}

func (r *SessionRegistry) Add(p *PiProcess, spec SessionSpec) error {
	return r.addInternal(p, spec, 0)
}

// AddIfCapacity adds the session only if the current spec count is below
// maxSessions. This eliminates the TOCTOU race between ActiveCount() and Add()
// in session creation handlers.
func (r *SessionRegistry) AddIfCapacity(p *PiProcess, spec SessionSpec, maxSessions int) error {
	return r.addInternal(p, spec, maxSessions)
}

func (r *SessionRegistry) addInternal(p *PiProcess, spec SessionSpec, maxSessions int) error {
	r.mu.Lock()
	if _, exists := r.specs[p.id]; exists {
		r.mu.Unlock()
		return fmt.Errorf("session already exists: %s", p.id)
	}
	if maxSessions > 0 && len(r.specs) >= maxSessions {
		r.mu.Unlock()
		return fmt.Errorf("max active Pi sessions reached")
	}
	now := time.Now().UTC()
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = now
	}
	spec.UpdatedAt = now
	r.sessions[p.id] = p
	r.specs[p.id] = spec
	r.mu.Unlock()
	return r.Save()
}

func (r *SessionRegistry) Attach(p *PiProcess) {
	r.mu.Lock()
	r.sessions[p.id] = p
	r.mu.Unlock()
}

// AttachIfAbsent atomically registers p only if no process is already
// registered for the same session ID. Returns the registered process and
// true if p was newly attached, or the existing process and false if one
// was already present. Callers must close the redundant process when false
// is returned to avoid leaking OS resources.
func (r *SessionRegistry) AttachIfAbsent(p *PiProcess) (*PiProcess, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.sessions[p.id]; ok {
		return existing, false
	}
	r.sessions[p.id] = p
	return p, true
}

func (r *SessionRegistry) Get(id string) (*PiProcess, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.sessions[id]
	return p, ok
}

func (r *SessionRegistry) GetSpec(id string) (SessionSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.specs[id]
	return s, ok
}

func (r *SessionRegistry) UpdateMetadata(id string, update SessionMetadataUpdate) (SessionSpec, error) {
	r.mu.Lock()
	spec, ok := r.specs[id]
	if !ok {
		r.mu.Unlock()
		return SessionSpec{}, fmt.Errorf("session not found: %s", id)
	}
	if update.Project != nil {
		spec.Project = *update.Project
	}
	if update.Title != nil {
		spec.Title = *update.Title
	}
	if update.TaskType != nil {
		spec.TaskType = *update.TaskType
	}
	if update.Owner != nil {
		spec.Owner = *update.Owner
	}
	if update.Labels != nil {
		spec.Labels = append([]string(nil), (*update.Labels)...)
	}
	if update.Metadata != nil {
		spec.Metadata = make(map[string]string, len(*update.Metadata))
		for k, v := range *update.Metadata {
			spec.Metadata[k] = v
		}
	}
	spec.UpdatedAt = time.Now().UTC()
	r.specs[id] = spec
	r.mu.Unlock()
	return spec, r.Save()
}

func (r *SessionRegistry) Delete(id string) error {
	r.mu.Lock()
	delete(r.sessions, id)
	delete(r.specs, id)
	r.mu.Unlock()
	return r.Save()
}

func (r *SessionRegistry) ListSpecs() []SessionSpec {
	r.mu.RLock()
	// Copy specs and session references under the lock, then call Status()
	// outside it to prevent lock-ordering deadlocks (SessionRegistry.mu →
	// PiProcess.mu vs the reverse order in file watchers / Emit paths).
	specs := make([]SessionSpec, 0, len(r.specs))
	processes := make(map[string]*PiProcess, len(r.sessions))
	for _, spec := range r.specs {
		specs = append(specs, spec)
	}
	for id, p := range r.sessions {
		processes[id] = p
	}
	r.mu.RUnlock()
	for i := range specs {
		if process, ok := processes[specs[i].ID]; ok {
			if status, ok := process.Status()["status"].(string); ok {
				specs[i].Status = status
			}
		}
	}
	return specs
}

func (r *SessionRegistry) ActiveCount() int {
	r.mu.RLock()
	processes := make([]*PiProcess, 0, len(r.sessions))
	for _, p := range r.sessions {
		processes = append(processes, p)
	}
	r.mu.RUnlock()
	count := 0
	for _, p := range processes {
		status := p.Status()
		if running, _ := status["running"].(bool); running {
			count++
		}
	}
	return count
}

func (r *SessionRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.specs))
	for id := range r.specs {
		out = append(out, id)
	}
	return out
}

func (r *SessionRegistry) CloseAll(ctx context.Context) {
	r.mu.RLock()
	sessions := make([]*PiProcess, 0, len(r.sessions))
	for _, p := range r.sessions {
		sessions = append(sessions, p)
	}
	r.mu.RUnlock()
	for _, p := range sessions {
		_ = p.Close(ctx)
	}
}

// checkCanStart atomically verifies that starting a new process won't exceed
// MaxSessions. Called from ensureSessionCapacity before the process auto-starts.
func (r *SessionRegistry) checkCanStart() bool {
	if r.maxSessions <= 0 {
		return true
	}
	// Copy process references under the lock, then call Status() outside
	// to prevent lock-ordering deadlocks (SessionRegistry.mu → PiProcess.mu
	// vs the reverse in file watchers / dispatch paths).
	r.mu.RLock()
	processes := make([]*PiProcess, 0, len(r.sessions))
	for _, p := range r.sessions {
		processes = append(processes, p)
	}
	r.mu.RUnlock()
	count := 0
	for _, p := range processes {
		if st := p.Status(); st["running"] == true {
			count++
		}
	}
	return count < r.maxSessions
}
