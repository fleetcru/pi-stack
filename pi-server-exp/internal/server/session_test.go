package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRemoveManagedSessionDir(t *testing.T) {
	dataDir := t.TempDir()
	s := New(Config{DataDir: dataDir}, testLogger())
	managed := filepath.Join(dataDir, "pi-sessions", "s-1")
	if err := os.MkdirAll(managed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(managed, "session.jsonl"), []byte("history"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.removeManagedSessionDir(managed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(managed); !os.IsNotExist(err) {
		t.Fatalf("managed directory still exists: %v", err)
	}
	if err := s.removeManagedSessionDir(dataDir); err == nil {
		t.Fatal("expected refusal to remove outside managed session root")
	}
}

func TestSessionRegistryRejectsDuplicateID(t *testing.T) {
	r := NewSessionRegistry("", 0)
	cfg := Config{}
	spec := SessionSpec{ID: "same", CWD: "."}
	if err := r.Add(NewPiProcess(spec, cfg, testLogger()), spec); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(NewPiProcess(spec, cfg, testLogger()), spec); err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestHistoryOwnershipRejectsSamePathAndReleases(t *testing.T) {
	s := New(Config{DataDir: t.TempDir()}, testLogger())
	path := filepath.Join(t.TempDir(), "shared.jsonl")
	first := SessionSpec{ID: "one", SessionPath: path}
	second := SessionSpec{ID: "two", SessionPath: path}
	if err := s.reserveHistoryOwner(first); err != nil {
		t.Fatal(err)
	}
	if err := s.reserveHistoryOwner(second); err == nil {
		t.Fatal("expected duplicate history ownership to be rejected")
	}
	s.releaseHistoryOwner(first)
	if err := s.reserveHistoryOwner(second); err != nil {
		t.Fatalf("history ownership was not released: %v", err)
	}
	t.Cleanup(func() { s.releaseHistoryOwner(second) })
}

func TestHistoryOwnershipReclaimsStaleLock(t *testing.T) {
	s := New(Config{DataDir: t.TempDir()}, testLogger())
	path := filepath.Join(t.TempDir(), "shared.jsonl")
	// Find the lock file path the same way reserveHistoryOwner does.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = path
	}
	digest := sha256.Sum256([]byte(resolved))
	lockPath := filepath.Join(s.cfg.DataDir, "history-locks", hex.EncodeToString(digest[:])+".lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o750); err != nil {
		t.Fatal(err)
	}
	// Write a lock whose owner PID cannot exist (above the Windows PID max,
	// and no live process on Unix), simulating a server that crashed without
	// cleanup.
	if err := os.WriteFile(lockPath, []byte("999999999\n"+resolved+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := SessionSpec{ID: "one", SessionPath: path}
	if err := s.reserveHistoryOwner(spec); err != nil {
		t.Fatalf("stale lock was not reclaimed: %v", err)
	}
	t.Cleanup(func() { s.releaseHistoryOwner(spec) })
	// A second session must still be rejected by the live owner.
	if err := s.reserveHistoryOwner(SessionSpec{ID: "two", SessionPath: path}); err == nil {
		t.Fatal("expected duplicate history ownership to be rejected after reclaim")
	}
}

func TestExternalRegisterRejectsManagedSessionID(t *testing.T) {
	s := newTestServer(t, "")
	spec := SessionSpec{ID: "managed", CWD: s.cfg.CWD, Transport: "rpc"}
	p := NewPiProcess(spec, s.cfg, s.logger)
	defer p.Close(context.Background())
	if err := s.sessions.Add(p, spec); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/v1/external-sessions/register", strings.NewReader(`{"id":"managed","bridgeId":"bridge"}`))
	rec := httptest.NewRecorder()
	s.externalRegister(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("external registration status=%d body=%s", rec.Code, rec.Body.String())
	}
}
