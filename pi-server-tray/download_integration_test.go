package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestGitHubServerDownload exercises the real public GitHub Releases API and
// downloaded executable. It is opt-in to keep normal tests fast and offline.
func TestGitHubServerDownload(t *testing.T) {
	if os.Getenv("PI_TRAY_INTEGRATION") != "1" {
		t.Skip("set PI_TRAY_INTEGRATION=1 to test the real GitHub download")
	}
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("server release assets are currently published for Windows and Linux")
	}

	name := "pi-server"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	destination := filepath.Join(t.TempDir(), name)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	version, err := newServerDownloader(defaultReleaseRepo).ensureLatest(ctx, destination)
	if err != nil {
		t.Fatalf("download latest server: %v", err)
	}
	if version == "" || !fileExists(destination) {
		t.Fatalf("download did not install a server executable; version=%q", version)
	}

	cmd := exec.Command(destination, "-h")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("downloaded server did not execute: %v\n%s", err, output)
	}
}
