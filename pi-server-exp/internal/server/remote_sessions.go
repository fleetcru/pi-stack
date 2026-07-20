package server

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type RemoteSession struct {
	ID              string    `json:"id"`
	WorkerID        string    `json:"workerId"`
	WorkerSessionID string    `json:"workerSessionId"`
	CreatedAt       time.Time `json:"createdAt"`
}
type RemoteSessionRegistry struct {
	mu       sync.RWMutex
	path     string
	sessions map[string]RemoteSession
}

func NewRemoteSessionRegistry(path string) *RemoteSessionRegistry {
	return &RemoteSessionRegistry{path: path, sessions: map[string]RemoteSession{}}
}
func (r *RemoteSessionRegistry) Load() error {
	b, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var all []RemoteSession
	if err = json.Unmarshal(b, &all); err != nil {
		return err
	}
	for _, s := range all {
		r.sessions[s.ID] = s
	}
	return nil
}
func (r *RemoteSessionRegistry) Add(s RemoteSession) error {
	r.mu.Lock()
	r.sessions[s.ID] = s
	r.mu.Unlock()
	return r.Save()
}
func (r *RemoteSessionRegistry) Get(id string) (RemoteSession, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[id]
	return s, ok
}
func (r *RemoteSessionRegistry) Delete(id string) error {
	r.mu.Lock()
	delete(r.sessions, id)
	r.mu.Unlock()
	return r.Save()
}

func (r *RemoteSessionRegistry) List() []RemoteSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RemoteSession, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	return out
}
func (r *RemoteSessionRegistry) Save() error {
	r.mu.RLock()
	all := make([]RemoteSession, 0, len(r.sessions))
	for _, s := range r.sessions {
		all = append(all, s)
	}
	r.mu.RUnlock()
	return writeJSONAtomic(r.path, all)
}
