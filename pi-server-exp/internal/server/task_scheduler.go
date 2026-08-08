package server

import (
	"container/heap"
	"sync"
	"time"
)

// TaskPriority keeps interactive work ahead of background work when capacity
// is constrained. The scheduler is intentionally transport-agnostic so local
// and worker-backed runs can share the same admission policy.
type TaskPriority int

const (
	TaskPriorityBackground TaskPriority = iota
	TaskPriorityNormal
	TaskPriorityInteractive
)

type ScheduledRun struct {
	TaskID   string
	RunID    string
	Priority TaskPriority
	QueuedAt time.Time
	Canceled bool
}

type runQueue []ScheduledRun

func (q runQueue) Len() int { return len(q) }
func (q runQueue) Less(i, j int) bool {
	if q[i].Priority != q[j].Priority {
		return q[i].Priority > q[j].Priority
	}
	return q[i].QueuedAt.Before(q[j].QueuedAt)
}
func (q runQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *runQueue) Push(x any)   { *q = append(*q, x.(ScheduledRun)) }
func (q *runQueue) Pop() any     { old := *q; n := len(old); item := old[n-1]; *q = old[:n-1]; return item }

// TaskScheduler provides bounded admission, priority ordering, and queued-run
// cancellation. Execution remains owned by the session/worker layer.
type TaskScheduler struct {
	mu        sync.Mutex
	maxQueued int
	queue     runQueue
}

func NewTaskScheduler(maxQueued int) *TaskScheduler {
	if maxQueued <= 0 {
		maxQueued = 100
	}
	q := runQueue{}
	heap.Init(&q)
	return &TaskScheduler{maxQueued: maxQueued, queue: q}
}

func (s *TaskScheduler) Enqueue(run ScheduledRun) bool {
	if run.TaskID == "" || run.RunID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) >= s.maxQueued {
		return false
	}
	if run.QueuedAt.IsZero() {
		run.QueuedAt = time.Now().UTC()
	}
	heap.Push(&s.queue, run)
	return true
}

func (s *TaskScheduler) Next() (ScheduledRun, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.queue) > 0 {
		run := heap.Pop(&s.queue).(ScheduledRun)
		if !run.Canceled {
			return run, true
		}
	}
	return ScheduledRun{}, false
}

func (s *TaskScheduler) Cancel(runID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.queue {
		if s.queue[index].RunID == runID && !s.queue[index].Canceled {
			s.queue[index].Canceled = true
			return true
		}
	}
	return false
}

func (s *TaskScheduler) Queued() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.queue) }
