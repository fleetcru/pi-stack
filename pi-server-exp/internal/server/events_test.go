package server

import (
	"context"
	"runtime"
	"testing"
	"time"
)

// TestEventHistoryDefaultsAgree pins the two places that define the event-ring
// bounds to the same values. ConfigFromEnv seeds Config, while NewPiProcess
// falls back for a hand-built Config; if those drift, a session created through
// one path retains a different amount of history than the other, and the
// documented defaults in README stop being true.
func TestEventHistoryDefaultsAgree(t *testing.T) {
	t.Setenv("PI_SERVER_DATA_DIR", t.TempDir())
	t.Setenv("PI_SERVER_EVENT_HISTORY_MAX", "")
	t.Setenv("PI_SERVER_EVENT_HISTORY_BYTES", "")

	cfg := ConfigFromEnv()
	if cfg.EventHistoryMax != DefaultEventHistoryMax {
		t.Fatalf("ConfigFromEnv max = %d, want %d", cfg.EventHistoryMax, DefaultEventHistoryMax)
	}
	if cfg.EventHistoryBytes != DefaultEventHistoryBytes {
		t.Fatalf("ConfigFromEnv bytes = %d, want %d", cfg.EventHistoryBytes, DefaultEventHistoryBytes)
	}

	// A zero-value Config must fall back to exactly the same bounds.
	p := NewPiProcess(SessionSpec{ID: "defaults", CWD: t.TempDir()}, Config{DataDir: t.TempDir()}, testLogger())
	if p.eventMax != DefaultEventHistoryMax {
		t.Fatalf("NewPiProcess max = %d, want %d", p.eventMax, DefaultEventHistoryMax)
	}
	if p.eventMaxBytes != DefaultEventHistoryBytes {
		t.Fatalf("NewPiProcess bytes = %d, want %d", p.eventMaxBytes, DefaultEventHistoryBytes)
	}
	// Release the journal handle so Windows can remove the temp directory.
	if p.journal != nil {
		if err := p.journal.close(); err != nil {
			t.Fatalf("close journal: %v", err)
		}
	}
}

// TestDispatchDoesNotRaceUnsubscribe is a regression guard for a
// send-on-closed-channel panic. dispatch() used to copy the subscriber set
// under the write lock, release the lock, then send. unsubscribe() closes a
// channel under that same lock, so a concurrent unsubscribe could close a
// channel that dispatch was about to send on. The panic surfaced under
// Windows CI when Start() raced a subscriber detaching.
func TestDispatchDoesNotRaceUnsubscribe(t *testing.T) {
	p := NewPiProcess(SessionSpec{ID: "subscribe-race", CWD: t.TempDir()}, Config{DataDir: t.TempDir()}, testLogger())
	defer func() {
		if p.journal != nil {
			_ = p.journal.close()
		}
	}()

	const iterations = 300
	done := make(chan struct{})

	// Producer: continuously emit events while subscribers churn.
	go func() {
		defer close(done)
		for i := 0; i < iterations; i++ {
			p.dispatch(RPCEvent{"type": "message_update", "delta": "x"})
			p.dispatchRuntimeState(RPCEvent{
				"type":          "runtime_state",
				"runtimeState":  "working",
				"runtimeReason": "assistant",
			})
		}
	}()

	// Consumer churn: subscribe and immediately unsubscribe.
	for i := 0; i < iterations; i++ {
		_, unsubscribe := p.Subscribe()
		runtime.Gosched()
		unsubscribe()
	}
	<-done
}

func TestEventHistoryRespectsByteBudget(t *testing.T) {
	p := NewPiProcess(SessionSpec{ID: "s", CWD: "."}, Config{EventHistoryMax: 10, EventHistoryBytes: 100}, testLogger())
	p.dispatch(RPCEvent{"type": "one", "data": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	p.dispatch(RPCEvent{"type": "two", "data": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"})

	records := p.Events(10, 0)
	if len(records) != 1 || records[0].Event["type"] != "two" {
		t.Fatalf("history did not evict to its byte budget: %#v", records)
	}
	if p.eventBytes > p.eventMaxBytes {
		t.Fatalf("retained %d bytes, limit is %d", p.eventBytes, p.eventMaxBytes)
	}
}

func TestEventHistorySkipsOversizedEvent(t *testing.T) {
	p := NewPiProcess(SessionSpec{ID: "s", CWD: "."}, Config{EventHistoryBytes: 50}, testLogger())
	p.dispatch(RPCEvent{"type": "large", "data": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})

	if records := p.Events(10, 0); len(records) != 0 {
		t.Fatalf("oversized event was retained: %#v", records)
	}
}

func TestEventReplaySignalsHistoryGap(t *testing.T) {
	p := NewPiProcess(SessionSpec{ID: "s", CWD: "."}, Config{EventHistoryMax: 2}, testLogger())
	p.dispatch(RPCEvent{"type": "one"})
	p.dispatch(RPCEvent{"type": "two"})
	p.dispatch(RPCEvent{"type": "three"})
	p.dispatch(RPCEvent{"type": "four"})

	_, replay, close := p.SubscribeSince(1)
	defer close()
	if len(replay) != 3 || replay[0].ID != 0 || replay[0].Event["type"] != "events_lost" {
		t.Fatalf("missing events_lost marker: %#v", replay)
	}
	if replay[0].Event["expectedAfter"] != uint64(1) || replay[0].Event["received"] != uint64(3) {
		t.Fatalf("wrong gap marker: %#v", replay[0].Event)
	}
}

func TestCloseReleasesAdmissionWithoutLifecycleEvent(t *testing.T) {
	p := NewPiProcess(SessionSpec{ID: "s", CWD: "."}, Config{}, testLogger())
	releases := 0
	if !p.holdAdmission(func() { releases++ }) {
		t.Fatal("could not hold admission")
	}
	if err := p.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if releases != 1 {
		t.Fatalf("release count = %d, want 1", releases)
	}
}

func TestAgentSettledReleasesAdmissionOnce(t *testing.T) {
	p := NewPiProcess(SessionSpec{ID: "s", CWD: "."}, Config{}, testLogger())
	releases := 0
	if !p.holdAdmission(func() { releases++ }) {
		t.Fatal("could not hold admission")
	}
	p.dispatch(RPCEvent{"type": "agent_end"})
	p.dispatch(RPCEvent{"type": "agent_settled"})
	if releases != 1 {
		t.Fatalf("release count = %d, want 1", releases)
	}
}

func TestStatusReportsEventHealth(t *testing.T) {
	p := NewPiProcess(SessionSpec{ID: "s", CWD: "."}, Config{}, testLogger())
	p.dispatch(RPCEvent{"type": "message_start"})
	status := p.Status()
	if status["lastEventAt"].(time.Time).IsZero() {
		t.Fatalf("missing last event timestamp: %#v", status)
	}
	if status["droppedEvents"] != uint64(0) {
		t.Fatalf("unexpected dropped event count: %#v", status)
	}
}

func TestEventIncludesTaskAndRunMetadata(t *testing.T) {
	p := NewPiProcess(SessionSpec{ID: "task-1", CWD: "."}, Config{}, testLogger())
	ch, _, close := p.SubscribeSince(0)
	defer close()
	p.dispatch(RPCEvent{"type": "agent_start"})
	event := <-ch
	if event["_daemonTaskId"] != "task-1" {
		t.Fatalf("missing task metadata: %#v", event)
	}
	if runID, ok := event["_daemonRunId"].(string); !ok || runID == "" {
		t.Fatalf("missing run metadata: %#v", event)
	}
}

func TestEventCursorReplay(t *testing.T) {
	p := NewPiProcess(SessionSpec{ID: "s", CWD: "."}, Config{}, testLogger())
	p.dispatch(RPCEvent{"type": "one"})
	p.dispatch(RPCEvent{"type": "two"})
	records := p.Events(10, 1)
	if len(records) != 1 || records[0].ID != 2 {
		t.Fatalf("unexpected records %#v", records)
	}
	ch, replay, close := p.SubscribeSince(1)
	defer close()
	if len(replay) != 1 || replay[0].ID != 2 {
		t.Fatal("replay cursor failed")
	}
	p.dispatch(RPCEvent{"type": "three"})
	event := <-ch
	if event["_daemonEventId"] != uint64(3) {
		t.Fatalf("missing event id: %#v", event)
	}
}
