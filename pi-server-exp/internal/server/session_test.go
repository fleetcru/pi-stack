package server

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
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
