package server

import (
	"context"
	"errors"
	"time"
)

// DefaultIdleProcessTimeout is the fallback used when no timeout is configured.
// An idle Pi RPC child holds roughly 200 MB RSS regardless of whether it is
// doing work, so keeping one alive per dormant session is the dominant memory
// cost on a multi-session hub. Twenty minutes keeps a process warm across a
// normal read/think/reply cadence while reclaiming it from sessions the user
// has genuinely walked away from.
const DefaultIdleProcessTimeout = 20 * time.Minute

// idleSweepInterval bounds how long past its deadline an idle process may
// linger. The sweeper is cheap (it only reads in-memory state), so a short
// interval costs nothing and keeps reclamation predictable.
const idleSweepInterval = 30 * time.Second

// errProcessReaped reports that a Pi child was stopped because it sat idle.
// Start() relaunches from the persisted SessionSpec, so this is recoverable
// and callers should generally retry rather than surface it to a user.
var errProcessReaped = errors.New("pi process stopped after idle timeout")

// stopReason distinguishes a deliberate idle stop from a terminal Close.
// A reaped process remains usable: only its OS child is gone.
type stopReason int

const (
	stopReasonClose stopReason = iota
	stopReasonIdle
)

// Stop halts the Pi child process while keeping the PiProcess reusable.
//
// This is the reaping counterpart to Close. Close is terminal: it sets
// p.closed, which makes every later Start() fail with "session closed" and
// closes the durable event journal. Stop must not do either, because the next
// prompt on this session has to relaunch Pi from the same spec and keep
// appending to the same journal.
//
// Stop clears the child handle, fails any in-flight waiters, releases run
// admission, and publishes a "stopped" runtime state so subscribers and
// inventory observers see the transition. The event ring and journal are left
// intact, so a reconnecting client still gets its replay.
func (p *PiProcess) Stop(ctx context.Context, reason stopReason) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return errors.New("session closed")
	}
	if !p.running || p.cmd == nil || p.cmd.Process == nil {
		p.mu.Unlock()
		return nil
	}
	cmd := p.cmd
	done := p.done
	stdin := p.stdin
	// Mark the stop before the child exits so wait() does not treat this as an
	// unexpected crash and spend the restart budget relaunching a process we
	// just deliberately reclaimed. Without this, a session with Restart=true
	// would resurrect itself the moment the sweeper stopped it.
	p.reaping = true
	if reason == stopReasonIdle {
		p.setRuntimeLocked("stopped", "idle", "Stopped after idle timeout")
	} else {
		p.setRuntimeLocked("stopped", "process", "Pi process stopped")
	}
	stateEvent := p.runtimeStateEventLocked()
	// Write the abort and close stdin under the lock, matching Close(), so a
	// concurrent Request()/Send() cannot interleave a write onto a dying pipe.
	if stdin != nil {
		_, _ = stdin.Write([]byte("{\"type\":\"abort\"}\n"))
		_ = stdin.Close()
	}
	p.stdin = nil
	p.mu.Unlock()

	p.dispatch(stateEvent)

	// Give Pi a moment to exit cleanly on the abort; escalate to a kill after
	// that, exactly like Close. A stopped child must not outlive the sweep.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return ctx.Err()
	}
	// wait() has run by now and cleared cmd/stdin/done. It observed p.reaping
	// and therefore skipped the restart path. Clear the flag so a later
	// Start() behaves normally and so a genuine crash after this point still
	// triggers the configured restart policy.
	p.mu.Lock()
	p.reaping = false
	p.mu.Unlock()
	p.logger.Info("pi rpc process stopped")
	return nil
}

// IdleSince reports when the process last became idle, and whether it is idle
// right now. "Idle" means the agent finished its turn and is waiting for the
// next prompt, as opposed to working, waiting_for_input, or failed.
func (p *PiProcess) IdleSince() (time.Time, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.running || p.runtimeState != "idle" {
		return time.Time{}, false
	}
	since := p.runtimeSince
	// A process that started but never transitioned still carries a zero
	// runtimeSince; fall back to lastEventAt so it is still eligible.
	if since.IsZero() {
		since = p.lastEventAt
	}
	if since.IsZero() {
		return time.Time{}, false
	}
	return since, true
}

// reapEligible reports whether this process may be stopped to reclaim memory.
//
// Every guard here exists because stopping the child would lose something the
// user is actively relying on:
//
//   - not running: nothing to reclaim.
//   - closed: Close already owns the shutdown.
//   - idle too briefly: a session mid-conversation must stay warm.
//   - subscribers > 0: a live WebSocket/SSE client would lose its stream.
//   - pendingUIRequest: Pi is blocked on an extension dialog; stopping it
//     discards the question the user has not answered yet.
//   - admissionHeld: a distributed run slot is checked out; releasing it by
//     force would desync the hub's admission accounting.
//   - waiting_for_input: covered by pendingUIRequest, but also guarded via the
//     state check above so a missed UI request cannot strand the user.
func (p *PiProcess) reapEligible(now time.Time, timeout time.Duration) bool {
	if timeout <= 0 {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.running || p.closed || p.reaping {
		return false
	}
	if p.runtimeState != "idle" {
		return false
	}
	if len(p.subs) > 0 {
		return false
	}
	if p.pendingUIRequest != nil {
		return false
	}
	if p.admissionHeld {
		return false
	}
	since := p.runtimeSince
	if since.IsZero() {
		since = p.lastEventAt
	}
	if since.IsZero() {
		return false
	}
	return now.Sub(since) >= timeout
}

// idleReaper stops Pi child processes that have been idle longer than the
// configured timeout, keeping the session spec and event journal so the next
// prompt transparently relaunches Pi.
type idleReaper struct {
	timeout  time.Duration
	interval time.Duration
	sessions *SessionRegistry
	logger   Logger
	stop     chan struct{}
}

// Logger is the narrow logging surface the reaper needs. It matches
// *slog.Logger so the server can pass its own logger directly.
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}

func newIdleReaper(timeout time.Duration, sessions *SessionRegistry, logger Logger) *idleReaper {
	return &idleReaper{
		timeout:  timeout,
		interval: idleSweepInterval,
		sessions: sessions,
		logger:   logger,
		stop:     make(chan struct{}),
	}
}

func (r *idleReaper) start() {
	if r.timeout <= 0 {
		// Zero or negative disables reaping entirely, matching the
		// "0 = unlimited/disabled" convention used by the other limit knobs.
		r.logger.Info("idle process reaping disabled", "timeout", r.timeout.String())
		return
	}
	r.logger.Info("idle process reaping enabled", "timeout", r.timeout.String(), "interval", r.interval.String())
	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.sweep()
			case <-r.stop:
				return
			}
		}
	}()
}

func (r *idleReaper) shutdown() {
	close(r.stop)
}

// sweep stops every eligible process. Candidates are copied under the registry
// read lock and mutated outside it, preserving the documented lock ordering
// (SessionRegistry.mu then PiProcess.mu, never the reverse).
func (r *idleReaper) sweep() {
	if r.timeout <= 0 {
		return
	}
	now := time.Now()
	for _, p := range r.sessions.processes() {
		since, ok := p.IdleSince()
		if !ok || now.Sub(since) < r.timeout {
			continue
		}
		if !p.reapEligible(now, r.timeout) {
			continue
		}
		// Re-check immediately before stopping: a prompt may have arrived
		// between the eligibility read and the child shutdown.
		if !p.reapEligible(time.Now(), r.timeout) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := p.Stop(ctx, stopReasonIdle); err != nil {
			r.logger.Warn("failed to stop idle pi process", "session", p.id, "error", err)
		} else {
			r.logger.Info("stopped idle pi process", "session", p.id, "idleFor", now.Sub(since).Round(time.Second).String())
		}
		cancel()
	}
}

// processes returns a snapshot of registered processes without holding the
// registry lock while PiProcess methods run.
func (r *SessionRegistry) processes() []*PiProcess {
	r.mu.RLock()
	out := make([]*PiProcess, 0, len(r.sessions))
	for _, p := range r.sessions {
		out = append(out, p)
	}
	r.mu.RUnlock()
	return out
}

// reapOldestIdle stops the longest-idle eligible process to free a session
// slot. Returns true when a process was reclaimed and the caller may retry its
// capacity check.
//
// This turns "max active Pi sessions reached" from a hard rejection into a
// graceful eviction: a hub at its session limit now sleeps the coldest dormant
// session instead of refusing the user's new work.
func (r *idleReaper) reapOldestIdle(within time.Duration) bool {
	if r.timeout <= 0 {
		return false
	}
	// Pressure eviction is allowed to run before the full timeout has elapsed,
	// but only after a short grace period so a session that just spawned for an
	// in-flight prompt is never the eviction victim.
	const gracePeriod = 60 * time.Second
	if within < gracePeriod {
		within = gracePeriod
	}
	now := time.Now()
	var victim *PiProcess
	var victimSince time.Time
	for _, p := range r.sessions.processes() {
		if !p.reapEligible(now, within) {
			continue
		}
		since, ok := p.IdleSince()
		if !ok {
			continue
		}
		if victim == nil || since.Before(victimSince) {
			victim, victimSince = p, since
		}
	}
	if victim == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if !victim.reapEligible(time.Now(), within) {
		return false
	}
	if err := victim.Stop(ctx, stopReasonIdle); err != nil {
		r.logger.Warn("failed to reclaim idle pi process", "session", victim.id, "error", err)
		return false
	}
	r.logger.Info("reclaimed idle pi process for capacity", "session", victim.id)
	return true
}
