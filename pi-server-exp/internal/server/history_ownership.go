package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
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
	if owner, ok := s.historyOwners[key]; ok && owner == spec.ID {
		return nil
	}
	lockDir := filepath.Join(s.cfg.DataDir, "history-locks")
	if err := os.MkdirAll(lockDir, 0o750); err != nil {
		return fmt.Errorf("create history lock directory: %w", err)
	}
	digest := sha256.Sum256([]byte(key))
	lockPath := filepath.Join(lockDir, hex.EncodeToString(digest[:])+".lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("session history %q is already owned by another server", key)
		}
		return fmt.Errorf("create history lock: %w", err)
	}
	if _, err := fmt.Fprintf(lock, "%d\n%s\n", os.Getpid(), key); err != nil {
		_ = lock.Close()
		_ = os.Remove(lockPath)
		return fmt.Errorf("write history lock: %w", err)
	}
	s.historyOwnerLocks[key] = lock
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
		if lock := s.historyOwnerLocks[key]; lock != nil {
			_ = lock.Close()
			delete(s.historyOwnerLocks, key)
			digest := sha256.Sum256([]byte(key))
			_ = os.Remove(filepath.Join(s.cfg.DataDir, "history-locks", hex.EncodeToString(digest[:])+".lock"))
		}
	}
	s.historyOwnerMu.Unlock()
}
