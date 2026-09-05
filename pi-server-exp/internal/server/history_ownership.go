package server

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
		if os.IsExist(err) && reclaimStaleHistoryLock(lockPath) {
			// The previous owner's process is gone; take over the lock.
			lock, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		}
		if err != nil {
			if os.IsExist(err) {
				return fmt.Errorf("session history %q is already owned by another server", key)
			}
			return fmt.Errorf("create history lock: %w", err)
		}
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

// reclaimStaleHistoryLock checks whether the lock file records a PID that is
// no longer running. Locks are only removed on clean release, so a crashed
// server leaves them behind; those are stale and safe to reclaim.
func reclaimStaleHistoryLock(lockPath string) bool {
	file, err := os.Open(lockPath)
	if err != nil {
		return false
	}
	// Close explicitly before removing: on Windows a file cannot be deleted
	// while it is open, so a deferred Close would run after os.Remove fails.
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		_ = file.Close()
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil {
		_ = file.Close()
		return false
	}
	// Never reclaim our own server's live in-process claim; pidAlive would
	// report true for us anyway, so this is only a guard against self-race.
	if pid <= 0 || pid == os.Getpid() {
		_ = file.Close()
		return false
	}
	if pidAlive(pid) {
		_ = file.Close()
		return false
	}
	_ = file.Close()
	return os.Remove(lockPath) == nil
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
