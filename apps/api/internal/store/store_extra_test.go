package store

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWithQueryTimeoutDisabled(t *testing.T) {
	s := &Store{} // QueryTimeout unset
	ctx, cancel := s.WithQueryTimeout(context.Background())
	defer cancel()
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		t.Fatal("no deadline expected when QueryTimeout is unset")
	}
}

func TestWithQueryTimeoutExpires(t *testing.T) {
	s := &Store{queryTimeout: 10 * time.Millisecond}
	ctx, cancel := s.WithQueryTimeout(context.Background())
	defer cancel()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("context did not expire")
	}
	if err := ctx.Err(); err != context.DeadlineExceeded {
		t.Fatalf("ctx.Err() = %v, want DeadlineExceeded", err)
	}
}

func TestApplyPoolConfig(t *testing.T) {
	poolCfg, err := pgxpool.ParseConfig("postgres://u:p@localhost:5432/db")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	applyPoolConfig(poolCfg, Config{
		MaxConns:          7,
		MinConns:          2,
		MaxConnLifetime:   time.Minute,
		MaxConnIdleTime:   30 * time.Second,
		HealthCheckPeriod: 10 * time.Second,
	})
	if poolCfg.MaxConns != 7 || poolCfg.MinConns != 2 {
		t.Fatalf("pool sizes = %d/%d, want 7/2", poolCfg.MaxConns, poolCfg.MinConns)
	}
	if poolCfg.MaxConnLifetime != time.Minute || poolCfg.MaxConnIdleTime != 30*time.Second || poolCfg.HealthCheckPeriod != 10*time.Second {
		t.Fatalf("pool durations = %v %v %v, want 1m/30s/10s",
			poolCfg.MaxConnLifetime, poolCfg.MaxConnIdleTime, poolCfg.HealthCheckPeriod)
	}

	// A zero Config must not clobber the parsed (pgx) defaults.
	poolCfg2, err := pgxpool.ParseConfig("postgres://u:p@localhost:5432/db")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	wantMax := poolCfg2.MaxConns
	applyPoolConfig(poolCfg2, Config{})
	if poolCfg2.MaxConns != wantMax {
		t.Fatalf("zero Config changed MaxConns from %d to %d", wantMax, poolCfg2.MaxConns)
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ErrorClass
	}{
		{"nil", nil, ErrorClassOther},
		{"deadline exceeded", context.DeadlineExceeded, ErrorClassTimeout},
		{"context canceled", context.Canceled, ErrorClassTimeout},
		{"no rows", pgx.ErrNoRows, ErrorClassNotFound},
		{"unique violation", &pgconn.PgError{Code: "23505"}, ErrorClassConflict},
		{"fk violation", &pgconn.PgError{Code: "23503"}, ErrorClassConflict},
		{"not-null violation", &pgconn.PgError{Code: "23502"}, ErrorClassConflict},
		{"other sqlstate", &pgconn.PgError{Code: "57014"}, ErrorClassOther},
		{"safe to retry", safeRetryErr{}, ErrorClassConnection},
		{"net op error", &net.OpError{Op: "dial", Err: errors.New("refused")}, ErrorClassConnection},
		{"wrapped no rows", fmt.Errorf("lookup: %w", pgx.ErrNoRows), ErrorClassNotFound},
		{"generic", errors.New("boom"), ErrorClassOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyError(tt.err); got != tt.want {
				t.Fatalf("ClassifyError(%v) = %s, want %s", tt.err, got, tt.want)
			}
		})
	}
}

// safeRetryErr mimics pgconn errors that are safe to retry (connection class).
type safeRetryErr struct{}

func (safeRetryErr) Error() string     { return "connection reset" }
func (safeRetryErr) SafeToRetry() bool { return true }

func TestErrorObserver(t *testing.T) {
	s := &Store{}
	var got []string
	s.SetErrorObserver(func(op string, class ErrorClass) {
		got = append(got, op+":"+string(class))
	})

	s.observeError("tenant", context.DeadlineExceeded)
	s.observeError("tenant.commit", &pgconn.PgError{Code: "23505"})
	s.observeError("operator", nil) // nil must be ignored
	s.observeError("tenant", errors.New("boom"))

	want := []string{"tenant:timeout", "tenant.commit:conflict", "tenant:other"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestErrorObserverUnsetIsNoop(t *testing.T) {
	s := &Store{}
	s.observeError("tenant", errors.New("boom")) // must not panic
}

func TestSetErrorObserverNilIgnored(t *testing.T) {
	s := &Store{}
	s.SetErrorObserver(nil) // must not panic
	s.observeError("tenant", errors.New("boom"))
}

func TestQueryTracerReportsSlowQuery(t *testing.T) {
	var got []string
	tracer := &queryTracer{threshold: time.Millisecond}
	tracer.observer = new(atomic.Pointer[SlowQueryObserver])
	obs := SlowQueryObserver(func(sql string, d time.Duration, err error) {
		got = append(got, fmt.Sprintf("%s:%d:%v", sql, d.Milliseconds(), err))
	})
	tracer.observer.Store(&obs)

	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
	time.Sleep(2 * time.Millisecond)
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})

	if len(got) != 1 {
		t.Fatalf("slow query not reported: got %v", got)
	}
	if got[0] != "SELECT 1:2:<nil>" && got[0] != "SELECT 1:3:<nil>" {
		t.Fatalf("unexpected report %q", got[0])
	}
}

func TestQueryTracerDropsFastQuery(t *testing.T) {
	var got []string
	tracer := &queryTracer{threshold: time.Hour}
	tracer.observer = new(atomic.Pointer[SlowQueryObserver])
	obs := SlowQueryObserver(func(sql string, d time.Duration, err error) {
		got = append(got, sql)
	})
	tracer.observer.Store(&obs)

	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})

	if len(got) != 0 {
		t.Fatalf("fast query must not be reported: got %v", got)
	}
}

func TestQueryTracerNoObserverIsNoop(t *testing.T) {
	tracer := &queryTracer{threshold: time.Nanosecond}
	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{}) // must not panic
}

func TestQueryTracerMissingStartIsNoop(t *testing.T) {
	tracer := &queryTracer{threshold: time.Nanosecond}
	tracer.TraceQueryEnd(context.Background(), nil, pgx.TraceQueryEndData{}) // no matching start
}

func TestSetSlowQueryObserver(t *testing.T) {
	s := &Store{}
	var got string
	s.SetSlowQueryObserver(func(sql string, d time.Duration, err error) {
		got = sql
	})
	if p := s.slowObserver.Load(); p == nil {
		t.Fatal("observer not installed")
	} else {
		(*p)("SELECT 1", time.Millisecond, nil)
	}
	if got != "SELECT 1" {
		t.Fatalf("observer not invoked: got %q", got)
	}

	s.SetSlowQueryObserver(nil) // must not panic; nil is ignored (keeps prior)
	if p := s.slowObserver.Load(); p == nil {
		t.Fatal("nil observer must be ignored, not clear the previous one")
	}
}
