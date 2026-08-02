package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionBridge creates symlinks in Pi's native session store (~/.pi/agent/sessions/)
// pointing to managed session files. This allows pi -r to discover sessions
// created via the web/companion API.
type SessionBridge struct {
	nativeRoot string // ~/.pi/agent/sessions/
	logger     interface{ Warn(msg string, args ...any) }
}

// NewSessionBridge creates a bridge between the server's managed sessions and
// Pi's native session store.
func NewSessionBridge(logger interface{ Warn(msg string, args ...any) }) (*SessionBridge, error) {
	root, err := defaultMachineSessionRoot()
	if err != nil {
		return nil, fmt.Errorf("cannot determine native session root: %w", err)
	}
	// Ensure the native session directory exists
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create native session directory: %w", err)
	}
	return &SessionBridge{nativeRoot: root, logger: logger}, nil
}

// LinkManagedSession creates symlinks in the native per-project session
// directory. Pi's pi -r only searches ~/.pi/agent/sessions/<encoded-cwd>/, so
// placing a link directly under the sessions root leaves web-created sessions
// invisible even though their JSONL exists.
func (b *SessionBridge) LinkManagedSession(sessionID, managedDir, cwd string) error {
	if managedDir == "" || cwd == "" {
		return nil
	}
	nativeDir, err := b.nativeDirForCWD(cwd)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(nativeDir, 0o755); err != nil {
		return fmt.Errorf("cannot create native session directory: %w", err)
	}
	// Wait briefly for the Pi process to create its session file.
	// The JSONL file is created on first message, so we poll for up to 5s.
	jsonlFiles, err := b.waitForJSONLFiles(managedDir, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to find JSONL files in managed dir: %w", err)
	}
	if len(jsonlFiles) == 0 {
		// No JSONL file yet. The message-end callback retries linking once Pi has
		// persisted the conversation; the placeholder is only best-effort.
		return b.createPlaceholderLink(nativeDir, sessionID, managedDir)
	}
	for i, jsonlFile := range jsonlFiles {
		linkName := b.linkNameForSession(sessionID, i, len(jsonlFiles))
		linkPath := filepath.Join(nativeDir, linkName)
		// Remove existing link if present.
		_ = os.Remove(linkPath)
		if err := os.Symlink(jsonlFile, linkPath); err != nil {
			b.logger.Warn("failed to create session symlink", "link", linkPath, "target", jsonlFile, "error", err)
		}
	}
	return nil
}

func (b *SessionBridge) nativeDirForCWD(cwd string) (string, error) {
	resolved, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("cannot resolve session cwd: %w", err)
	}
	safePath := "--" + strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(strings.TrimLeft(resolved, "/\\")) + "--"
	return filepath.Join(b.nativeRoot, safePath), nil
}

// UnlinkManagedSession removes links created by LinkManagedSession, including
// the legacy root-level location used by older pi-server versions.
func (b *SessionBridge) UnlinkManagedSession(sessionID, cwd string) {
	dirs := []string{b.nativeRoot}
	if cwd != "" {
		if nativeDir, err := b.nativeDirForCWD(cwd); err == nil {
			dirs = append(dirs, nativeDir)
		}
	}
	patterns := []string{sessionID + ".jsonl", sessionID + "_*.jsonl"}
	for _, dir := range dirs {
		for _, pattern := range patterns {
			matches, _ := filepath.Glob(filepath.Join(dir, pattern))
			for _, match := range matches {
				if info, err := os.Lstat(match); err == nil && info.Mode()&os.ModeSymlink != 0 {
					_ = os.Remove(match)
				}
			}
		}
	}
}

// createPlaceholderLink creates a symlink to the managed directory itself.
// It is replaced with a JSONL-file link after Pi persists its first message.
func (b *SessionBridge) createPlaceholderLink(nativeDir, sessionID, managedDir string) error {
	linkPath := filepath.Join(nativeDir, sessionID+".jsonl")
	// Remove existing link if present
	os.Remove(linkPath)
	// Create a symlink to the managed directory.
	// Note: Directory symlinks may not work on all platforms, but this is
	// the best we can do without knowing the exact JSONL file name.
	if err := os.Symlink(managedDir, linkPath); err != nil {
		b.logger.Warn("failed to create placeholder symlink", "link", linkPath, "error", err)
		return err
	}
	return nil
}

// waitForJSONLFiles polls the managed directory until at least one .jsonl file
// appears or the timeout expires.
func (b *SessionBridge) waitForJSONLFiles(dir string, timeout time.Duration) ([]string, error) {
	deadline := time.Now().Add(timeout)
	for {
		files, err := findJSONLFiles(dir)
		if err != nil {
			return nil, err
		}
		if len(files) > 0 {
			return files, nil
		}
		if time.Now().After(deadline) {
			return nil, nil // Timeout - no files yet
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// findJSONLFiles returns all .jsonl files in the given directory (non-recursive).
func findJSONLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files, nil
}

// linkNameForSession generates the symlink filename for a session's JSONL file.
func (b *SessionBridge) linkNameForSession(sessionID string, index, total int) string {
	if total == 1 {
		return sessionID + ".jsonl"
	}
	return fmt.Sprintf("%s_%d.jsonl", sessionID, index)
}
