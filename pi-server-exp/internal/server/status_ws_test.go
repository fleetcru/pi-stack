package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// dialStatusWS starts a real HTTP server for s and connects to /v1/status/ws
// with the given query parameters.
func dialStatusWS(t *testing.T, s *Server, query string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	srv := httptest.NewServer(serve(s))
	t.Cleanup(srv.Close)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/status/ws" + query
	dialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	return dialer.Dial(wsURL, nil)
}

func readStatusMessage(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var msg map[string]any
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read status message: %v", err)
	}
	return msg
}

func issueStatusTicket(t *testing.T, s *Server, auth string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/status-tickets", nil)
	req.Header.Set("Authorization", "Bearer "+auth)
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status ticket: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Ticket
}

func TestStatusWebSocketSnapshotThenLive(t *testing.T) {
	s := newTestServer(t, "")
	s.status.publish("s1", "local", "working", "assistant", "Generating", "run-1")

	conn, _, err := dialStatusWS(t, s, "")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	snap := readStatusMessage(t, conn)
	if snap["type"] != "status_snapshot" {
		t.Fatalf("want snapshot, got %#v", snap)
	}
	if gen, _ := snap["generation"].(string); gen == "" {
		t.Fatalf("snapshot missing generation: %#v", snap)
	}
	events, _ := snap["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("want 1 snapshot event, got %d", len(events))
	}

	// Live delta after the snapshot must arrive on the same connection.
	s.status.publish("s1", "local", "idle", "", "Ready", "run-1")
	live := readStatusMessage(t, conn)
	if live["type"] != "status" {
		t.Fatalf("want live status, got %#v", live)
	}
	if live["generation"] != snap["generation"] {
		t.Fatalf("generation mismatch: %v vs %v", live["generation"], snap["generation"])
	}
}

func TestStatusWebSocketReplayWithoutGap(t *testing.T) {
	s := newTestServer(t, "")
	s.status.publish("s1", "local", "working", "assistant", "Generating", "")
	_, seq := s.status.snapshot()

	conn, _, err := dialStatusWS(t, s, "?since=0&epoch="+s.status.Generation())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	msg := readStatusMessage(t, conn)
	if msg["type"] != "status_replay" && msg["type"] != "status_snapshot" {
		t.Fatalf("unexpected message type: %#v", msg)
	}
	if msg["type"] == "status_snapshot" && msg["gap"] == true {
		t.Fatalf("recent cursor must not report a gap: %#v", msg)
	}
	if cursor, ok := msg["cursor"].(float64); !ok || cursor < float64(seq) {
		t.Fatalf("unexpected cursor: %#v", msg)
	}
}

func TestStatusWebSocketEpochMismatchForcesSnapshot(t *testing.T) {
	s := newTestServer(t, "")
	s.status.publish("s1", "local", "working", "assistant", "Generating", "")

	// A cursor from a previous server lifetime (stale epoch) must resync.
	conn, _, err := dialStatusWS(t, s, "?since=99999&epoch=deadbeef")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	msg := readStatusMessage(t, conn)
	if msg["type"] != "status_snapshot" {
		t.Fatalf("want snapshot on epoch mismatch, got %#v", msg)
	}
	if msg["gap"] != true {
		t.Fatalf("epoch mismatch must flag a gap: %#v", msg)
	}
	if msg["generation"] == "deadbeef" {
		t.Fatalf("generation must come from the live hub: %#v", msg)
	}
}

func TestStatusWebSocketTicketAuthFromBrowser(t *testing.T) {
	s := newTestServer(t, "secret")

	// Without a ticket and without an Authorization header (the browser
	// WebSocket case), the auth middleware must reject the connection.
	if _, _, err := dialStatusWS(t, s, ""); err == nil {
		t.Fatal("expected rejection without ticket")
	}

	ticket := issueStatusTicket(t, s, "secret")

	// Ticket in query, no Authorization header: must upgrade and stream.
	conn, resp, err := dialStatusWS(t, s, "?ticket="+ticket)
	if err != nil {
		t.Fatalf("ticket-authenticated dial failed: %v", err)
	}
	defer conn.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("want 101, got %d", resp.StatusCode)
	}
	msg := readStatusMessage(t, conn)
	if msg["type"] != "status_snapshot" {
		t.Fatalf("want snapshot, got %#v", msg)
	}

	// Tickets are single-use: the same ticket must not connect twice.
	if _, _, err2 := dialStatusWS(t, s, "?ticket="+ticket); err2 == nil {
		t.Fatal("expected single-use ticket rejection")
	}

	// A session-scoped ticket must not open the status socket.
	sessionTicket, _, err := s.wsTickets.issueScoped("session:session-1", tokenFingerprint("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err3 := dialStatusWS(t, s, "?ticket="+sessionTicket); err3 == nil {
		t.Fatal("expected session ticket rejection on status socket")
	}
}
func TestWorkerHeartbeatCallbackOnlyOnTransition(t *testing.T) {
	r := NewWorkerRegistry("") // no persistence path: Add must not hit the filesystem
	calls := 0
	r.onHealth = func(id string, healthy bool) { calls++ }
	if err := r.Add(Worker{ID: "w1", URL: "http://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	r.Heartbeat("w1", true)
	r.Heartbeat("w1", true)
	r.Heartbeat("w1", true)
	if calls != 1 {
		t.Fatalf("steady-state heartbeats fired %d callbacks, want 1", calls)
	}
	r.Heartbeat("w1", false)
	r.Heartbeat("w1", false)
	if calls != 2 {
		t.Fatalf("unhealthy transition fired extra callbacks: %d, want 2", calls)
	}
	r.Heartbeat("w1", true)
	if calls != 3 {
		t.Fatalf("recovery transition not reported: %d, want 3", calls)
	}
}

func TestQueuedRunKeepsOriginalQueuedAtWhenActivated(t *testing.T) {
	a := NewTaskAdmission(1, 1, 1)
	if !a.TryAcquire("s1", "local") {
		t.Fatal("expected first acquire")
	}
	acquired := make(chan string, 1)
	go func() {
		runID, ok := a.AcquireRun(context.Background(), "s2", "local")
		if ok {
			acquired <- runID
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	var queued AdmissionRun
	for time.Now().Before(deadline) {
		for _, run := range a.DetailedRuns() {
			if run.Phase == "queued" && run.SessionID == "s2" {
				queued = run
			}
		}
		if queued.RunID != "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if queued.RunID == "" {
		t.Fatal("run never queued")
	}
	time.Sleep(20 * time.Millisecond) // ensure the queuedAt would differ if clobbered
	a.Release("s1", "local")

	select {
	case runID := <-acquired:
		for _, run := range a.DetailedRuns() {
			if run.RunID == runID {
				if !run.QueuedAt.Equal(queued.QueuedAt) {
					t.Fatalf("activation clobbered queuedAt: want %v got %v", queued.QueuedAt, run.QueuedAt)
				}
				return
			}
		}
		t.Fatalf("activated run %s missing from detailed runs", runID)
	case <-time.After(2 * time.Second):
		t.Fatal("waiter never granted")
	}
}

func TestPromptRejectionReleasesTurnActive(t *testing.T) {
	p, reader := runningTestProcess(t)
	defer reader.Close()
	p.cfg.RequestTimeout = 5 * time.Second

	// A prompt sent over the streaming path marks the turn active. Drain the
	// pipe concurrently so Send's write does not block.
	sendErr := make(chan error, 1)
	go func() { sendErr <- p.Send(RPCCommand{"type": "prompt", "id": "prompt-1", "message": "hi"}) }()
	readRPCCommand(t, reader)
	if err := <-sendErr; err != nil {
		t.Fatalf("send prompt: %v", err)
	}
	p.mu.RLock()
	stuck := p.turnActive
	p.mu.RUnlock()
	if !stuck {
		t.Fatal("turnActive should be set after a prompt")
	}

	// Pi answers a rejected prompt with a failed response and no agent events.
	p.dispatch(RPCEvent{"type": "response", "id": "prompt-1", "success": false, "error": "rejected"})

	p.mu.RLock()
	released := !p.turnActive && p.activePromptID == ""
	p.mu.RUnlock()
	if !released {
		t.Fatal("rejected prompt must release turnActive")
	}

	// The session must accept a new prompt afterwards.
	go func() { sendErr <- p.Send(RPCCommand{"type": "prompt", "id": "prompt-2", "message": "again"}) }()
	readRPCCommand(t, reader)
	if err := <-sendErr; err != nil {
		t.Fatalf("prompt after rejection refused: %v", err)
	}
	// A successful prompt response only acknowledges acceptance: the turn
	// stays active until Pi emits its lifecycle events.
	p.dispatch(RPCEvent{"type": "response", "id": "prompt-2", "success": true})
	p.mu.RLock()
	stillActive := p.turnActive
	p.mu.RUnlock()
	if !stillActive {
		t.Fatal("accepted prompt must keep the turn active until settled")
	}
	p.dispatch(RPCEvent{"type": "agent_settled"})
	p.mu.RLock()
	released = !p.turnActive
	p.mu.RUnlock()
	if !released {
		t.Fatal("agent_settled must release the turn")
	}
}

func TestPromptRejectionViaWaiterReleasesTurnActive(t *testing.T) {
	p, reader := runningTestProcess(t)
	defer reader.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	type reqResult struct {
		err error
	}
	result := make(chan reqResult, 1)
	go func() {
		_, err := p.Request(ctx, RPCCommand{"type": "prompt", "message": "hi"})
		result <- reqResult{err: err}
	}()
	command := readRPCCommand(t, reader)
	id, _ := command["id"].(string)
	if id == "" {
		t.Fatal("prompt request missing id")
	}

	// Pi rejects the prompt: a failed response with no agent lifecycle events.
	p.dispatch(RPCEvent{"type": "response", "id": id, "success": false, "error": "rejected"})

	select {
	case r := <-result:
		if r.err == nil {
			t.Fatal("expected error from rejected prompt request")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Request did not observe the rejection")
	}

	p.mu.RLock()
	released := !p.turnActive
	p.mu.RUnlock()
	if !released {
		t.Fatal("rejected prompt request must release turnActive")
	}
}
