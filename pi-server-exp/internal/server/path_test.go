package server

import "testing"

func TestValidSessionID(t *testing.T) {
	valid := []string{"session-1", "session_1", "ABC123"}
	for _, id := range valid {
		if !validSessionID(id) {
			t.Fatalf("expected %q to be valid", id)
		}
	}

	invalid := []string{"", "../escape", `folder\\session`, "has space", "slash/id"}
	for _, id := range invalid {
		if validSessionID(id) {
			t.Fatalf("expected %q to be invalid", id)
		}
	}
}

func TestBuildSessionSpecRejectsUnsafeID(t *testing.T) {
	s := New(Config{CWD: t.TempDir(), DataDir: t.TempDir()}, testLogger())
	if _, err := s.buildSessionSpec(createSessionRequest{ID: "../escape"}); err == nil {
		t.Fatal("expected unsafe session ID to be rejected")
	}
}
