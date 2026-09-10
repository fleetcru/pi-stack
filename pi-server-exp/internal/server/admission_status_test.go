package server

import (
	"context"
	"sync"
	"testing"
	"time"
)

// waitForAdmissionChange polls until the observer recorded a matching change.
func waitForAdmissionChange(t *testing.T, collect func() []admissionChange, match func(admissionChange) bool) admissionChange {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, change := range collect() {
			if match(change) {
				return change
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("expected admission change never arrived")
	return admissionChange{}
}

func newObservedAdmission(t *testing.T) (*TaskAdmission, func() []admissionChange) {
	t.Helper()
	a := NewTaskAdmissionWithQueue(1, 1, 1, 8)
	var mu sync.Mutex
	var changes []admissionChange
	a.SetOnChange(func(change admissionChange) {
		a.Queued() // must never run while the admission mutex is held
		mu.Lock()
		changes = append(changes, change)
		mu.Unlock()
	})
	return a, func() []admissionChange {
		mu.Lock()
		defer mu.Unlock()
		return append([]admissionChange(nil), changes...)
	}
}

func TestAdmissionQueueEnqueueNotification(t *testing.T) {
	a, changes := newObservedAdmission(t)
	if !a.TryAcquire("s1", "w1") {
		t.Fatal("first acquire failed")
	}
	granted := make(chan string, 1)
	go func() {
		runID, ok := a.AcquireRun(context.Background(), "s2", "w2")
		if ok {
			granted <- runID
		}
	}()
	queued := waitForAdmissionChange(t, changes, func(c admissionChange) bool { return c.Kind == "queued" })
	if queued.Run.SessionID != "s2" || queued.Run.Phase != "queued" {
		t.Fatalf("unexpected queued change: %#v", queued)
	}
	if queued.Run.Position != 1 {
		t.Fatalf("want position 1, got %d", queued.Run.Position)
	}
	if queued.Run.RunID == "" || queued.Run.QueuedAt.IsZero() {
		t.Fatalf("queued change missing runId or queuedAt: %#v", queued.Run)
	}
	select {
	case runID := <-granted:
		t.Fatalf("run %s granted prematurely", runID)
	default:
	}
}

func TestAdmissionQueuePromotionNotification(t *testing.T) {
	a, changes := newObservedAdmission(t)
	if !a.TryAcquire("s1", "w1") {
		t.Fatal("first acquire failed")
	}
	granted := make(chan string, 1)
	go func() {
		if runID, ok := a.AcquireRun(context.Background(), "s2", "w2"); ok {
			granted <- runID
		}
	}()
	queued := waitForAdmissionChange(t, changes, func(c admissionChange) bool { return c.Kind == "queued" })
	a.Release("s1", "w1")
	runID := <-granted
	if runID != queued.Run.RunID {
		t.Fatalf("granted runId %s does not match queued runId %s", runID, queued.Run.RunID)
	}
	promoted := waitForAdmissionChange(t, changes, func(c admissionChange) bool { return c.Kind == "granted" })
	if promoted.Run.RunID != runID || promoted.Run.Phase != "active" {
		t.Fatalf("unexpected granted change: %#v", promoted)
	}
}

func TestAdmissionQueuePositionUpdate(t *testing.T) {
	a, changes := newObservedAdmission(t)
	if !a.TryAcquire("s1", "w1") {
		t.Fatal("first acquire failed")
	}
	release := make(chan string, 2)
	for _, id := range []string{"s2", "s3"} {
		go func(sessionID string) {
			if runID, ok := a.AcquireRun(context.Background(), sessionID, "w"+sessionID[1:]); ok {
				release <- runID
			}
		}(id)
	}
	second := waitForAdmissionChange(t, changes, func(c admissionChange) bool { return c.Kind == "queued" && c.Run.Position == 2 })
	first := waitForAdmissionChange(t, changes, func(c admissionChange) bool {
		return c.Kind == "queued" && c.Run.Position == 1 && c.Run.RunID != second.Run.RunID
	})
	if !a.CancelQueued(first.Run.RunID) {
		t.Fatal("CancelQueued failed")
	}
	// The surviving waiter must be republished at position 1.
	promoted := waitForAdmissionChange(t, changes, func(c admissionChange) bool {
		return c.Kind == "queued" && c.Run.RunID == second.Run.RunID && c.Run.Position == 1
	})
	if promoted.Run.Phase != "queued" {
		t.Fatalf("unexpected phase: %q", promoted.Run.Phase)
	}
	a.Release("s1", "w1")
	if runID := <-release; runID != second.Run.RunID {
		t.Fatalf("expected surviving waiter %s to be granted, got %s", second.Run.RunID, runID)
	}
}

func TestAdmissionCancelWakesBlockedAcquire(t *testing.T) {
	a, _ := newObservedAdmission(t)
	if !a.TryAcquire("s1", "w1") {
		t.Fatal("first acquire failed")
	}
	// Long context: only explicit cancellation may end the wait promptly.
	result := make(chan bool, 1)
	go func() {
		_, ok := a.AcquireRun(context.Background(), "s2", "w2")
		result <- ok
	}()
	var queuedRunID string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, run := range a.DetailedRuns() {
			if run.Phase == "queued" && run.SessionID == "s2" {
				queuedRunID = run.RunID
			}
		}
		if queuedRunID != "" {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if queuedRunID == "" {
		t.Fatal("run never queued")
	}
	if !a.CancelQueued(queuedRunID) {
		t.Fatal("CancelQueued failed")
	}
	select {
	case ok := <-result:
		if ok {
			t.Fatal("cancelled AcquireRun reported success")
		}
	case <-time.After(time.Second):
		t.Fatal("CancelQueued did not wake the blocked AcquireRun")
	}
	// Capacity must be unchanged by the cancelled wait.
	if a.Active() != 1 {
		t.Fatalf("want 1 active run, got %d", a.Active())
	}
}

func TestStatusStreamCarriesAdmissionQueuePayload(t *testing.T) {
	hub := newStatusHub()
	events, unsubscribe := hub.subscribe()
	defer unsubscribe()
	queuedAt := time.Now().UTC().Add(-time.Second)
	hub.publishAdmission("s1", "local", "queued", "admission", "Run waiting in admission queue", "run-7", 1, queuedAt)
	hub.publishAdmission("s1", "local", "queued", "admission", "Run waiting in admission queue", "run-7", 1, queuedAt) // deduped
	hub.publishAdmission("s1", "local", "queued", "admission", "Run waiting in admission queue", "run-7", 2, queuedAt) // position change

	received := make([]StatusEvent, 0, 2)
	deadline := time.After(time.Second)
	for len(received) < 2 {
		select {
		case ev := <-events:
			received = append(received, ev)
		case <-deadline:
			t.Fatalf("expected 2 deltas, got %d", len(received))
		}
	}
	first, second := received[0], received[1]
	if first.State != "queued" || first.RunID != "run-7" || first.Position != 1 {
		t.Fatalf("unexpected first event: %#v", first)
	}
	if first.QueuedAt == nil || !first.QueuedAt.Equal(queuedAt) {
		t.Fatalf("queuedAt not propagated: %#v", first.QueuedAt)
	}
	if second.Position != 2 {
		t.Fatalf("position change was deduped away: %#v", second)
	}
	// A detail change must also pierce dedup at the same state.
	hub.publishAdmission("s1", "local", "queued", "admission", "Queue jumped", "run-7", 2, queuedAt)
	select {
	case ev := <-events:
		if ev.Detail != "Queue jumped" {
			t.Fatalf("detail change was deduped away: %#v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("detail change delta missing")
	}
	// And the grant transition publishes "starting" with the same runId.
	hub.publishAdmission("s1", "local", "starting", "admission", "Run admitted from queue", "run-7", 0, queuedAt)
	select {
	case ev := <-events:
		if ev.State != "starting" || ev.RunID != "run-7" {
			t.Fatalf("unexpected starting event: %#v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("starting delta missing")
	}
}

// Regression: when a queued AcquireRun's context is done at the same moment
// CancelQueued removes the waiter, the context branch must not release
// capacity the waiter never held. A wrongful release steals the slot of the
// other active run on the same session/worker and over-admits the hub.
func TestAcquireRunContextDoneAfterCancelQueuedKeepsCounters(t *testing.T) {
	const iterations = 100
	for i := 0; i < iterations; i++ {
		a := NewTaskAdmission(1, 2, 2)
		// Another run holds the same session/worker so a wrongful release has
		// visible counter effects instead of hitting the zero guards.
		if !a.TryAcquire("s", "w") {
			t.Fatal("holder acquire failed")
		}
		var once sync.Once
		ctx, cancel := context.WithCancel(context.Background())
		a.SetOnChange(func(change admissionChange) {
			if change.Kind != "queued" {
				return
			}
			// Runs inside the AcquireRun goroutine just before it reaches the
			// select: both ctx.Done and waiter.cancelled become ready, so the
			// select exercises the context-vs-cancelled race on every pass.
			once.Do(func() {
				if !a.CancelQueued(change.Run.RunID) {
					t.Errorf("iteration %d: CancelQueued failed", i)
				}
				cancel()
			})
		})
		result := make(chan bool, 1)
		go func() {
			_, ok := a.AcquireRun(ctx, "s", "w")
			result <- ok
		}()
		if ok := <-result; ok {
			t.Fatalf("iteration %d: cancelled AcquireRun reported success", i)
		}
		// The holder must still occupy its slot.
		snap := a.Snapshot()
		if snap.Active != 1 || snap.Sessions["s"] != 1 || snap.Workers["w"] != 1 {
			t.Fatalf("iteration %d: counters corrupted after cancelled wait: %+v", i, snap)
		}
		if a.TryAcquire("other", "other") {
			t.Fatalf("iteration %d: cancelled wait over-admitted capacity", i)
		}
	}
}

// Regression: a context done racing a grant must consume the grant exactly
// once so counters stay balanced whichever select case wins.
func TestAcquireRunContextDoneRacingGrantKeepsCounters(t *testing.T) {
	a := NewTaskAdmission(1, 1, 1)
	if !a.TryAcquire("holder", "holder") {
		t.Fatal("holder acquire failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan bool, 1)
	go func() {
		_, ok := a.AcquireRun(ctx, "s", "w")
		result <- ok
	}()
	deadline := time.Now().Add(2 * time.Second)
	for a.Queued() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("run never queued")
		}
		time.Sleep(time.Millisecond)
	}
	// Grant the waiter, then cancel so both select cases may be ready.
	a.Release("holder", "holder")
	cancel()
	ok := <-result
	snap := a.Snapshot()
	if ok {
		// The grant was consumed normally; its run is active.
		if snap.Active != 1 || snap.Sessions["s"] != 1 || snap.Workers["w"] != 1 {
			t.Fatalf("granted run counters wrong: %+v", snap)
		}
		a.Release("s", "w")
	} else {
		// The context branch consumed the raced grant via releaseLocked;
		// counters must already be balanced.
		if snap.Active != 0 || len(snap.Sessions) != 0 || len(snap.Workers) != 0 {
			t.Fatalf("counters unbalanced after grant race: %+v", snap)
		}
	}
	final := a.Snapshot()
	if final.Active != 0 || len(final.Sessions) != 0 || len(final.Workers) != 0 {
		t.Fatalf("final counters unbalanced: %+v", final)
	}
	if !a.TryAcquire("s", "w") {
		t.Fatal("capacity not restored after grant race")
	}
}
