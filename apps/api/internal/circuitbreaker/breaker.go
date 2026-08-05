// Package circuitbreaker implements a concurrency-safe three-state circuit
// breaker protecting outbound dependencies (webhook endpoints, the billing
// engine) from cascading failures:
//
//	closed → open (after N consecutive failures) → half_open (after a
//	cool-down, allowing a probe) → closed (probe succeeds) or open again.
//
// While open, calls are fast-failed instead of hitting a dependency that is
// already known to be failing, so workers stop hammering a dead endpoint and
// latency-sensitive callers get an immediate decision.
//
// The state machine is lock-free: transitions use atomic CAS, and recovery
// from open is lazy (evaluated on the next Allow call), so no background
// timer goroutine is needed.
package circuitbreaker

import (
	"log/slog"
	"sync/atomic"
	"time"
)

// State is the breaker's current state.
type State int32

const (
	// StateClosed lets every call through and counts consecutive failures.
	StateClosed State = iota
	// StateHalfOpen lets a bounded number of probe calls through to test
	// whether the dependency has recovered.
	StateHalfOpen
	// StateOpen fast-fails every call until the cool-down elapses.
	StateOpen
)

// String returns a Prometheus-friendly lowercase state name.
func (s State) String() string {
	switch s {
	case StateHalfOpen:
		return "half_open"
	case StateOpen:
		return "open"
	default:
		return "closed"
	}
}

// Options configures a Breaker. Zero-valued fields fall back to defaults.
type Options struct {
	// FailureThreshold is the number of consecutive failures that trips the
	// breaker from closed to open (default 5).
	FailureThreshold int
	// OpenTimeout is how long the breaker stays open before the next Allow
	// call may probe the dependency again (default 30s).
	OpenTimeout time.Duration
	// HalfOpenMax bounds the number of concurrent probe calls in the
	// half-open state (default 1).
	HalfOpenMax int
}

// OnTransition is invoked on every state change (including the initial
// closed state, so metric gauges start at zero). It must be safe for
// concurrent use.
type OnTransition func(name string, from, to State)

// Breaker is a named, concurrency-safe circuit breaker.
type Breaker struct {
	name         string
	opts         Options
	log          *slog.Logger
	onTransition OnTransition

	state        atomic.Int32
	failures     atomic.Int64 // consecutive failures while closed
	openedAt     atomic.Int64 // unix nanos of the last open transition
	halfInFlight atomic.Int64 // in-flight half-open probes
	now          func() time.Time
}

// New builds a breaker with the given name. opts' zero fields use defaults.
func New(name string, opts Options) *Breaker {
	return NewWithLog(name, opts, nil, nil)
}

// NewWithLog builds a breaker that logs state changes to log and reports
// transitions to on (used for metric updates). Both are optional.
func NewWithLog(name string, opts Options, log *slog.Logger, on OnTransition) *Breaker {
	if opts.FailureThreshold <= 0 {
		opts.FailureThreshold = 5
	}
	if opts.OpenTimeout <= 0 {
		opts.OpenTimeout = 30 * time.Second
	}
	if opts.HalfOpenMax <= 0 {
		opts.HalfOpenMax = 1
	}
	b := &Breaker{
		name:         name,
		opts:         opts,
		log:          log,
		onTransition: on,
		now:          time.Now,
	}
	// Report the initial state so metric gauges appear (as closed/0) from the
	// moment the breaker exists.
	if on != nil {
		on(name, StateClosed, StateClosed)
	}
	return b
}

// Name returns the breaker's identifier.
func (b *Breaker) Name() string { return b.name }

// State returns the current state.
func (b *Breaker) State() State { return State(b.state.Load()) }

// Failures returns the current consecutive-failure count (meaningful while
// closed).
func (b *Breaker) Failures() int64 { return b.failures.Load() }

// Allow reports whether the caller may invoke the protected dependency.
//
//   - closed: always allowed.
//   - open: denied until the cool-down elapses; once it has, the breaker
//     lazily moves to half_open and the call is allowed as a probe.
//   - half_open: allowed only while the number of in-flight probes is below
//     HalfOpenMax; otherwise denied.
func (b *Breaker) Allow() bool {
	for {
		switch b.State() {
		case StateClosed:
			return true
		case StateOpen:
			if !b.expireOpen() {
				return false
			}
			// Cool-down elapsed: now half_open; fall through to probe.
		case StateHalfOpen:
			if b.tryAcquireHalfOpen() {
				return true
			}
			return false
		}
	}
}

// expireOpen moves the breaker to half_open once the cool-down has elapsed.
// It returns true when the breaker is (or just became) half_open, so the
// caller can attempt a probe.
func (b *Breaker) expireOpen() bool {
	if b.now().Sub(time.Unix(0, b.openedAt.Load())) < b.opts.OpenTimeout {
		return false
	}
	if b.state.CompareAndSwap(int32(StateOpen), int32(StateHalfOpen)) {
		b.transition(StateOpen, StateHalfOpen)
	}
	return true
}

// tryAcquireHalfOpen reserves one half-open probe slot, returning false when
// the in-flight count already equals HalfOpenMax.
func (b *Breaker) tryAcquireHalfOpen() bool {
	for {
		n := b.halfInFlight.Load()
		if n >= int64(b.opts.HalfOpenMax) {
			return false
		}
		if b.halfInFlight.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

// OnSuccess records a successful call. While closed it resets the failure
// counter; while half_open it closes the breaker again (releasing its probe
// slot).
func (b *Breaker) OnSuccess() {
	for {
		switch b.State() {
		case StateClosed:
			b.failures.Store(0)
			return
		case StateHalfOpen:
			if b.state.CompareAndSwap(int32(StateHalfOpen), int32(StateClosed)) {
				b.halfInFlight.Add(-1)
				b.failures.Store(0)
				b.transition(StateHalfOpen, StateClosed)
				return
			}
			// Lost the race to another goroutine; re-evaluate.
		default: // StateOpen: callers must not run calls while open.
			return
		}
	}
}

// OnFailure records a failed call. While closed it increments the failure
// counter and trips the breaker to open once the threshold is reached;
// while half_open it re-opens the breaker and restarts the cool-down.
func (b *Breaker) OnFailure() {
	for {
		switch b.State() {
		case StateClosed:
			if b.failures.Add(1) < int64(b.opts.FailureThreshold) {
				return
			}
			if b.state.CompareAndSwap(int32(StateClosed), int32(StateOpen)) {
				b.openedAt.Store(b.now().UnixNano())
				b.transition(StateClosed, StateOpen)
			}
			return
		case StateHalfOpen:
			b.halfInFlight.Add(-1)
			if b.state.CompareAndSwap(int32(StateHalfOpen), int32(StateOpen)) {
				b.openedAt.Store(b.now().UnixNano())
				b.transition(StateHalfOpen, StateOpen)
			}
			return
		default: // StateOpen: nothing to do.
			return
		}
	}
}

func (b *Breaker) transition(from, to State) {
	if b.onTransition != nil {
		b.onTransition(b.name, from, to)
	}
	if b.log != nil {
		b.log.Warn("circuit breaker state changed",
			"breaker", b.name,
			"from", from.String(),
			"to", to.String(),
		)
	}
}
