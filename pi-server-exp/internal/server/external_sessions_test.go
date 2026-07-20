package server

import (
	"testing"
	"time"
)

func TestExternalCommandLeaseRejectsStalePollAndAck(t *testing.T) {
	r := newExternalRegistry(t.TempDir() + "/relay-commands.json")
	_, firstLease := r.register("external-session", ".", "", "", "bridge-one")
	if firstLease == "" {
		t.Fatal("register did not issue a lease")
	}
	// Re-registering the same bridge must retain its lease.
	_, sameLease := r.register("external-session", ".", "", "", "bridge-one")
	if sameLease != firstLease {
		t.Fatal("same bridge unexpectedly rotated its lease")
	}
	if !r.enqueue("external-session", ExternalCommand{ID: "command-1", Type: "prompt", Message: "hello"}) {
		t.Fatal("failed to enqueue command")
	}

	commands, exists, authorized := r.commandsFor("external-session", firstLease)
	if !exists || !authorized || len(commands) != 1 {
		t.Fatalf("current lease could not poll: exists=%v authorized=%v commands=%#v", exists, authorized, commands)
	}

	_, secondLease := r.register("external-session", ".", "", "", "bridge-two")
	if secondLease == "" || secondLease == firstLease {
		t.Fatal("new bridge did not rotate the lease")
	}
	if _, exists, authorized := r.commandsFor("external-session", firstLease); !exists || authorized {
		t.Fatalf("stale lease polled commands: exists=%v authorized=%v", exists, authorized)
	}
	if _, _, _, _, exists, authorized := r.attachRelay("external-session", firstLease); !exists || authorized {
		t.Fatalf("stale lease attached relay: exists=%v authorized=%v", exists, authorized)
	}
	if exists, authorized := r.acknowledgeForLease("external-session", []string{"command-1"}, firstLease); !exists || authorized {
		t.Fatalf("stale lease acknowledged command: exists=%v authorized=%v", exists, authorized)
	}

	commands, exists, authorized = r.commandsFor("external-session", secondLease)
	if !exists || !authorized || len(commands) != 1 || commands[0].ID != "command-1" {
		t.Fatalf("new lease did not receive retained command: exists=%v authorized=%v commands=%#v", exists, authorized, commands)
	}
	if exists, authorized := r.acknowledgeForLease("external-session", []string{"command-1"}, secondLease); !exists || !authorized {
		t.Fatalf("current lease could not acknowledge: exists=%v authorized=%v", exists, authorized)
	}
	commands, _, authorized = r.commandsFor("external-session", secondLease)
	if !authorized || len(commands) != 0 {
		t.Fatalf("acknowledged command remained queued: %#v", commands)
	}
}

func TestRelayCommandPersistenceAcrossRestart(t *testing.T) {
	path := t.TempDir() + "/relay-commands.json"
	// First registry instance: enqueue commands.
	r1 := newExternalRegistry(path)
	r1.register("session-1", ".", "", "", "bridge-1")
	if !r1.enqueue("session-1", ExternalCommand{ID: "cmd-1", Type: "prompt", Message: "first"}) {
		t.Fatal("failed to enqueue first command")
	}
	if !r1.enqueue("session-1", ExternalCommand{ID: "cmd-2", Type: "prompt", Message: "second"}) {
		t.Fatal("failed to enqueue second command")
	}
	// Second registry instance simulates a server restart.
	r2 := newExternalRegistry(path)
	r2.register("session-1", ".", "", "", "bridge-1")
	commands, exists, authorized := r2.commandsFor("session-1", r2.sessions["session-1"].leaseToken)
	if !exists || !authorized {
		t.Fatalf("session not found after restart: exists=%v authorized=%v", exists, authorized)
	}
	if len(commands) != 2 {
		t.Fatalf("expected 2 persisted commands, got %d: %#v", len(commands), commands)
	}
	if commands[0].ID != "cmd-1" || commands[1].ID != "cmd-2" {
		t.Fatalf("command order not preserved: %#v", commands)
	}
}

func TestRelayCommandPersistenceClearedOnAck(t *testing.T) {
	path := t.TempDir() + "/relay-commands.json"
	r1 := newExternalRegistry(path)
	r1.register("s", ".", "", "", "bridge")
	if !r1.enqueue("s", ExternalCommand{ID: "cmd-1", Type: "prompt", Message: "hello"}) {
		t.Fatal("enqueue failed")
	}
	r1.acknowledgeForLease("s", []string{"cmd-1"}, r1.sessions["s"].leaseToken)
	// After ack, the persisted store should be empty for this session.
	r2 := newExternalRegistry(path)
	r2.register("s", ".", "", "", "bridge")
	commands, _, _ := r2.commandsFor("s", r2.sessions["s"].leaseToken)
	if len(commands) != 0 {
		t.Fatalf("acknowledged command persisted: %#v", commands)
	}
}

func TestTicketDeletionOnValidationFailure(t *testing.T) {
	st := newWSTicketStore()
	// Session mismatch: ticket should be deleted.
	ticket1, _, _ := st.issue("s1", "fp")
	if code := st.consume(ticket1, "wrong-session", "fp"); code != CodeTicketSessionMismatch {
		t.Fatalf("want %q, got %q", CodeTicketSessionMismatch, code)
	}
	if code := st.consume(ticket1, "s1", "fp"); code != CodeInvalidTicket {
		t.Fatalf("ticket should be deleted after mismatch, got %q", code)
	}
	// Fingerprint mismatch: ticket should be deleted.
	ticket2, _, _ := st.issue("s2", "fp")
	if code := st.consume(ticket2, "s2", "wrong-fp"); code != CodeInvalidTicket {
		t.Fatalf("want %q, got %q", CodeInvalidTicket, code)
	}
	if code := st.consume(ticket2, "s2", "fp"); code != CodeInvalidTicket {
		t.Fatalf("ticket should be deleted after fp mismatch, got %q", code)
	}
}

func TestHistoryCacheInvalidation(t *testing.T) {
	s := newTestServer(t, "")
	spec := SessionSpec{ID: "s1", CWD: s.cfg.CWD, Status: "created", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	p := NewPiProcess(spec, s.cfg, s.logger)
	p.onMessageEnd = func() { s.invalidateHistoryCache("s1") }
	if err := s.sessions.Add(p, spec); err != nil {
		t.Fatal(err)
	}
	// Seed the cache.
	s.historyMu.Lock()
	s.historyCache["s1"] = historyCacheEntry{messages: []any{"old"}, expires: time.Now().Add(20 * time.Second)}
	s.historyMu.Unlock()
	// Verify cache exists.
	s.historyMu.Lock()
	if _, ok := s.historyCache["s1"]; !ok {
		t.Fatal("cache not seeded")
	}
	s.historyMu.Unlock()
	// Simulate message_end dispatch.
	p.dispatch(RPCEvent{"type": "message_end", "message": map[string]any{"role": "assistant"}})
	// Cache should be invalidated.
	s.historyMu.Lock()
	_, ok := s.historyCache["s1"]
	s.historyMu.Unlock()
	if ok {
		t.Fatal("cache should have been invalidated after message_end")
	}
}
