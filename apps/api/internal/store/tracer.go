package store

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

// SlowQueryObserver receives each query whose execution exceeded the configured
// slow-query threshold. sql is the query text (params are deliberately omitted
// — they may carry secrets), duration is the wall-clock execution time and err
// is the query error (nil on success). main wires this to a Prometheus counter
// and a Warn log, keeping the store package free of logging/metrics imports.
type SlowQueryObserver func(sql string, duration time.Duration, err error)

// queryTracer implements pgx.QueryTracer so every statement executed through
// the pool is measured. Queries that outlive the threshold are reported to the
// observer; all others are dropped with negligible overhead. The threshold and
// observer are only read on the hot path (no allocation per query when the
// query finishes fast and the observer is nil).
type queryTracer struct {
	threshold time.Duration
	observer  *atomic.Pointer[SlowQueryObserver]
}

type queryStartKey struct{}

type queryStartInfo struct {
	start time.Time
	sql   string
}

// TraceQueryStart stamps the start wall clock and the query text into the
// returned context so TraceQueryEnd can measure the statement. The tracer
// carries no per-connection state, so it is safe for concurrent use across the
// pool.
func (t *queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryStartKey{}, queryStartInfo{start: time.Now(), sql: data.SQL})
}

// TraceQueryEnd measures the completed statement and reports it to the
// observer when it outlived the threshold.
func (t *queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	info, ok := ctx.Value(queryStartKey{}).(queryStartInfo)
	if !ok {
		return
	}
	if elapsed := time.Since(info.start); elapsed >= t.threshold {
		if t.observer == nil {
			return
		}
		if p := t.observer.Load(); p != nil {
			(*p)(info.sql, elapsed, data.Err)
		}
	}
}
