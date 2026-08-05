package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// waitDone waits for done to close with a generous timeout so flaky CI
// backoff timing never hangs the suite.
func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("supervised worker did not stop")
	}
}

func TestGracefulShutdownNoRestart(t *testing.T) {
	var calls atomic.Int32
	var restarts atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	done := New(nil, Options{
		OnRestart: func(name, reason string) { restarts.Add(1) },
	}).Run(ctx, "blocking", func(ctx context.Context) error {
		calls.Add(1)
		<-ctx.Done()
		return nil
	})

	// Wait for the worker goroutine to start blocking before cancelling:
	// cancelling too early can preempt the supervisor before its first run.
	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	waitDone(t, done)
	if got := calls.Load(); got != 1 {
		t.Fatalf("fn calls = %d, want 1", got)
	}
	if got := restarts.Load(); got != 0 {
		t.Fatalf("restarts = %d, want 0", got)
	}
}

func TestPanicRecoveredAndRestarted(t *testing.T) {
	var calls atomic.Int32
	var panics atomic.Int32
	var restarts atomic.Int32
	var lastReason atomic.Value

	ctx, cancel := context.WithCancel(context.Background())
	done := New(nil, Options{
		BackoffInitial: time.Millisecond,
		BackoffMax:     5 * time.Millisecond,
		OnPanic: func(name string, stack []byte) {
			panics.Add(1)
			if len(stack) == 0 {
				t.Error("panic stack dump is empty")
			}
		},
		OnRestart: func(name, reason string) {
			restarts.Add(1)
			lastReason.Store(reason)
		},
	}).Run(ctx, "panic-worker", func(ctx context.Context) error {
		if calls.Add(1) == 1 {
			panic("boom")
		}
		<-ctx.Done()
		return nil
	})

	// Let the restart happen AND the second (healthy) invocation start,
	// then shut down cleanly. Waiting only on restarts races the restart
	// backoff sleep against cancel and can yield calls == 1.
	deadline := time.Now().Add(2 * time.Second)
	for (restarts.Load() == 0 || calls.Load() < 2) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	waitDone(t, done)

	if got := calls.Load(); got != 2 {
		t.Fatalf("fn calls = %d, want 2 (1 panic + 1 healthy)", got)
	}
	if got := panics.Load(); got != 1 {
		t.Fatalf("OnPanic calls = %d, want 1", got)
	}
	if got := restarts.Load(); got != 1 {
		t.Fatalf("OnRestart calls = %d, want 1", got)
	}
	if got := lastReason.Load().(string); got != "panic" {
		t.Fatalf("restart reason = %q, want panic", got)
	}
}

func TestExitWithErrorRestarted(t *testing.T) {
	var calls atomic.Int32
	var restarts atomic.Int32
	var lastReason atomic.Value

	ctx, cancel := context.WithCancel(context.Background())
	done := New(nil, Options{
		BackoffInitial: time.Millisecond,
		BackoffMax:     5 * time.Millisecond,
		OnRestart: func(name, reason string) {
			restarts.Add(1)
			lastReason.Store(reason)
		},
	}).Run(ctx, "error-worker", func(ctx context.Context) error {
		if calls.Add(1) == 1 {
			return errors.New("transient failure")
		}
		<-ctx.Done()
		return nil
	})

	deadline := time.Now().Add(2 * time.Second)
	for restarts.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	waitDone(t, done)

	if got := restarts.Load(); got != 1 {
		t.Fatalf("OnRestart calls = %d, want 1", got)
	}
	if got := lastReason.Load().(string); got != "exit" {
		t.Fatalf("restart reason = %q, want exit", got)
	}
}

func TestNilExitBeforeCancellationRestarted(t *testing.T) {
	var calls atomic.Int32
	var restarts atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	done := New(nil, Options{
		BackoffInitial: time.Millisecond,
		BackoffMax:     5 * time.Millisecond,
		OnRestart:      func(name, reason string) { restarts.Add(1) },
	}).Run(ctx, "nil-worker", func(ctx context.Context) error {
		if calls.Add(1) == 1 {
			return nil // returned while ctx is live: unexpected exit
		}
		<-ctx.Done()
		return nil
	})

	deadline := time.Now().Add(2 * time.Second)
	for (restarts.Load() == 0 || calls.Load() < 2) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	waitDone(t, done)

	if got := restarts.Load(); got != 1 {
		t.Fatalf("OnRestart calls = %d, want 1", got)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("fn calls = %d, want 2", got)
	}
}

func TestBackoffCapsAtMax(t *testing.T) {
	var restarts atomic.Int32
	restartCh := make(chan struct{}, 100)

	ctx, cancel := context.WithCancel(context.Background())
	done := New(nil, Options{
		BackoffInitial: time.Millisecond,
		BackoffMax:     2 * time.Millisecond,
		OnRestart: func(name, reason string) {
			restarts.Add(1)
			restartCh <- struct{}{}
		},
	}).Run(ctx, "crash-loop", func(ctx context.Context) error {
		panic("always panics")
	})

	// A permanently panicking worker must keep being restarted, not die.
	for i := 0; i < 5; i++ {
		select {
		case <-restartCh:
		case <-time.After(3 * time.Second):
			t.Fatalf("expected restart #%d but worker stopped", i+1)
		}
	}
	cancel()
	waitDone(t, done)
	if got := restarts.Load(); got < 5 {
		t.Fatalf("restarts = %d, want >= 5", got)
	}
}

func TestBackoffResetAfterHealthyRun(t *testing.T) {
	// call 1 panics immediately, call 2 stays healthy past ResetAfter then
	// panics: the backoff must reset, so restart #3 happens ~BackoffInitial
	// later instead of the multiplied delay.
	var calls atomic.Int32
	restartTimes := make(chan time.Time, 8)

	ctx, cancel := context.WithCancel(context.Background())
	done := New(nil, Options{
		BackoffInitial: 5 * time.Millisecond,
		BackoffFactor:  20, // multiply to 100ms so the reset is unambiguous
		BackoffMax:     500 * time.Millisecond,
		ResetAfter:     30 * time.Millisecond,
		OnRestart: func(name, reason string) {
			restartTimes <- time.Now()
		},
	}).Run(ctx, "reset-worker", func(ctx context.Context) error {
		switch calls.Add(1) {
		case 1:
			panic("first crash")
		case 2:
			time.Sleep(60 * time.Millisecond) // healthy > ResetAfter
			panic("second crash")
		case 3:
			panic("third crash") // restart must land ~5ms after r2
		default:
			<-ctx.Done()
			return nil
		}
	})

	// r1: after first panic; r2: after second panic; r3: after third panic
	// (the reset must make r3 land ~5ms after r2, not ~100ms).
	var times []time.Time
	for len(times) < 3 {
		select {
		case ts := <-restartTimes:
			times = append(times, ts)
		case <-time.After(3 * time.Second):
			t.Fatalf("only got %d restarts, want 3", len(times))
		}
	}
	cancel()
	waitDone(t, done)

	if d := times[2].Sub(times[1]); d > 60*time.Millisecond {
		t.Fatalf("restart #3 delay = %v, want ~5ms (backoff did not reset after healthy run)", d)
	}
}
