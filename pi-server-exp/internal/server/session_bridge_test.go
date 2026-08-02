package server

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionBridgeLinksIntoProjectSessionDirectory(t *testing.T) {
	nativeRoot := t.TempDir()
	cwd := filepath.Join(t.TempDir(), "srv", "projects")
	managedDir := t.TempDir()
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(managedDir, "session.jsonl")
	if err := os.WriteFile(target, []byte(`{"type":"session"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	bridge := &SessionBridge{nativeRoot: nativeRoot, logger: slog.Default()}
	if err := bridge.LinkManagedSession("web-session", managedDir, cwd); err != nil {
		t.Fatalf("LinkManagedSession() error = %v", err)
	}

	projectDir, err := bridge.nativeDirForCWD(cwd)
	if err != nil {
		t.Fatalf("nativeDirForCWD() error = %v", err)
	}
	linkPath := filepath.Join(projectDir, "web-session.jsonl")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("project session link missing: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("project session link mode = %v, want symlink", info.Mode())
	}
	gotTarget, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if gotTarget != target {
		t.Errorf("link target = %q, want %q", gotTarget, target)
	}
	if _, err := os.Lstat(filepath.Join(nativeRoot, "web-session.jsonl")); !os.IsNotExist(err) {
		t.Errorf("legacy root-level link exists or could not be checked: %v", err)
	}
}
