package server

import (
	"fmt"
	"path/filepath"
)

func historyOwnerKey(spec SessionSpec) string {
	if spec.SessionPath == "" {
		return ""
	}
	path, err := filepath.Abs(filepath.Clean(spec.SessionPath))
	if err != nil {
		return filepath.Clean(spec.SessionPath)
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

func (s *Server) reserveHistoryOwner(spec SessionSpec) error {
	key := historyOwnerKey(spec)
	if key == "" {
		return nil
	}
	s.historyOwnerMu.Lock()
	defer s.historyOwnerMu.Unlock()
	if owner, ok := s.historyOwners[key]; ok && owner != spec.ID {
		return fmt.Errorf("session history %q is already owned by session %q", key, owner)
	}
	s.historyOwners[key] = spec.ID
	return nil
}

func (s *Server) releaseHistoryOwner(spec SessionSpec) {
	key := historyOwnerKey(spec)
	if key == "" {
		return
	}
	s.historyOwnerMu.Lock()
	if owner, ok := s.historyOwners[key]; ok && owner == spec.ID {
		delete(s.historyOwners, key)
	}
	s.historyOwnerMu.Unlock()
}
