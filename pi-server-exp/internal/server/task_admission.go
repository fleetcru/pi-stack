package server

import (
	"context"
	"sync"
)

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
}

type admissionWaiter struct {
	sessionID string
	workerID  string
	granted   chan struct{}
}

func NewTaskAdmission(globalMax, perSession, perWorker int) *TaskAdmission {
	return NewTaskAdmissionWithQueue(globalMax, perSession, perWorker, 32)
}

func NewTaskAdmissionWithQueue(globalMax, perSession, perWorker, maxQueued int) *TaskAdmission {
	return &TaskAdmission{globalMax: globalMax, perSession: perSession, perWorker: perWorker, maxQueued: maxQueued, sessions: map[string]int{}, workers: map[string]int{}}
}

func (a *TaskAdmission) canAcquireLocked(sessionID, workerID string) bool {
	return (a.globalMax <= 0 || a.active < a.globalMax) &&
		(a.perSession <= 0 || a.sessions[sessionID] < a.perSession) &&
		(a.perWorker <= 0 || a.workers[workerID] < a.perWorker)
}

func (a *TaskAdmission) acquireLocked(sessionID, workerID string) {
	a.active++
	a.sessions[sessionID]++
	a.workers[workerID]++
}

func (a *TaskAdmission) TryAcquire(sessionID, workerID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	// New arrivals must not bypass already queued work.
	if len(a.waiters) > 0 || !a.canAcquireLocked(sessionID, workerID) {
		return false
	}
	a.acquireLocked(sessionID, workerID)
	return true
}

// Acquire waits in a bounded FIFO queue. When the oldest waiter is blocked by
// a session/worker limit, the oldest eligible waiter may proceed so unrelated
// capacity does not sit idle.
func (a *TaskAdmission) Acquire(ctx context.Context, sessionID, workerID string) bool {
	a.mu.Lock()
	if len(a.waiters) == 0 && a.canAcquireLocked(sessionID, workerID) {
		a.acquireLocked(sessionID, workerID)
		a.mu.Unlock()
		return true
	}
	if a.maxQueued <= 0 || len(a.waiters) >= a.maxQueued {
		a.mu.Unlock()
		return false
	}
	waiter := &admissionWaiter{sessionID: sessionID, workerID: workerID, granted: make(chan struct{})}
	a.waiters = append(a.waiters, waiter)
	a.dispatchWaitersLocked()
	a.mu.Unlock()

	select {
	case <-waiter.granted:
		return true
	case <-ctx.Done():
		a.mu.Lock()
		for i, candidate := range a.waiters {
			if candidate == waiter {
				a.waiters = append(a.waiters[:i], a.waiters[i+1:]...)
				a.dispatchWaitersLocked()
				a.mu.Unlock()
				return false
			}
		}
		// A grant raced with cancellation; consume it and return the capacity.
		a.releaseLocked(sessionID, workerID)
		a.dispatchWaitersLocked()
		a.mu.Unlock()
		return false
	}
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
		a.acquireLocked(waiter.sessionID, waiter.workerID)
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
