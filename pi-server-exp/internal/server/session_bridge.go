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

// LinkManagedSession creates a symlink in ~/.pi/agent/sessions/ for each JSONL
// file found in the managed session directory. This is called after a session
// is created via the API so that pi -r can discover it.
//
// The symlink name format is: {sessionID}.jsonl -> {managedDir}/{filename}.jsonl
// If multiple JSONL files exist, they're linked as {sessionID}_{n}.jsonl.
func (b *SessionBridge) LinkManagedSession(sessionID, managedDir string) error {
	if managedDir == "" {
		return nil
	}
	// Wait briefly for the Pi process to create its session file.
	// The JSONL file is created on first message, so we poll for up to 5s.
	jsonlFiles, err := b.waitForJSONLFiles(managedDir, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to find JSONL files in managed dir: %w", err)
	}
	if len(jsonlFiles) == 0 {
		// No JSONL files yet - this is normal for a fresh session.
		// We'll create a placeholder link that can be updated later.
		return b.createPlaceholderLink(sessionID, managedDir)
	}
	for i, jsonlFile := range jsonlFiles {
		linkName := b.linkNameForSession(sessionID, i, len(jsonlFiles))
		linkPath := filepath.Join(b.nativeRoot, linkName)
		// Remove existing link if present
		os.Remove(linkPath)
		// Create symlink pointing to the actual JSONL file
		if err := os.Symlink(jsonlFile, linkPath); err != nil {
			b.logger.Warn("failed to create session symlink", "link", linkPath, "target", jsonlFile, "error", err)
			continue
		}
	}
	return nil
}

// UnlinkManagedSession removes symlinks created by LinkManagedSession.
func (b *SessionBridge) UnlinkManagedSession(sessionID string) {
	// Remove all possible link patterns for this session
	patterns := []string{
		sessionID + ".jsonl",
		sessionID + "_*.jsonl",
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(b.nativeRoot, pattern))
		for _, match := range matches {
			// Only remove symlinks, never actual files
			if info, err := os.Lstat(match); err == nil && info.Mode()&os.ModeSymlink != 0 {
				os.Remove(match)
			}
		}
	}
}

// createPlaceholderLink creates a symlink to the managed directory itself.
// When pi -r scans the directory, it will follow the symlink and find JSONL files.
// This handles the case where the session hasn't written any messages yet.
func (b *SessionBridge) createPlaceholderLink(sessionID, managedDir string) error {
	linkPath := filepath.Join(b.nativeRoot, sessionID+".jsonl")
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
