package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStatusHubPublishSuppressesUnchangedState(t *testing.T) {
	hub := newStatusHub()
	hub.publish("s1", "local", "working", "assistant", "Generating", "run-1")
	hub.publish("s1", "local", "working", "assistant", "Generating", "run-1")
	snap, seq := hub.snapshot()
	if len(snap) != 1 {
		t.Fatalf("want 1 snapshot entry, got %d", len(snap))
	}
	if seq != 1 {
		t.Fatalf("want seq 1 after dedup, got %d", seq)
	}
	hub.publish("s1", "local", "idle", "", "Ready", "run-1")
	events, ok := hub.replaySince(0)
	if !ok || len(events) != 2 {
		t.Fatalf("want 2 replayed events, got %d ok=%v", len(events), ok)
	}
}

func TestStatusHubKeepsQueuedRunSeparateFromSessionRuntime(t *testing.T) {
	hub := newStatusHub()
	hub.publish("s1", "local", "working", "assistant", "Generating", "run-1")
	hub.publishAdmission("s1", "local", "queued", "admission", "Waiting", "run-2", 1, time.Now())

	snapshot, _ := hub.snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("want session runtime plus queued run, got %#v", snapshot)
	}
	hub.publishAdmission("s1", "local", "cancelled", "admission", "Cancelled", "run-2", 0, time.Now())
	snapshot, _ = hub.snapshot()
	if len(snapshot) != 1 || snapshot[0].State != "working" {
		t.Fatalf("cancelled run should leave working session intact: %#v", snapshot)
	}
}

func TestStatusHubReplayAndGap(t *testing.T) {
	hub := newStatusHub()
	for i := 0; i < statusHubReplayLimit+10; i++ {
		hub.publish("s1", "local", "working", "assistant", "tick", "")
		hub.publish("s1", "local", "idle", "", "Ready", "")
	}
	// Cursor deep in the past predates the ring: caller must fall back to a snapshot.
	if _, ok := hub.replaySince(1); ok {
		t.Fatal("expected gap for ancient cursor")
	}
	// Recent cursor replays without a gap.
	_, seq := hub.snapshot()
	events, ok := hub.replaySince(seq - 2)
	if !ok || len(events) == 0 {
		t.Fatalf("expected recent replay, got %d events ok=%v", len(events), ok)
	}
}

func TestStatusHubSubscribeReceivesDeltas(t *testing.T) {
	hub := newStatusHub()
	events, unsubscribe := hub.subscribe()
	defer unsubscribe()
	hub.publish("s1", "local", "working", "assistant", "Generating", "run-1")
	select {
	case ev := <-events:
		if ev.Type != "session_status" || ev.SessionID != "s1" || ev.State != "working" {
			t.Fatalf("unexpected event: %#v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no delta received")
	}
}

func TestStatusTicketScopedToStatusSocket(t *testing.T) {
	s := newTestServer(t, "secret")
	req := httptest.NewRequest(http.MethodPost, "/v1/status-tickets", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("want 201 got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	// It must redeem against the status scope exactly once.
	if code := s.wsTickets.consumeScoped(resp.Ticket, statusTicketScope, ""); code != "" {
		t.Fatalf("status ticket rejected: %q", code)
	}
	if code := s.wsTickets.consumeScoped(resp.Ticket, statusTicketScope, ""); code != CodeInvalidTicket {
		t.Fatalf("status ticket reused: %q", code)
	}
	// Issue a second ticket to prove scope separation does not consume it.
	ticket2, _, err := s.wsTickets.issueScoped(statusTicketScope, tokenFingerprint("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if code := s.wsTickets.consumeScoped(ticket2, "session:whatever", tokenFingerprint("secret")); code != CodeTicketSessionMismatch {
		t.Fatalf("status ticket redeemed against session scope: %q", code)
	}
}

func TestStatusEventShape(t *testing.T) {
	s := newTestServer(t, "")
	events, unsubscribe := s.status.subscribe()
	defer unsubscribe()
	s.status.publish("s1", "local", "working", "assistant", "Generating response", "run-42")
	select {
	case ev := <-events:
		if ev.Type != "session_status" {
			t.Fatalf("type = %q", ev.Type)
		}
		if ev.SessionID != "s1" || ev.WorkerID != "local" || ev.State != "working" ||
			ev.Reason != "assistant" || ev.Detail != "Generating response" || ev.RunID != "run-42" {
			t.Fatalf("unexpected event: %#v", ev)
		}
		if ev.UpdatedAt.IsZero() {
			t.Fatal("updatedAt missing")
		}
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
}

func TestWorkerHealthPublishesOfflineStatus(t *testing.T) {
	s := newTestServer(t, "")
	events, unsubscribe := s.status.subscribe()
	defer unsubscribe()
	s.workers.onHealth("worker-1", false)
	select {
	case ev := <-events:
		if ev.SessionID != "" || ev.WorkerID != "worker-1" || ev.State != "offline" {
			t.Fatalf("unexpected worker event: %#v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no worker status event")
	}
}

func TestAdmissionDetailedRunsAndCancelQueued(t *testing.T) {
	a := NewTaskAdmission(1, 1, 1)
	done := make(chan struct{})
	if !a.TryAcquire("s1", "local") {
		t.Fatal("expected first acquire")
	}
	go func() {
		a.Acquire(context.Background(), "s2", "local")
		close(done)
	}()
	// Wait for the waiter to register.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(a.DetailedRuns()) == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("queued run never appeared")
		}
		time.Sleep(5 * time.Millisecond)
	}
	runs := a.DetailedRuns()
	var queued *AdmissionRun
	for i := range runs {
		if runs[i].Phase == "queued" {
			queued = &runs[i]
		}
	}
	if queued == nil || queued.SessionID != "s2" || queued.Position != 1 || queued.RunID == "" {
		t.Fatalf("unexpected queued run: %#v", runs)
	}
	if !a.CancelQueued(queued.RunID) {
		t.Fatal("CancelQueued failed")
	}
	if a.Queued() != 0 {
		t.Fatalf("queue not drained: %d", a.Queued())
	}
	select {
	case <-done:
		t.Fatal("cancelled waiter should not be granted")
	default:
	}
	a.Release("s1", "local")
}

func TestCancelRunEndpointAbortsQueuedAndActive(t *testing.T) {
	s := newTestServer(t, "")
	// Bound capacity so a second run is actually queued.
	s.admission.Reconfigure(1, 1, 1, 8)
	// Queued path: fill capacity then queue a second run.
	if !s.admission.TryAcquire("s1", "local") {
		t.Fatal("expected acquire")
	}
	acquired := make(chan struct{})
	go func() {
		s.admission.Acquire(context.Background(), "s2", "local")
		close(acquired)
	}()
	deadline := time.Now().Add(2 * time.Second)
	var queuedRun AdmissionRun
	for time.Now().Before(deadline) {
		for _, run := range s.admission.DetailedRuns() {
			if run.Phase == "queued" {
				queuedRun = run
			}
		}
		if queuedRun.RunID != "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+queuedRun.RunID+"/cancel", nil)
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "cancelled") {
		t.Fatalf("queued cancel: %d %s", w.Code, w.Body.String())
	}

	// Unknown run.
	w2 := httptest.NewRecorder()
	serve(s).ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/v1/runs/run-9999/cancel", nil))
	if w2.Code != http.StatusNotFound {
		t.Fatalf("unknown run: %d", w2.Code)
	}
	s.admission.Release("s1", "local")
	select {
	case <-acquired:
	default:
	}
}

func TestSessionInventoryRuntimeModeNeverContactsWorkers(t *testing.T) {
	hit := false
	workerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		http.NotFound(w, r)
	}))
	defer workerServer.Close()
	s := newTestServer(t, "")
	if err := s.workers.Update(Worker{ID: "w1", URL: workerServer.URL, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.remoteSessions.Add(RemoteSession{ID: "remote-1", WorkerID: "w1", WorkerSessionID: "ws-1", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions?scope=all&include=runtime", nil)
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("runtime inventory: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Sessions        []SessionSummary `json:"sessions"`
		PartialFailures []partialFailure `json:"partialFailures"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Fatal("runtime inventory contacted a worker")
	}
	if len(body.PartialFailures) != 0 {
		t.Fatalf("unexpected partial failures: %#v", body.PartialFailures)
	}
	var remote *SessionSummary
	for i := range body.Sessions {
		if body.Sessions[i].ID == "remote-1" {
			remote = &body.Sessions[i]
		}
	}
	if remote == nil || remote.Status != "mapped" {
		t.Fatalf("remote session missing or wrong status: %#v", remote)
	}
}

func TestSchedulerStatusIncludesDetailedRuns(t *testing.T) {
	s := newTestServer(t, "")
	s.admission.TryAcquire("s1", "local")
	req := httptest.NewRequest(http.MethodGet, "/v1/scheduler", nil)
	w := httptest.NewRecorder()
	serve(s).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("scheduler status: %d", w.Code)
	}
	var body struct {
		Runs []AdmissionRun `json:"runs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Runs) != 1 || body.Runs[0].Phase != "active" || body.Runs[0].SessionID != "s1" {
		t.Fatalf("unexpected runs: %#v", body.Runs)
	}
	s.admission.Release("s1", "local")
}
