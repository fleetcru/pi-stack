package server

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AdmissionRun is an observable queued or active run record.
type AdmissionRun struct {
	RunID     string    `json:"runId"`
	SessionID string    `json:"sessionId"`
	WorkerID  string    `json:"workerId"`
	Phase     string    `json:"phase"`
	Position  int       `json:"position,omitempty"`
	QueuedAt  time.Time `json:"queuedAt"`
}

type admissionRunRecord struct {
	sessionID string
	workerID  string
	queuedAt  time.Time
}

type admissionWaiter struct {
	sessionID string
	workerID  string
	runID     string
	queuedAt  time.Time
	granted   chan struct{}
}

// TaskAdmission tracks active runs and grants queued work deterministically.
type TaskAdmission struct {
	mu         sync.Mutex
	globalMax  int
	perSession int
	perWorker  int
	active     int
	sessions   map[string]int
	workers    map[string]int
	maxQueued  int
	waiters    []*admissionWaiter
	activeRuns map[string]admissionRunRecord
	runSeq     uint64
}

func NewTaskAdmission(globalMax, perSession, perWorker int) *TaskAdmission {
	return NewTaskAdmissionWithQueue(globalMax, perSession, perWorker, 32)
}

func NewTaskAdmissionWithQueue(globalMax, perSession, perWorker, maxQueued int) *TaskAdmission {
	return &TaskAdmission{globalMax: globalMax, perSession: perSession, perWorker: perWorker, maxQueued: maxQueued, sessions: map[string]int{}, workers: map[string]int{}, activeRuns: map[string]admissionRunRecord{}}
}

// newRunIDLocked allocates an observable run identifier. Caller holds mu.
func (a *TaskAdmission) newRunIDLocked() string {
	a.runSeq++
	return fmt.Sprintf("run-%d", a.runSeq)
}

func (a *TaskAdmission) canAcquireLocked(sessionID, workerID string) bool {
	return (a.globalMax <= 0 || a.active < a.globalMax) &&
		(a.perSession <= 0 || a.sessions[sessionID] < a.perSession) &&
		(a.perWorker <= 0 || a.workers[workerID] < a.perWorker)
}

func (a *TaskAdmission) acquireLocked(sessionID, workerID, runID string) {
	a.active++
	a.sessions[sessionID]++
	a.workers[workerID]++
	if runID != "" {
		a.activeRuns[runID] = admissionRunRecord{sessionID: sessionID, workerID: workerID, queuedAt: time.Now().UTC()}
	}
}

func (a *TaskAdmission) TryAcquire(sessionID, workerID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	// New arrivals must not bypass already queued work.
	if len(a.waiters) > 0 || !a.canAcquireLocked(sessionID, workerID) {
		return false
	}
	a.acquireLocked(sessionID, workerID, a.newRunIDLocked())
	return true
}

// AcquireRun behaves like Acquire but also records the run under a caller
// observable run ID so queued work can be listed and cancelled.
func (a *TaskAdmission) AcquireRun(ctx context.Context, sessionID, workerID string) (string, bool) {
	a.mu.Lock()
	runID := a.newRunIDLocked()
	if len(a.waiters) == 0 && a.canAcquireLocked(sessionID, workerID) {
		a.acquireLocked(sessionID, workerID, runID)
		a.mu.Unlock()
		return runID, true
	}
	if a.maxQueued <= 0 || len(a.waiters) >= a.maxQueued {
		a.mu.Unlock()
		return "", false
	}
	waiter := &admissionWaiter{sessionID: sessionID, workerID: workerID, runID: runID, queuedAt: time.Now().UTC(), granted: make(chan struct{})}
	a.waiters = append(a.waiters, waiter)
	a.dispatchWaitersLocked()
	a.mu.Unlock()

	select {
	case <-waiter.granted:
		return runID, true
	case <-ctx.Done():
		a.mu.Lock()
		for i, candidate := range a.waiters {
			if candidate == waiter {
				a.waiters = append(a.waiters[:i], a.waiters[i+1:]...)
				a.dispatchWaitersLocked()
				a.mu.Unlock()
				return "", false
			}
		}
		// A grant raced with cancellation; consume it and return the capacity.
		a.releaseLocked(sessionID, workerID)
		a.dispatchWaitersLocked()
		a.mu.Unlock()
		return "", false
	}
}

// Acquire waits in a bounded FIFO queue. When the oldest waiter is blocked by
// a session/worker limit, the oldest eligible waiter may proceed so unrelated
// capacity does not sit idle.
func (a *TaskAdmission) Acquire(ctx context.Context, sessionID, workerID string) bool {
	_, ok := a.AcquireRun(ctx, sessionID, workerID)
	return ok
}

func (a *TaskAdmission) dispatchWaitersLocked() {
	for {
		granted := -1
		for i, waiter := range a.waiters {
			if a.canAcquireLocked(waiter.sessionID, waiter.workerID) {
				granted = i
				break
			}
		}
		if granted < 0 {
			return
		}
		waiter := a.waiters[granted]
		a.waiters = append(a.waiters[:granted], a.waiters[granted+1:]...)
		a.acquireLocked(waiter.sessionID, waiter.workerID, waiter.runID)
		close(waiter.granted)
	}
}

func (a *TaskAdmission) releaseLocked(sessionID, workerID string) bool {
	if a.sessions[sessionID] == 0 || a.workers[workerID] == 0 || a.active == 0 {
		return false
	}
	a.active--
	if a.sessions[sessionID]--; a.sessions[sessionID] == 0 {
		delete(a.sessions, sessionID)
	}
	if a.workers[workerID]--; a.workers[workerID] == 0 {
		delete(a.workers, workerID)
	}
	for runID, record := range a.activeRuns {
		if record.sessionID == sessionID && record.workerID == workerID {
			delete(a.activeRuns, runID)
			break
		}
	}
	return true
}

func (a *TaskAdmission) Release(sessionID, workerID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.releaseLocked(sessionID, workerID) {
		a.dispatchWaitersLocked()
	}
}

// Reconfigure updates admission limits without disturbing active reservations.
// Lower limits take effect for future grants; already-active work is allowed to
// finish. Existing queued work is retained even if the new queue limit is lower.
func (a *TaskAdmission) Reconfigure(globalMax, perSession, perWorker, maxQueued int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.globalMax, a.perSession, a.perWorker, a.maxQueued = globalMax, perSession, perWorker, maxQueued
	a.dispatchWaitersLocked()
}

type TaskAdmissionSnapshot struct {
	Active          int            `json:"active"`
	Queued          int            `json:"queued"`
	GlobalLimit     int            `json:"globalLimit"`
	PerSessionLimit int            `json:"perSessionLimit"`
	PerWorkerLimit  int            `json:"perWorkerLimit"`
	QueueLimit      int            `json:"queueLimit"`
	Sessions        map[string]int `json:"sessions"`
	Workers         map[string]int `json:"workers"`
}

func (a *TaskAdmission) Snapshot() TaskAdmissionSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	sessions := make(map[string]int, len(a.sessions))
	workers := make(map[string]int, len(a.workers))
	for id, count := range a.sessions {
		sessions[id] = count
	}
	for id, count := range a.workers {
		workers[id] = count
	}
	return TaskAdmissionSnapshot{Active: a.active, Queued: len(a.waiters), GlobalLimit: a.globalMax, PerSessionLimit: a.perSession, PerWorkerLimit: a.perWorker, QueueLimit: a.maxQueued, Sessions: sessions, Workers: workers}
}

func (a *TaskAdmission) Active() int { return a.Snapshot().Active }
func (a *TaskAdmission) Queued() int { return a.Snapshot().Queued }

// DetailedRuns lists active and queued runs. Queued runs carry a 1-based
// position in admission order; active runs report phase "active".
func (a *TaskAdmission) DetailedRuns() []AdmissionRun {
	a.mu.Lock()
	defer a.mu.Unlock()
	runs := make([]AdmissionRun, 0, len(a.activeRuns)+len(a.waiters))
	for runID, record := range a.activeRuns {
		runs = append(runs, AdmissionRun{RunID: runID, SessionID: record.sessionID, WorkerID: record.workerID, Phase: "active", QueuedAt: record.queuedAt})
	}
	for position, waiter := range a.waiters {
		runs = append(runs, AdmissionRun{RunID: waiter.runID, SessionID: waiter.sessionID, WorkerID: waiter.workerID, Phase: "queued", Position: position + 1, QueuedAt: waiter.queuedAt})
	}
	return runs
}

// CancelQueued removes a queued waiter. Active runs cannot be cancelled here;
// the caller aborts the target session instead.
func (a *TaskAdmission) CancelQueued(runID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, waiter := range a.waiters {
		if waiter.runID == runID {
			a.waiters = append(a.waiters[:i], a.waiters[i+1:]...)
			a.dispatchWaitersLocked()
			return true
		}
	}
	return false
}

// ActiveRunTarget resolves the session and worker an active run occupies.
func (a *TaskAdmission) ActiveRunTarget(runID string) (sessionID, workerID string, ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	record, found := a.activeRuns[runID]
	return record.sessionID, record.workerID, found
}
