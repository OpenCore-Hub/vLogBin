package circuitbreaker

import (
	"testing"
	"time"
)

// newTest builds a breaker with a controllable clock.
func newTest(t *testing.T, opts Options) (*Breaker, *time.Time) {
	t.Helper()
	if opts.FailureThreshold == 0 {
		opts.FailureThreshold = 3
	}
	if opts.OpenTimeout == 0 {
		opts.OpenTimeout = time.Minute
	}
	if opts.HalfOpenMax == 0 {
		opts.HalfOpenMax = 1
	}
	now := time.Now()
	b := New("test", opts)
	b.now = func() time.Time { return now }
	return b, &now
}

// advance moves the injected clock forward by d.
func advance(now *time.Time, d time.Duration) { *now = now.Add(d) }

func TestDefaultsApplied(t *testing.T) {
	b := New("d", Options{})
	if got := b.opts.FailureThreshold; got != 5 {
		t.Fatalf("FailureThreshold = %d, want 5", got)
	}
	if got := b.opts.OpenTimeout; got != 30*time.Second {
		t.Fatalf("OpenTimeout = %v, want 30s", got)
	}
	if got := b.opts.HalfOpenMax; got != 1 {
		t.Fatalf("HalfOpenMax = %d, want 1", got)
	}
	if b.State() != StateClosed {
		t.Fatalf("initial state = %v, want closed", b.State())
	}
}

func TestClosedAllowsAndSuccessResetsFailures(t *testing.T) {
	b, _ := newTest(t, Options{})
	if !b.Allow() {
		t.Fatal("closed breaker must allow calls")
	}
	// A success in the middle resets the failure streak.
	b.OnFailure()
	b.OnFailure()
	b.OnSuccess()
	if b.Failures() != 0 {
		t.Fatalf("failures = %d after success, want 0", b.Failures())
	}
	if b.State() != StateClosed {
		t.Fatalf("state = %v, want closed", b.State())
	}
}

func TestTripsOpenAfterThreshold(t *testing.T) {
	b, _ := newTest(t, Options{FailureThreshold: 3})
	for range 2 {
		b.OnFailure()
		if b.State() != StateClosed {
			t.Fatalf("state = %v after %d failures, want closed", b.State(), b.Failures())
		}
	}
	b.OnFailure()
	if b.State() != StateOpen {
		t.Fatalf("state = %v after threshold, want open", b.State())
	}
	// Open rejects everything.
	if b.Allow() {
		t.Fatal("open breaker must reject calls")
	}
}

func TestOpenRecoversToHalfOpenAfterTimeout(t *testing.T) {
	b, now := newTest(t, Options{FailureThreshold: 2, OpenTimeout: 10 * time.Second})
	b.OnFailure()
	b.OnFailure()
	if b.State() != StateOpen {
		t.Fatalf("state = %v, want open", b.State())
	}
	// Still within the cool-down: rejected.
	if b.Allow() {
		t.Fatal("open breaker must reject before cool-down elapses")
	}
	advance(now, 10*time.Second)
	// Cool-down elapsed: first Allow moves to half_open and probes.
	if !b.Allow() {
		t.Fatal("half_open probe must be allowed after cool-down")
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("state = %v, want half_open", b.State())
	}
	// No second slot with HalfOpenMax=1.
	if b.Allow() {
		t.Fatal("half_open must cap concurrent probes")
	}
}

func TestHalfOpenSuccessCloses(t *testing.T) {
	b, now := newTest(t, Options{FailureThreshold: 2, OpenTimeout: time.Second})
	b.OnFailure()
	b.OnFailure()
	advance(now, time.Second)
	if !b.Allow() {
		t.Fatal("probe must be allowed")
	}
	b.OnSuccess()
	if b.State() != StateClosed {
		t.Fatalf("state = %v after probe success, want closed", b.State())
	}
	if b.Failures() != 0 {
		t.Fatalf("failures = %d after close, want 0", b.Failures())
	}
	if !b.Allow() {
		t.Fatal("closed breaker must allow calls")
	}
}

func TestHalfOpenFailureReopensAndRestartsCooldown(t *testing.T) {
	b, now := newTest(t, Options{FailureThreshold: 2, OpenTimeout: 10 * time.Second})
	b.OnFailure()
	b.OnFailure()
	advance(now, 10*time.Second)
	if !b.Allow() {
		t.Fatal("probe must be allowed")
	}
	b.OnFailure()
	if b.State() != StateOpen {
		t.Fatalf("state = %v after probe failure, want open", b.State())
	}
	// Cool-down restarted: still rejected right after the re-open.
	if b.Allow() {
		t.Fatal("breaker must reject immediately after re-opening")
	}
	advance(now, 10*time.Second)
	if !b.Allow() {
		t.Fatal("probe must be allowed after second cool-down")
	}
}

func TestHalfOpenConcurrency(t *testing.T) {
	b, now := newTest(t, Options{FailureThreshold: 2, OpenTimeout: time.Second, HalfOpenMax: 3})
	b.OnFailure()
	b.OnFailure()
	advance(now, time.Second)
	var granted int
	for range 4 {
		if b.Allow() {
			granted++
		}
	}
	if granted != 3 {
		t.Fatalf("half_open allowed %d probes, want 3 (HalfOpenMax)", granted)
	}
	if b.State() != StateHalfOpen {
		t.Fatalf("state = %v, want half_open", b.State())
	}
	// A successful probe closes the breaker, releasing all slots.
	b.OnSuccess()
	if b.State() != StateClosed {
		t.Fatalf("state = %v after probe success, want closed", b.State())
	}
	if !b.Allow() {
		t.Fatal("closed breaker must allow calls")
	}
}

func TestStateChangeCallback(t *testing.T) {
	var transitions []State
	on := func(name string, from, to State) {
		if name != "cb" {
			t.Fatalf("callback name = %q, want cb", name)
		}
		transitions = append(transitions, to)
	}
	b := NewWithLog("cb", Options{FailureThreshold: 2}, nil, on)
	// Initial closed callback.
	if len(transitions) != 1 || transitions[0] != StateClosed {
		t.Fatalf("initial transitions = %v, want [closed]", transitions)
	}
	b.OnFailure()
	b.OnFailure() // → open
	b.State()
	if len(transitions) != 2 || transitions[1] != StateOpen {
		t.Fatalf("transitions = %v, want [closed open]", transitions)
	}
}

func TestFailureThresholdDefaultAndHalfOpenMaxDefault(t *testing.T) {
	b := New("d", Options{})
	b.now = func() time.Time { return time.Now() }
	for range 5 {
		b.OnFailure()
	}
	if b.State() != StateOpen {
		t.Fatalf("state = %v after 5 failures, want open", b.State())
	}
}
