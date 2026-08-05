package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// withRecordingTracer installs a recording tracer provider globally (as
// production wiring does) and returns a restore func. Package tests are
// serial, so the global swap is safe here.
func withRecordingTracer(t *testing.T) (*tracetest.SpanRecorder, func()) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	return rec, func() { otel.SetTracerProvider(prev) }
}

func attr(spans []sdktrace.ReadOnlySpan, key string) (string, bool) {
	for _, s := range spans {
		for _, kv := range s.Attributes() {
			if kv.Key == attribute.Key(key) {
				return kv.Value.String(), true
			}
		}
	}
	return "", false
}

func TestTracingMiddlewareRecordsRouteAndStatus(t *testing.T) {
	rec, restore := withRecordingTracer(t)
	defer restore()

	r := chi.NewRouter()
	r.Use((&Server{}).tracingMiddleware)
	r.Get("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	// The span is renamed to the matched chi route after routing completes.
	if spans[0].Name() != "/v1/health" {
		t.Fatalf("span name = %q, want /v1/health", spans[0].Name())
	}
	if v, _ := attr(spans, "http.response.status_code"); v != "200" {
		t.Fatalf("http.response.status_code = %q, want 200", v)
	}
	if v, _ := attr(spans, "http.request.method"); v != "GET" {
		t.Fatalf("http.request.method = %q, want GET", v)
	}
	if v, _ := attr(spans, "url.path"); v != "/v1/health" {
		t.Fatalf("url.path = %q, want /v1/health", v)
	}
}

func TestTracingMiddlewareMarks5xxAsError(t *testing.T) {
	rec, restore := withRecordingTracer(t)
	defer restore()

	r := chi.NewRouter()
	r.Use((&Server{}).tracingMiddleware)
	r.Get("/boom", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	if spans[0].Status().Code != codes.Error {
		t.Fatalf("status code = %v, want error", spans[0].Status().Code)
	}
	if v, _ := attr(spans, "http.response.status_code"); v != "500" {
		t.Fatalf("http.response.status_code = %q, want 500", v)
	}
}

func TestTracingMiddlewareFallbackToRawPath(t *testing.T) {
	rec, restore := withRecordingTracer(t)
	defer restore()

	r := chi.NewRouter()
	r.Use((&Server{}).tracingMiddleware)
	// Registering at least one route makes chi build the middleware chain;
	// the request below matches none, so the pattern is empty and the raw
	// path is kept.
	r.Get("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/nope/404", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	if spans[0].Name() != "/nope/404" {
		t.Fatalf("span name = %q, want /nope/404", spans[0].Name())
	}
	if v, _ := attr(spans, "http.response.status_code"); v != "404" {
		t.Fatalf("http.response.status_code = %q, want 404", v)
	}
}

func TestHostOnly(t *testing.T) {
	cases := []struct{ in, want string }{
		{"127.0.0.1:8080", "127.0.0.1"},
		{"[::1]:443", "::1"},
		{"no-port", "no-port"},
		{"", ""},
	}
	for _, c := range cases {
		if got := hostOnly(c.in); got != c.want {
			t.Errorf("hostOnly(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
