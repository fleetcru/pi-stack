package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseConfig(t *testing.T) {
	t.Setenv("PI_SERVER_DATA_DIR", t.TempDir())
	cfg, err := parseConfig([]string{
		"--server", "/opt/pi-server",
		"--url", "http://127.0.0.1:4141/",
		"--log-file", filepath.Join(t.TempDir(), "server.log"),
		"--", "--addr", "127.0.0.1:4141",
	})
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.serverPath != "/opt/pi-server" {
		t.Fatalf("serverPath = %q", cfg.serverPath)
	}
	if cfg.autoDownload {
		t.Fatal("autoDownload = true with an explicit --server path")
	}
	if cfg.serverURL != "http://127.0.0.1:4141" {
		t.Fatalf("serverURL = %q", cfg.serverURL)
	}
	wantArgs := []string{"--addr", "127.0.0.1:4141"}
	if !reflect.DeepEqual(cfg.serverArgs, wantArgs) {
		t.Fatalf("serverArgs = %#v, want %#v", cfg.serverArgs, wantArgs)
	}
}

func TestParseConfigDefaultsToAutoDownload(t *testing.T) {
	t.Setenv("PI_SERVER_DATA_DIR", t.TempDir())
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if !cfg.autoDownload {
		t.Fatal("autoDownload = false by default")
	}
	if cfg.serverPath == "" {
		t.Fatal("serverPath is empty")
	}
}

func TestParseConfigNoDownload(t *testing.T) {
	t.Setenv("PI_SERVER_DATA_DIR", t.TempDir())
	cfg, err := parseConfig([]string{"--no-download", "--release-repo", "example/project"})
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.autoDownload {
		t.Fatal("autoDownload = true with --no-download")
	}
	if cfg.releaseRepo != "example/project" {
		t.Fatalf("releaseRepo = %q", cfg.releaseRepo)
	}
}

func TestParseConfigRejectsEmptyURL(t *testing.T) {
	t.Setenv("PI_SERVER_DATA_DIR", t.TempDir())
	if _, err := parseConfig([]string{"--url", ""}); err == nil {
		t.Fatal("parseConfig() accepted an empty URL")
	}
}

func TestQueueDownloadRejectsDuplicate(t *testing.T) {
	a := &app{}
	started := make(chan struct{})
	release := make(chan struct{})
	var runs atomic.Int32
	run := func() error {
		runs.Add(1)
		close(started)
		<-release
		return nil
	}
	if !a.queueDownload(run) {
		t.Fatal("first download was rejected")
	}
	<-started
	if a.queueDownload(run) {
		t.Fatal("duplicate download was queued")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for a.downloadQueued.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if runs.Load() != 1 {
		t.Fatalf("download runs = %d, want 1", runs.Load())
	}
}

func TestShutdownTimeoutExceedsStopTimeouts(t *testing.T) {
	if trayShutdownTimeout <= gracefulStopTimeout+forcedStopTimeout {
		t.Fatalf("shutdown timeout %s must exceed stop timeout %s", trayShutdownTimeout, gracefulStopTimeout+forcedStopTimeout)
	}
}

func TestHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := app{cfg: config{serverURL: server.URL}}
	if !a.healthy() {
		t.Fatal("healthy() = false for a healthy server")
	}
}

func TestHealthyRejectsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	a := app{cfg: config{serverURL: server.URL}}
	if a.healthy() {
		t.Fatal("healthy() = true for a server error")
	}
}
