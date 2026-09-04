package server

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testWriteCloser struct{ bytes.Buffer }

func (w *testWriteCloser) Close() error { return nil }

// TestExtensionUIRequiresResponseClassifiesAskUser locks in the daemon
// classification that lets clients treat the mobile ask_user dialog like the
// blocking dialog methods (select/confirm/input/editor).
func TestExtensionUIRequiresResponseClassifiesAskUser(t *testing.T) {
	for _, method := range []string{"select", "confirm", "input", "editor", "ask_user"} {
		if !extensionUIRequiresResponse(RPCEvent{"type": "extension_ui_request", "method": method}) {
			t.Fatalf("method %q should require a response", method)
		}
	}
	for _, method := range []string{"notify", "status", ""} {
		if extensionUIRequiresResponse(RPCEvent{"type": "extension_ui_request", "method": method}) {
			t.Fatalf("method %q must not require a response", method)
		}
	}
}

// TestRelayPublishStampsAndClosesAskUser verifies relayed ask_user events are
// stamped like local ones, move the session to waiting_for_input, and that
// ask:closed clears the pending request while the tool resumes working.
func TestLocalProcessTracksPendingExtensionUIRequest(t *testing.T) {
	s := newTestServer(t, "")
	p := NewPiProcess(SessionSpec{ID: "local", CWD: s.cfg.CWD}, s.cfg, s.logger)
	t.Cleanup(func() { _ = p.Close(context.Background()) })
	p.dispatch(RPCEvent{"type": "extension_ui_request", "method": "select", "id": "select-1", "title": "Choose"})

	status := p.Status()
	pending, _ := status["pendingExtensionUiRequest"].(RPCEvent)
	if pending["id"] != "select-1" {
		t.Fatalf("pending request = %#v, want select-1", pending)
	}
	p.dispatch(RPCEvent{"type": "extension_ui_closed", "id": "select-1"})
	status = p.Status()
	pending, _ = status["pendingExtensionUiRequest"].(RPCEvent)
	if len(pending) != 0 {
		t.Fatalf("pending request remained after close: %#v", pending)
	}
}

func TestLocalProcessRejectsStaleUIResponses(t *testing.T) {
	s := newTestServer(t, "")
	p := NewPiProcess(SessionSpec{ID: "local", CWD: s.cfg.CWD}, s.cfg, s.logger)
	t.Cleanup(func() { _ = p.Close(context.Background()) })
	p.dispatch(RPCEvent{"type": "extension_ui_request", "method": "input", "id": "input-1"})
	writer := &testWriteCloser{}
	p.mu.Lock()
	p.running = true
	p.stdin = writer
	p.mu.Unlock()

	if err := p.Send(RPCCommand{"type": "extension_ui_response"}); err == nil {
		t.Fatal("empty response id was accepted")
	}
	command := RPCCommand{"type": "extension_ui_response", "id": "stale"}
	if err := p.Send(command); !errors.Is(err, errExtensionUIRequestMismatch) {
		t.Fatalf("Send(%#v) error = %v, want request mismatch", command, err)
	}
	pending, _ := p.Status()["pendingExtensionUiRequest"].(RPCEvent)
	if pending["id"] != "input-1" || writer.Len() != 0 {
		t.Fatalf("stale response changed state: pending=%#v bytes=%d", pending, writer.Len())
	}

	if err := p.Send(RPCCommand{"type": "extension_ui_response", "id": "input-1", "value": "answer"}); err != nil {
		t.Fatalf("valid response failed: %v", err)
	}
	pending, _ = p.Status()["pendingExtensionUiRequest"].(RPCEvent)
	if len(pending) != 0 || writer.Len() == 0 {
		t.Fatalf("valid response was not applied: pending=%#v bytes=%d", pending, writer.Len())
	}
}

func TestRelayPublishStampsAndClosesAskUser(t *testing.T) {
	r := newExternalRegistry(t.TempDir() + "/relay-commands.json")
	r.register("s", ".", "", "", "bridge")
	ch, _, detach, ok := r.subscribe("s", 0)
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer detach()

	if !r.publish("s", RPCEvent{"type": "extension_ui_request", "method": "ask_user", "id": "ask-1", "question": "Pick one"}) {
		t.Fatal("publish failed")
	}
	select {
	case ev := <-ch:
		if requires, _ := ev["_daemonExtensionUiRequiresResponse"].(bool); !requires {
			t.Fatalf("relayed ask_user event missing requires-response stamp: %#v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive ask_user event")
	}
	snap := r.stateSnapshot("s")
	if snap["status"] != "waiting_for_input" {
		t.Fatalf("status after ask_user = %v, want waiting_for_input", snap["status"])
	}
	pending, _ := snap["pendingExtensionUiRequest"].(RPCEvent)
	if pending["id"] != "ask-1" {
		t.Fatalf("pending request = %#v, want ask-1", pending)
	}
	if !r.publish("s", RPCEvent{"type": "extension_ui_closed", "id": "stale"}) {
		t.Fatal("publish stale close failed")
	}
	snap = r.stateSnapshot("s")
	pending, _ = snap["pendingExtensionUiRequest"].(RPCEvent)
	if snap["status"] != "waiting_for_input" || pending["id"] != "ask-1" {
		t.Fatalf("stale close changed pending request: status=%v pending=%#v", snap["status"], pending)
	}

	if !r.publish("s", RPCEvent{"type": "extension_ui_closed", "id": "ask-1"}) {
		t.Fatal("publish closed failed")
	}
	snap = r.stateSnapshot("s")
	if snap["status"] != "working" {
		t.Fatalf("status after close = %v, want working", snap["status"])
	}
	pending, _ = snap["pendingExtensionUiRequest"].(RPCEvent)
	if len(pending) != 0 {
		t.Fatalf("pending request remained after close: %#v", pending)
	}

	// Non-blocking methods must not flip the session into waiting_for_input.
	if !r.publish("s", RPCEvent{"type": "extension_ui_request", "method": "notify", "message": "heads up"}) {
		t.Fatal("publish notify failed")
	}
	select {
	case ev := <-ch:
		if requires, _ := ev["_daemonExtensionUiRequiresResponse"].(bool); requires {
			t.Fatalf("notify event incorrectly stamped requires-response: %#v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive notify event")
	}
	if snap := r.stateSnapshot("s"); snap["status"] != "working" {
		t.Fatalf("status after notify = %v, want working", snap["status"])
	}
}

// TestExternalSessionUIResponseEndpoint exercises POST
// /v1/sessions/{id}/ui-response against an external relay session: every
// response field must be preserved on the queued command and an empty id must
// be rejected.
func TestExternalSessionUIResponseEndpoint(t *testing.T) {
	s := newTestServer(t, "")
	_, lease := s.external.register("ext-session", ".", "", "", "bridge-one")
	if lease == "" {
		t.Fatal("register did not issue a lease")
	}
	if !s.external.publish("ext-session", RPCEvent{"type": "extension_ui_request", "method": "ask_user", "id": "ask-1"}) {
		t.Fatal("failed to publish pending request")
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/ext-session/ui-response",
		strings.NewReader(`{"id":"ask-1","cancelled":false,"value":"custom answer","confirmed":true,"selections":["a","b"],"comment":"extra note","responseKind":"selection"}`))
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("POST ui-response = %d, want 202; body=%s", w.Code, w.Body.String())
	}

	commands, exists, authorized := s.external.commandsFor("ext-session", lease)
	if !exists || !authorized || len(commands) != 1 {
		t.Fatalf("commands after ui-response: exists=%v authorized=%v commands=%#v", exists, authorized, commands)
	}
	cmd := commands[0]
	if cmd.Type != "extension_ui_response" {
		t.Fatalf("command type = %q, want extension_ui_response", cmd.Type)
	}
	if cmd.RequestID != "ask-1" || cmd.Value != "custom answer" || cmd.Comment != "extra note" || cmd.ResponseKind != "selection" {
		t.Fatalf("response fields lost: %#v", cmd)
	}
	if cmd.Cancelled == nil || *cmd.Cancelled {
		t.Fatalf("cancelled not preserved: %#v", cmd.Cancelled)
	}
	if cmd.Confirmed == nil || !*cmd.Confirmed {
		t.Fatalf("confirmed not preserved: %#v", cmd.Confirmed)
	}
	if len(cmd.Selections) != 2 || cmd.Selections[0] != "a" || cmd.Selections[1] != "b" {
		t.Fatalf("selections not preserved: %#v", cmd.Selections)
	}

	// A second device cannot answer the already accepted request.
	req = httptest.NewRequest(http.MethodPost, "/v1/sessions/ext-session/ui-response", strings.NewReader(`{"id":"ask-1","value":"second"}`))
	w = httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate ui-response = %d, want 409; body=%s", w.Code, w.Body.String())
	}

	// Empty id must be rejected with 400.
	req = httptest.NewRequest(http.MethodPost, "/v1/sessions/ext-session/ui-response", strings.NewReader(`{"value":"x"}`))
	w = httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("ui-response without id = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

// TestExtensionUIResponseCommandPersistenceAndRelay verifies the queued
// response survives a daemon restart and reaches the bridge over the relay.
func TestExtensionUIResponseCommandPersistenceAndRelay(t *testing.T) {
	path := t.TempDir() + "/relay-commands.json"
	r1 := newExternalRegistry(path)
	r1.register("s", ".", "", "", "bridge")
	_, lease := r1.register("s", ".", "", "", "bridge")
	cancelled := true
	confirmed := false
	command := ExternalCommand{
		ID:           "cmd-1",
		Type:         "extension_ui_response",
		RequestID:    "ask-9",
		Cancelled:    &cancelled,
		Value:        "typed value",
		Confirmed:    &confirmed,
		Selections:   []string{"alpha"},
		Comment:      "a comment",
		ResponseKind: "input",
	}
	if !r1.enqueue("s", command) {
		t.Fatal("enqueue failed")
	}

	// Simulated restart: a fresh registry loads the persisted store.
	r2 := newExternalRegistry(path)
	// The lease is regenerated in-memory by the fresh registry; the command
	// queue itself is what must survive.
	_, lease = r2.register("s", ".", "", "", "bridge")
	commands, exists, authorized := r2.commandsFor("s", lease)
	if !exists || !authorized || len(commands) != 1 {
		t.Fatalf("persisted commands: exists=%v authorized=%v commands=%#v", exists, authorized, commands)
	}
	got := commands[0]
	if got.Type != "extension_ui_response" || got.RequestID != "ask-9" || got.Value != "typed value" ||
		got.Comment != "a comment" || got.ResponseKind != "input" {
		t.Fatalf("persisted command fields lost: %#v", got)
	}
	if got.Cancelled == nil || !*got.Cancelled {
		t.Fatalf("persisted cancelled not preserved: %#v", got.Cancelled)
	}
	if got.Confirmed == nil || *got.Confirmed {
		t.Fatalf("persisted confirmed not preserved: %#v", got.Confirmed)
	}
	if len(got.Selections) != 1 || got.Selections[0] != "alpha" {
		t.Fatalf("persisted selections not preserved: %#v", got.Selections)
	}

	// The relay must deliver the persisted command to the bridge. Commands
	// queued before the relay attached arrive via the pending batch; the
	// channel only carries later enqueues.
	channel, pending, _, _, detach, ok, authorized := r2.attachRelay("s", lease)
	if !ok || !authorized || channel == nil {
		t.Fatalf("relay attach: ok=%v authorized=%v", ok, authorized)
	}
	defer detach()
	if len(pending) != 1 {
		t.Fatalf("pending commands = %#v, want 1", pending)
	}
	delivered := pending[0]
	if delivered.RequestID != "ask-9" || delivered.Value != "typed value" ||
		delivered.Cancelled == nil || !*delivered.Cancelled ||
		delivered.Confirmed == nil || *delivered.Confirmed ||
		len(delivered.Selections) != 1 || delivered.Selections[0] != "alpha" ||
		delivered.Comment != "a comment" || delivered.ResponseKind != "input" {
		t.Fatalf("relay-delivered command fields lost: %#v", delivered)
	}
}
