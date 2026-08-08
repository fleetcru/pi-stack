package server

import (
	"context"
	"testing"
	"time"
)

func TestTaskAdmissionQueuesUntilRelease(t *testing.T) {
	a := NewTaskAdmissionWithQueue(1, 1, 1, 1)
	if !a.TryAcquire("s1", "w1") {
		t.Fatal("first acquire failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan bool, 1)
	go func() { result <- a.Acquire(ctx, "s2", "w2") }()
	time.Sleep(10 * time.Millisecond)
	a.Release("s1", "w1")
	if !<-result {
		t.Fatal("queued acquire did not wake")
	}
}

func TestTaskAdmissionLimitsAndRelease(t *testing.T) {
	a := NewTaskAdmission(2, 1, 1)
	if !a.TryAcquire("s1", "w1") {
		t.Fatal("first acquire failed")
	}
	if a.TryAcquire("s1", "w2") {
		t.Fatal("per-session limit not enforced")
	}
	if a.TryAcquire("s2", "w1") {
		t.Fatal("per-worker limit not enforced")
	}
	if !a.TryAcquire("s2", "w2") {
		t.Fatal("second independent acquire failed")
	}
	if a.TryAcquire("s3", "w3") {
		t.Fatal("global limit not enforced")
	}
	a.Release("s1", "w1")
	if !a.TryAcquire("s3", "w3") {
		t.Fatal("release did not restore capacity")
	}
}
