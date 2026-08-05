package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/metrics"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func newMetricsServer() *Server {
	return &Server{metrics: metrics.New()}
}

func scrapeMetrics(t *testing.T, s *Server) string {
	t.Helper()
	w := httptest.NewRecorder()
	s.metrics.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("scrape status = %d, want 200", w.Code)
	}
	return w.Body.String()
}

func TestMetricsMiddlewareRecordsRequests(t *testing.T) {
	s := newMetricsServer()
	r := chi.NewRouter()
	r.Use(s.metricsMiddleware)
	r.Get("/v1/providers/{providerID}/credentials", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	pathUUID := uuid.NewString()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/providers/"+pathUUID+"/credentials", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := scrapeMetrics(t, s)
	// The route label must be the chi pattern, never the UUID-bearing path,
	// otherwise label cardinality explodes per provider.
	want := `http_requests_total{method="GET",route="/v1/providers/{providerID}/credentials",status="200"} 1`
	if !strings.Contains(body, want) {
		t.Fatalf("metrics output missing recorded request; want %q in:\n%s", want, body)
	}
	if strings.Contains(body, pathUUID) {
		t.Fatalf("metrics output must not contain raw UUID paths:\n%s", body)
	}
}

func TestMetricsMiddlewareFallsBackToPathWithoutRouter(t *testing.T) {
	// Direct use of the middleware (no chi RouteContext) must not panic and
	// must fall back to the raw request path.
	s := newMetricsServer()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	w := httptest.NewRecorder()
	s.metricsMiddleware(next).ServeHTTP(w, httptest.NewRequest("GET", "/v1/some/path", nil))
	if w.Code != http.StatusTeapot {
		t.Fatalf("status = %d, want 418", w.Code)
	}

	body := scrapeMetrics(t, s)
	want := `http_requests_total{method="GET",route="/v1/some/path",status="418"} 1`
	if !strings.Contains(body, want) {
		t.Fatalf("metrics output missing raw-path fallback; want %q in:\n%s", want, body)
	}
}

func TestMetricsMiddlewareSkipsProbes(t *testing.T) {
	s := newMetricsServer()
	r := chi.NewRouter()
	r.Use(s.metricsMiddleware)
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	r.Get("/health", ok)
	r.Get("/ready", ok)
	r.Get("/metrics", ok)

	for _, p := range []string{"/health", "/ready", "/metrics"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", p, w.Code)
		}
	}

	body := scrapeMetrics(t, s)
	if strings.Contains(body, "http_requests_total{") {
		t.Fatalf("probe/metrics requests must not be recorded in http_requests_total:\n%s", body)
	}
}
