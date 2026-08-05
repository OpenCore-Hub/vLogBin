package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// WithQueryTracerProvider instruments every pool query with an OpenTelemetry
// span. A nil provider is a no-op so the pool keeps zero per-query tracing
// overhead; main wires this only when distributed tracing is enabled.
func WithQueryTracerProvider(tp trace.TracerProvider) Option {
	return func(cfg *Config) { cfg.QueryTracerProvider = tp }
}

// pgxTracer is the subset of the pgx query-tracer contract used by both the
// slow-query tracer and the OpenTelemetry tracer. Declared locally so the
// store package does not depend on whether pgx exports its own interface.
type pgxTracer interface {
	TraceQueryStart(context.Context, *pgx.Conn, pgx.TraceQueryStartData) context.Context
	TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData)
}

// otelQueryTracer implements the pgx query-tracer contract, creating one span
// per statement. The SQL text is recorded without parameters (they may carry
// secrets), matching the slow-query tracer's policy. It carries no
// per-connection state, so it is safe for concurrent use across the pool.
type otelQueryTracer struct {
	tracer trace.Tracer
}

type otelSpanKey struct{}

// TraceQueryStart opens the span and stashes it in the returned context so
// TraceQueryEnd can finish it.
func (t *otelQueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if t.tracer == nil {
		return ctx
	}
	_, span := t.tracer.Start(ctx, "store.query",
		trace.WithAttributes(attribute.String("db.statement", data.SQL)))
	return context.WithValue(ctx, otelSpanKey{}, span)
}

// TraceQueryEnd finishes the span, recording the error (if any) and setting
// an error status so failed statements stand out in trace views.
func (t *otelQueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span, ok := ctx.Value(otelSpanKey{}).(trace.Span)
	if !ok {
		return
	}
	defer span.End()
	if data.Err != nil {
		span.RecordError(data.Err)
		span.SetStatus(codes.Error, data.Err.Error())
	}
}

// multiQueryTracer fans out pgx tracer events to several tracers so the
// slow-query observer and OpenTelemetry instrumentation can coexist on one
// pool. TraceQueryEnd runs tracers in order; each tracer only reads its own
// context value, so ordering between them is irrelevant.
type multiQueryTracer struct {
	tracers []pgxTracer
}

func (m *multiQueryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	for _, t := range m.tracers {
		ctx = t.TraceQueryStart(ctx, conn, data)
	}
	return ctx
}

func (m *multiQueryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	for _, t := range m.tracers {
		t.TraceQueryEnd(ctx, conn, data)
	}
}
