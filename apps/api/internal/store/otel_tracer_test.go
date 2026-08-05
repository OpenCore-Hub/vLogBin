package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func newTestTracer(t *testing.T) (trace.Tracer, *tracetest.SpanRecorder) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	return tp.Tracer("test"), rec
}

func endedStubs(rec *tracetest.SpanRecorder) []tracetest.SpanStub {
	raw := rec.Ended()
	stubs := make([]tracetest.SpanStub, len(raw))
	for i, s := range raw {
		stubs[i] = tracetest.SpanStubFromReadOnlySpan(s)
	}
	return stubs
}

func TestOTelQueryTracerRecordsSpan(t *testing.T) {
	tr, rec := newTestTracer(t)
	q := &otelQueryTracer{tracer: tr}

	ctx := q.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
	q.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})

	spans := endedStubs(rec)
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	s := spans[0]
	if s.Name != "store.query" {
		t.Fatalf("span name = %q, want store.query", s.Name)
	}
	var stmt string
	for _, kv := range s.Attributes {
		if kv.Key == attribute.Key("db.statement") {
			stmt = kv.Value.AsString()
		}
	}
	if stmt != "SELECT 1" {
		t.Fatalf("db.statement = %q, want SELECT 1", stmt)
	}
	if s.Status.Code != codes.Unset {
		t.Fatalf("unexpected status code %v", s.Status.Code)
	}
}

func TestOTelQueryTracerRecordsError(t *testing.T) {
	tr, rec := newTestTracer(t)
	q := &otelQueryTracer{tracer: tr}

	ctx := q.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "SELECT boom"})
	q.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: errors.New("syntax error")})

	spans := endedStubs(rec)
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	if spans[0].Status.Code != codes.Error {
		t.Fatalf("status code = %v, want error", spans[0].Status.Code)
	}
	if len(spans[0].Events) == 0 {
		t.Fatal("expected recorded exception event")
	}
}

func TestOTelQueryTracerNilTracerIsNoop(t *testing.T) {
	q := &otelQueryTracer{} // nil tracer

	ctx := q.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
	if _, ok := ctx.Value(otelSpanKey{}).(trace.Span); ok {
		t.Fatal("expected no span stashed with nil tracer")
	}
	// Must not panic.
	q.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})
}

type countingTracer struct {
	starts, ends int
}

func (c *countingTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.starts++
	return ctx
}

func (c *countingTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {
	c.ends++
}

func TestMultiQueryTracerFansOut(t *testing.T) {
	a, b := &countingTracer{}, &countingTracer{}
	m := &multiQueryTracer{tracers: []pgxTracer{a, b}}

	ctx := m.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
	m.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{})

	if a.starts != 1 || b.starts != 1 {
		t.Fatalf("start fan-out wrong: a=%d b=%d", a.starts, b.starts)
	}
	if a.ends != 1 || b.ends != 1 {
		t.Fatalf("end fan-out wrong: a=%d b=%d", a.ends, b.ends)
	}
}
