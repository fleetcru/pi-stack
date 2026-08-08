package server

import (
	"context"
	"testing"
	"time"
)

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
