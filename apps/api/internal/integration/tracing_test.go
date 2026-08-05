package integration

import (
	"net/http"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestHTTPTracingEndToEnd verifies the tracing middleware produces a span for
// a real request through the shared server, named after the matched route and
// carrying the response status.
func TestHTTPTracingEndToEnd(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	defer otel.SetTracerProvider(prev)

	resp, err := http.Get(httpServer.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	spans := rec.Ended()
	for _, s := range spans {
		if s.Name() != "/health" {
			continue
		}
		for _, kv := range s.Attributes() {
			if kv.Key == attribute.Key("http.response.status_code") && kv.Value.AsInt64() == 200 {
				return // expected span found
			}
		}
	}
	t.Fatalf("no span named /health with status 200 among %d recorded spans", len(spans))
}
