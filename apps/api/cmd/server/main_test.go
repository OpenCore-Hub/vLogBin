package main

import (
	"testing"
	"time"
)

// immediateWorker is already done: stop is a no-op and done is closed.
func immediateWorker(name string) bgWorker {
	done := make(chan struct{})
	close(done)
	return bgWorker{name: name, stop: func() {}, done: done}
}

// stuckWorker never signals completion within any realistic window.
func stuckWorker(name string) bgWorker {
	done := make(chan struct{})
	return bgWorker{name: name, stop: func() {}, done: done}
}

func TestStopWorkersAllExit(t *testing.T) {
	workers := []bgWorker{immediateWorker("a"), immediateWorker("b")}
	if late := stopWorkers(time.Second, workers); len(late) != 0 {
		t.Fatalf("late = %v, want none", late)
	}
}

func TestStopWorkersReportsLate(t *testing.T) {
	workers := []bgWorker{immediateWorker("fast"), stuckWorker("slow")}
	start := time.Now()
	late := stopWorkers(50*time.Millisecond, workers)
	if len(late) != 1 || late[0] != "slow" {
		t.Fatalf("late = %v, want [slow]", late)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("stopWorkers took %v, want ~50ms (must stop in parallel)", elapsed)
	}
}

func TestStopWorkersEmpty(t *testing.T) {
	if late := stopWorkers(time.Second, nil); len(late) != 0 {
		t.Fatalf("late = %v, want none", late)
	}
}
