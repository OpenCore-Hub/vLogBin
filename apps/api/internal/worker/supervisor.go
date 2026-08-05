// Package worker provides supervised background worker execution with panic
// recovery and exponential-backoff restarts.
//
// The platform runs a dozen+ background workers (outbox relay, webhook
// delivery, retention sweepers, ...). A single unguarded panic in any of them
// would silently stop that worker forever — a production incident that only
// surfaces later as stale metrics. Supervisor wraps every worker so a panic
// or an unexpected early return is logged with a stack dump, surfaced through
// Prometheus counters, and the worker is restarted with exponential backoff.
package worker

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"
)

// Options configures the supervisor's restart policy and observability hooks.
type Options struct {
	// BackoffInitial is the delay before the first restart after an
	// unexpected exit (default 1s).
	BackoffInitial time.Duration
	// BackoffMax caps the exponential backoff delay (default 30s).
	BackoffMax time.Duration
	// BackoffFactor multiplies the delay per consecutive restart
	// (default 2).
	BackoffFactor float64
	// ResetAfter is how long a worker must stay up before the backoff
	// counter resets to BackoffInitial (default 5m). Without this, one
	// transient crash would pin the delay at BackoffMax for the rest of
	// the process lifetime.
	ResetAfter time.Duration
	// OnPanic, when non-nil, is called with the worker name and the stack
	// dump every time the worker panics.
	OnPanic func(name string, stack []byte)
	// OnRestart, when non-nil, is called before every restart with the
	// worker name and the exit reason ("panic" or "exit").
	OnRestart func(name, reason string)
}

const (
	defaultBackoffInitial = time.Second
	defaultBackoffMax     = 30 * time.Second
	defaultBackoffFactor  = 2.0
	defaultResetAfter     = 5 * time.Minute
)

// Supervisor runs worker functions under a restart policy. It is safe for
// concurrent use: each Run call is independent.
type Supervisor struct {
	log  *slog.Logger
	opts Options
}

// New builds a Supervisor, filling unset backoff options with the defaults.
func New(log *slog.Logger, opts Options) *Supervisor {
	if log == nil {
		log = slog.Default()
	}
	if opts.BackoffInitial <= 0 {
		opts.BackoffInitial = defaultBackoffInitial
	}
	if opts.BackoffMax <= 0 {
		opts.BackoffMax = defaultBackoffMax
	}
	if opts.BackoffFactor <= 1 {
		opts.BackoffFactor = defaultBackoffFactor
	}
	if opts.ResetAfter <= 0 {
		opts.ResetAfter = defaultResetAfter
	}
	return &Supervisor{log: log, opts: opts}
}

// Run starts fn under supervision and returns a channel that is closed when
// the worker exits permanently. The returned channel lets callers treat
// supervised workers uniformly during shutdown (wait on done with a budget).
//
// Contract: fn must observe ctx cancellation and return nil promptly; that is
// the only graceful exit path. Any other outcome while ctx is still live —
// a panic, a non-nil error, or a nil return before cancellation — is treated
// as an unexpected exit and the worker is restarted with exponential backoff.
func (s *Supervisor) Run(ctx context.Context, name string, fn func(context.Context) error) <-chan struct{} {
	done := make(chan struct{})
	go s.supervise(ctx, name, fn, done)
	return done
}

// supervise is the per-worker supervision loop: run once, and on unexpected
// exit back off exponentially (resetting after a healthy run) and restart.
func (s *Supervisor) supervise(ctx context.Context, name string, fn func(context.Context) error, done chan<- struct{}) {
	defer close(done)
	backoff := s.opts.BackoffInitial
	for {
		start := time.Now()
		reason, panicked, err := s.runOnce(ctx, name, fn)
		if reason == "" {
			return // graceful shutdown: ctx was cancelled
		}

		// Unexpected exit: log, notify, back off and restart. A run that
		// stayed healthy for at least ResetAfter resets the delay so a
		// single transient crash does not pin the worker at BackoffMax.
		switch {
		case panicked:
			s.log.Error("worker restarted after panic", "worker", name, "backoff", backoff)
		case err != nil:
			s.log.Error("worker exited with error; restarting", "worker", name, "error", err, "backoff", backoff)
		default:
			s.log.Warn("worker exited without error; restarting", "worker", name, "backoff", backoff)
		}
		if s.opts.OnRestart != nil {
			s.opts.OnRestart(name, reason)
		}
		if time.Since(start) >= s.opts.ResetAfter {
			backoff = s.opts.BackoffInitial
		} else {
			backoff = time.Duration(float64(backoff) * s.opts.BackoffFactor)
			if backoff > s.opts.BackoffMax {
				backoff = s.opts.BackoffMax
			}
		}
		if !sleep(ctx, backoff) {
			return // ctx cancelled while backing off
		}
	}
}

// runOnce runs fn once with panic recovery.
//
// Returns:
//   - reason == "" when ctx was cancelled (graceful shutdown; do not restart);
//   - reason == "panic" when fn panicked;
//   - reason == "exit" when fn returned while ctx was still live.
func (s *Supervisor) runOnce(ctx context.Context, name string, fn func(context.Context) error) (reason string, panicked bool, err error) {
	if ctx.Err() != nil {
		return "", false, nil
	}
	defer func() {
		if r := recover(); r != nil {
			panicked = true
			stack := debug.Stack()
			s.log.Error("worker panicked",
				"worker", name,
				"panic", fmt.Sprintf("%v", r),
				"stack", string(stack),
			)
			if s.opts.OnPanic != nil {
				s.opts.OnPanic(name, stack)
			}
			reason = "panic"
		}
	}()
	err = fn(ctx)
	if ctx.Err() != nil {
		// fn returned because it observed the cancellation.
		return "", false, nil
	}
	return "exit", false, err
}

// sleep waits d or until ctx is cancelled, returning false on cancellation.
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
