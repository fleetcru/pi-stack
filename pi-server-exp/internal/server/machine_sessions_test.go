package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMatchingServerSessionResolvesBridgeSymlink(t *testing.T) {
	managedDir := filepath.Join(t.TempDir(), "managed", "session-1")
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(managedDir, "history.jsonl")
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nativeDir := filepath.Join(t.TempDir(), "native")
	if err := os.MkdirAll(nativeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(nativeDir, "session-1.jsonl")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	specs := []SessionSpec{{ID: "server-session-1", ManagedSessionDir: managedDir, Transport: "rpc"}}
	got := matchingServerSession(MachineSession{ID: "pi-history-id", Path: link}, specs)
	if got == nil || got.ID != "server-session-1" {
		t.Fatalf("matchingServerSession() = %#v, want server-session-1", got)
	}
}

func TestMatchingServerSessionMatchesExplicitPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	specs := []SessionSpec{{ID: "server-session-2", SessionPath: path, Transport: "rpc"}}
	got := matchingServerSession(MachineSession{Path: path}, specs)
	if got == nil || got.ID != "server-session-2" {
		t.Fatalf("matchingServerSession() = %#v, want server-session-2", got)
	}
}

func TestMatchingServerSessionIgnoresRelaySpecs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	specs := []SessionSpec{{ID: "relay-session", SessionPath: path, Transport: "relay"}}
	if got := matchingServerSession(MachineSession{Path: path}, specs); got != nil {
		t.Fatalf("matchingServerSession() = %#v, want nil", got)
	}
}

func TestLiveRelayOwnsDuplicateSessionHistory(t *testing.T) {
	history := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(history, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rpc := SessionSpec{ID: "rpc-session", SessionPath: history, Transport: "rpc"}
	relay := SessionSpec{ID: "relay-session", SessionPath: history, Transport: "relay"}
	specs := []SessionSpec{rpc, relay}
	s := newTestServer(t, "")
	s.external.register(relay.ID, ".", "", history, "bridge")

	preferred := s.preferredServerSession(MachineSession{Path: history}, specs)
	if preferred == nil || preferred.ID != relay.ID {
		t.Fatalf("preferred session = %#v, want live relay", preferred)
	}
	if !s.hideDuplicateSessionSpec(rpc, specs) {
		t.Fatal("duplicate RPC session remained visible while relay owns history")
	}
	if s.hideDuplicateSessionSpec(relay, specs) {
		t.Fatal("live relay session was hidden")
	}
}

func TestStaleDisconnectedRelayDoesNotHideRPCOwner(t *testing.T) {
	history := filepath.Join(t.TempDir(), "history.jsonl")
	if err := os.WriteFile(history, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rpc := SessionSpec{ID: "rpc-session", SessionPath: history, Transport: "rpc"}
	relay := SessionSpec{ID: "relay-session", SessionPath: history, Transport: "relay"}
	specs := []SessionSpec{rpc, relay}
	s := newTestServer(t, "")
	s.external.register(relay.ID, ".", "", history, "bridge")
	s.external.mu.Lock()
	s.external.sessions[relay.ID].RelayConnected = false
	s.external.sessions[relay.ID].UpdatedAt = time.Now().Add(-2 * time.Minute)
	s.external.mu.Unlock()

	preferred := s.preferredServerSession(MachineSession{Path: history}, specs)
	if preferred == nil || preferred.ID != rpc.ID {
		t.Fatalf("preferred session = %#v, want RPC after relay became stale", preferred)
	}
	if s.hideDuplicateSessionSpec(rpc, specs) {
		t.Fatal("stale disconnected relay hid the RPC owner")
	}
}

func TestDuplicateRelaySpecMatchesManagedRPCSession(t *testing.T) {
	managedDir := t.TempDir()
	history := filepath.Join(managedDir, "history.jsonl")
	if err := os.WriteFile(history, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rpc := SessionSpec{ID: "rpc-session", ManagedSessionDir: managedDir, Transport: "rpc"}
	relay := SessionSpec{ID: "relay-session", SessionPath: history, Transport: "relay"}
	specs := []SessionSpec{rpc, relay}
	if !duplicateRelaySpec(relay, specs) {
		t.Fatal("duplicateRelaySpec() = false, want true")
	}
	if duplicateRelaySpec(rpc, specs) {
		t.Fatal("duplicateRelaySpec() marked the RPC session as a duplicate")
	}
}
