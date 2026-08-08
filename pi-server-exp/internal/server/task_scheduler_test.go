package server

import "testing"

func TestTaskSchedulerRejectsInvalidAndOverCapacityRuns(t *testing.T) {
	scheduler := NewTaskScheduler(1)
	if scheduler.Enqueue(ScheduledRun{TaskID: "task"}) {
		t.Fatal("accepted missing run ID")
	}
	if !scheduler.Enqueue(ScheduledRun{TaskID: "task", RunID: "one"}) {
		t.Fatal("could not enqueue first run")
	}
	if scheduler.Enqueue(ScheduledRun{TaskID: "task", RunID: "two"}) {
		t.Fatal("accepted run beyond capacity")
	}
}

func TestTaskSchedulerPriorityAndCancellation(t *testing.T) {
	scheduler := NewTaskScheduler(3)
	if !scheduler.Enqueue(ScheduledRun{TaskID: "task", RunID: "background", Priority: TaskPriorityBackground}) ||
		!scheduler.Enqueue(ScheduledRun{TaskID: "task", RunID: "interactive", Priority: TaskPriorityInteractive}) {
		t.Fatal("could not enqueue runs")
	}
	if !scheduler.Cancel("background") {
		t.Fatal("could not cancel queued run")
	}
	run, ok := scheduler.Next()
	if !ok || run.RunID != "interactive" {
		t.Fatalf("unexpected next run: %#v", run)
	}
	if _, ok := scheduler.Next(); ok {
		t.Fatal("canceled run should not execute")
	}
}
