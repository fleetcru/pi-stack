package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	if errors.Is(err, os.ErrNotExist) { return persistedRelayCommands{}, nil }
	if err != nil { return persistedRelayCommands{}, err }
	var commands persistedRelayCommands
	if err := json.Unmarshal(data, &commands); err != nil {
		_ = os.WriteFile(fmt.Sprintf("%s.corrupt.%d", path, time.Now().UnixMilli()), data, 0o600)
		return persistedRelayCommands{}, err
	}
	if commands == nil { return persistedRelayCommands{}, nil }
	return commands, nil
}

// saveRelayCommands writes atomically (temp file + rename) so a crash mid-write
// cannot leave a truncated store that discards every queued command.
func saveRelayCommands(path string, commands persistedRelayCommands) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil { return err }
	data, err := json.MarshalIndent(commands, "", "  ")
	if err != nil { return err }
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { return err }
	return os.Rename(tmp, path)
}
