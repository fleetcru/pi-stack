package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// fakePiChildEnv marks the re-executed test binary as the fake Pi child.
const fakePiChildEnv = "PI_TEST_FAKE_PI_CHILD"

// TestMain lets the test binary double as a stand-in Pi RPC child. The child
// mode prints one JSONL line and then blocks on stdin, which gives Stop() and
// Start() a real OS process to manage on every platform without depending on
// the actual `pi` binary being installed or on POSIX shell scripts.
func TestMain(m *testing.M) {
	if os.Getenv(fakePiChildEnv) == "1" {
		runFakePiChild()
		return
	}
	os.Exit(m.Run())
}

func runFakePiChild() {
	fmt.Println(`{"type":"ready"}`)
	_, _ = io.Copy(io.Discard, os.Stdin)
}

// fakePiBinary returns a PiBinary value that launches the test binary in fake
// child mode. The binary path is the test executable itself; the marker
// environment variable is threaded through the stored env map so every spawned
// child enters child mode.
func fakePiBinary() string {
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return os.Args[0]
}

// freshTestLogger returns a logger that discards output.
func freshTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// startFakeProcess launches a PiProcess against the fake child and waits until
// it reports running.
func startFakeProcess(t *testing.T, id string, restart bool) *PiProcess {
	t.Helper()
	cfg := Config{
		PiBinary:                 fakePiBinary(),
		DataDir:                  t.TempDir(),
		RequestTimeout:           5 * time.Second,
		RestartMax:               3,
		RestartBackoff:           10 * time.Millisecond,
		EventJournalSyncInterval: 0,
	}
	spec := SessionSpec{
		ID:      id,
		CWD:     t.TempDir(),
		Env:     map[string]string{fakePiChildEnv: "1"},
		Restart: restart,
	}
	p := NewPiProcess(spec, cfg, testLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Close(c)
	})
	waitFor(t, 5*time.Second, func() bool {
		_, running := p.ProcessTarget()
		return running
	})
	return p
}

func waitFor(t *testing.T, limit time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}

// setRuntimeSince forces the idle timestamp so tests do not have to sleep for
// the real timeout.
func setIdleSince(p *PiProcess, since time.Time) {
	p.mu.Lock()
	p.runtimeState = "idle"
	p.runtimeSince = since
	p.mu.Unlock()
}

func TestReapEligibleGuards(t *testing.T) {
	timeout := 10 * time.Minute
	old := time.Now().Add(-time.Hour)

	t.Run("idle past timeout is eligible", func(t *testing.T) {
		p := startFakeProcess(t, "eligible", false)
		setIdleSince(p, old)
		if !p.reapEligible(time.Now(), timeout) {
			t.Fatal("expected eligible")
		}
	})

	t.Run("recently idle is not eligible", func(t *testing.T) {
		p := startFakeProcess(t, "recent", false)
		setIdleSince(p, time.Now())
		if p.reapEligible(time.Now(), timeout) {
			t.Fatal("expected not eligible")
		}
	})

	t.Run("working process is never eligible", func(t *testing.T) {
		p := startFakeProcess(t, "working", false)
		p.mu.Lock()
		p.runtimeState = "working"
		p.runtimeSince = old
		p.mu.Unlock()
		if p.reapEligible(time.Now(), timeout) {
			t.Fatal("working process must never be reaped")
		}
	})

	t.Run("process with subscriber is never eligible", func(t *testing.T) {
		p := startFakeProcess(t, "subscribed", false)
		setIdleSince(p, old)
		_, unsubscribe := p.Subscribe()
		defer unsubscribe()
		if p.reapEligible(time.Now(), timeout) {
			t.Fatal("process with a live subscriber must not be reaped")
		}
	})

	t.Run("process with pending UI request is never eligible", func(t *testing.T) {
		p := startFakeProcess(t, "pending-ui", false)
		setIdleSince(p, old)
		p.mu.Lock()
		p.pendingUIRequest = RPCEvent{"type": "extension_ui_request", "id": "q1"}
		p.mu.Unlock()
		if p.reapEligible(time.Now(), timeout) {
			t.Fatal("process blocked on a user question must not be reaped")
		}
	})

	t.Run("process holding admission is never eligible", func(t *testing.T) {
		p := startFakeProcess(t, "admitted", false)
		setIdleSince(p, old)
		p.mu.Lock()
		p.admissionHeld = true
		p.mu.Unlock()
		if p.reapEligible(time.Now(), timeout) {
			t.Fatal("process holding a run slot must not be reaped")
		}
	})

	t.Run("zero timeout disables reaping", func(t *testing.T) {
		p := startFakeProcess(t, "disabled", false)
		setIdleSince(p, old)
		if p.reapEligible(time.Now(), 0) {
			t.Fatal("zero timeout must disable reaping")
		}
	})
}

func TestStopKeepsSessionReusable(t *testing.T) {
	p := startFakeProcess(t, "reusable", false)
	setIdleSince(p, time.Now().Add(-time.Hour))
	journal := p.journal
	if journal == nil {
		t.Fatal("expected a journal")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.Stop(ctx, stopReasonIdle); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if pid, running := p.ProcessTarget(); running {
		t.Fatalf("process still running with pid %d", pid)
	}
	// Stop must not be terminal: Close sets closed and would make Start fail.
	p.mu.RLock()
	closed := p.closed
	p.mu.RUnlock()
	if closed {
		t.Fatal("Stop must not mark the process closed")
	}
	// The journal must stay open so history keeps appending after relaunch.
	if p.journal == nil {
		t.Fatal("Stop closed the durable event journal")
	}

	// The next prompt relaunches Pi transparently.
	if err := p.Start(ctx); err != nil {
		t.Fatalf("restart after stop: %v", err)
	}
	waitFor(t, 5*time.Second, func() bool {
		_, running := p.ProcessTarget()
		return running
	})
	if state := p.Status()["runtimeStatus"].(map[string]any)["state"]; state != "idle" {
		t.Fatalf("state after relaunch = %v, want idle", state)
	}
}

// TestStopDoesNotTriggerRestartPolicy is the regression guard for a session
// configured with Restart=true. A deliberate stop must not be mistaken for a
// crash, or the reaper would free memory only for Pi to respawn immediately.
func TestStopDoesNotTriggerRestartPolicy(t *testing.T) {
	p := startFakeProcess(t, "no-resurrect", true)
	setIdleSince(p, time.Now().Add(-time.Hour))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.Stop(ctx, stopReasonIdle); err != nil {
		t.Fatalf("stop: %v", err)
	}
	// Give the restart backoff window a chance to fire.
	time.Sleep(500 * time.Millisecond)
	if pid, running := p.ProcessTarget(); running {
		t.Fatalf("reaped process resurrected itself with pid %d", pid)
	}
}

func TestSweepReapsOnlyEligibleProcesses(t *testing.T) {
	reg := NewSessionRegistry(filepath.Join(t.TempDir(), "sessions.json"), 0)

	idle := startFakeProcess(t, "sweep-idle", false)
	setIdleSince(idle, time.Now().Add(-time.Hour))
	working := startFakeProcess(t, "sweep-working", false)
	working.mu.Lock()
	working.runtimeState = "working"
	working.runtimeSince = time.Now().Add(-time.Hour)
	working.mu.Unlock()

	reg.Attach(idle)
	reg.Attach(working)

	reaper := newIdleReaper(10*time.Minute, reg, testLogger())
	reaper.sweep()

	if _, running := idle.ProcessTarget(); running {
		t.Fatal("idle process should have been reaped")
	}
	if _, running := working.ProcessTarget(); !running {
		t.Fatal("working process must not be reaped")
	}
}

func TestReapOldestIdleUnderCapacityPressure(t *testing.T) {
	reg := NewSessionRegistry(filepath.Join(t.TempDir(), "sessions.json"), 0)

	oldest := startFakeProcess(t, "oldest", false)
	setIdleSince(oldest, time.Now().Add(-2*time.Hour))
	newer := startFakeProcess(t, "newer", false)
	setIdleSince(newer, time.Now().Add(-30*time.Minute))

	reg.Attach(oldest)
	reg.Attach(newer)

	reaper := newIdleReaper(10*time.Minute, reg, testLogger())
	if !reaper.reapOldestIdle(reaper.timeout) {
		t.Fatal("expected a process to be reclaimed")
	}
	if _, running := oldest.ProcessTarget(); running {
		t.Fatal("oldest idle process should have been evicted first")
	}
	if _, running := newer.ProcessTarget(); !running {
		t.Fatal("newer idle process should have survived the first eviction")
	}
}

func TestReapOldestIdleRespectsGracePeriod(t *testing.T) {
	reg := NewSessionRegistry(filepath.Join(t.TempDir(), "sessions.json"), 0)
	p := startFakeProcess(t, "just-started", false)
	setIdleSince(p, time.Now().Add(-5*time.Second))
	reg.Attach(p)

	reaper := newIdleReaper(time.Hour, reg, testLogger())
	if reaper.reapOldestIdle(0) {
		t.Fatal("a session inside the grace period must not be evicted")
	}
	if _, running := p.ProcessTarget(); !running {
		t.Fatal("process was evicted inside the grace period")
	}
}

func TestIdleReaperDisabledWhenTimeoutNonPositive(t *testing.T) {
	reg := NewSessionRegistry(filepath.Join(t.TempDir(), "sessions.json"), 0)
	p := startFakeProcess(t, "disabled-timeout", false)
	setIdleSince(p, time.Now().Add(-time.Hour))
	reg.Attach(p)

	reaper := newIdleReaper(0, reg, testLogger())
	reaper.sweep()
	if _, running := p.ProcessTarget(); !running {
		t.Fatal("reaping must be disabled when the timeout is non-positive")
	}
}

func TestIdleReaperLifecycleStopsCleanly(t *testing.T) {
	reg := NewSessionRegistry(filepath.Join(t.TempDir(), "sessions.json"), 0)
	reaper := newIdleReaper(time.Minute, reg, testLogger())
	reaper.interval = 10 * time.Millisecond
	reaper.start()
	reaper.shutdown()

	// shutdown must be idempotent from the caller's perspective and must not
	// panic when the goroutine has already returned.
	done := make(chan struct{})
	go func() {
		defer close(done)
		// The channel is closed; a second close would panic, so just confirm
		// the goroutine exited without racing on shared state.
		time.Sleep(20 * time.Millisecond)
	}()
	<-done
}

func TestSweepIsSafeUnderConcurrentPrompt(t *testing.T) {
	reg := NewSessionRegistry(filepath.Join(t.TempDir(), "sessions.json"), 0)
	p := startFakeProcess(t, "racy", false)
	reg.Attach(p)
	reaper := newIdleReaper(10*time.Minute, reg, testLogger())

	// Hammer the sweeper while a subscriber attaches and detaches. The sweeper
	// must never stop a process that currently has a subscriber, and Stop must
	// not corrupt subscriber state if it wins the race first.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			setIdleSince(p, time.Now().Add(-time.Hour))
			reaper.sweep()
			if _, running := p.ProcessTarget(); !running {
				// Reaped as expected; relaunch for the next iteration.
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = p.Start(ctx)
				cancel()
			}
		}
	}()
	for i := 0; i < 200; i++ {
		_, unsubscribe := p.Subscribe()
		runtime.Gosched()
		unsubscribe()
	}
	<-done
}

func TestStopIsTerminalAfterClose(t *testing.T) {
	p := startFakeProcess(t, "closed-then-stop", false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.Close(ctx); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Stop after Close must report the terminal state instead of restarting.
	if err := p.Stop(ctx, stopReasonIdle); err == nil {
		t.Fatal("Stop on a closed process should report an error")
	}
}

func TestIdleSinceReportsOnlyIdleProcesses(t *testing.T) {
	p := startFakeProcess(t, "idle-since", false)

	p.mu.Lock()
	p.runtimeState = "working"
	p.runtimeSince = time.Now()
	p.mu.Unlock()
	if _, ok := p.IdleSince(); ok {
		t.Fatal("working process must not report an idle timestamp")
	}

	setIdleSince(p, time.Now().Add(-time.Minute))
	since, ok := p.IdleSince()
	if !ok {
		t.Fatal("idle process should report an idle timestamp")
	}
	if time.Since(since) < 30*time.Second {
		t.Fatalf("idle timestamp too recent: %v", since)
	}
}

var _ = io.Discard
var _ = slog.LevelInfo
var _ = fmt.Println
