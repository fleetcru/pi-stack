package server

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIdempotencySurvivesServerRestart(t *testing.T) {
	dataDir := t.TempDir()
	first := &Server{cfg: Config{DataDir: dataDir}, logger: slog.Default()}
	if first.checkIdempotency("session:key") {
		t.Fatal("first request should not be treated as a duplicate")
	}
	second := &Server{cfg: Config{DataDir: dataDir}, logger: slog.Default()}
	if !second.checkIdempotency("session:key") {
		t.Fatal("persisted request should be treated as a duplicate")
	}
}

func TestExpiredIdempotencyKeysAreDropped(t *testing.T) {
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "idempotency.json")
	if err := os.WriteFile(path, []byte("{\"expired\":\"2000-01-01T00:00:00Z\"}"), 0o640); err != nil {
		t.Fatal(err)
	}
	server := &Server{cfg: Config{DataDir: dataDir}, logger: slog.Default()}
	if server.idempotencySeen("expired") {
		t.Fatal("expired key should not be seen")
	}
	if len(server.idempotency) != 0 {
		t.Fatalf("expired key was retained: %+v", server.idempotency)
	}
	_ = time.Now()
}
