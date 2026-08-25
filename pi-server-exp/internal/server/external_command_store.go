package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// Only commands are persisted. Relay connections/events are intentionally
// ephemeral, but an unacknowledged user command must survive a daemon restart.
type persistedRelayCommands map[string][]ExternalCommand

// loadRelayCommands never returns a nil map: a corrupt store must not poison
// the registry (a nil map panics on the first assignment). The corrupt file is
// preserved alongside the original for manual recovery.
func loadRelayCommands(path string) (persistedRelayCommands, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return persistedRelayCommands{}, nil
	}
	if err != nil {
		return persistedRelayCommands{}, err
	}

	var commands persistedRelayCommands
	if err := json.Unmarshal(data, &commands); err != nil {
		// Preserve the invalid payload beside the original so an operator can
		// recover queued commands instead of losing the evidence on startup.
		corruptPath := fmt.Sprintf("%s.corrupt.%d", path, time.Now().UnixMilli())
		_ = os.WriteFile(corruptPath, data, 0o600)
		return persistedRelayCommands{}, err
	}
	if commands == nil {
		return persistedRelayCommands{}, nil
	}
	return commands, nil
}

var cmdTmpSeq atomic.Uint64

// saveRelayCommands writes atomically (temp file + rename) so a crash mid-write
// cannot leave a truncated store that discards every queued command.
func saveRelayCommands(path string, commands persistedRelayCommands) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(commands, "", "  ")
	if err != nil {
		return err
	}

	// Use a process-unique temporary name so concurrent saves cannot rename
	// each other's payload. Remove it on every failure path; stale temp files
	// otherwise make the data directory look corrupted during recovery.
	tmp := fmt.Sprintf("%s.%d.%d.tmp", path, os.Getpid(), cmdTmpSeq.Add(1))
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
