package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/config"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/metrics"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/ratelimit"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// TestCORSOriginsHotReload verifies that SetCORSOrigins takes effect after
// Router() has already been built: the middleware must resolve the origin
// list per request instead of capturing it at build time.
func TestCORSOriginsHotReload(t *testing.T) {
	s := NewServer(nil, nil, "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.SetCORSOrigins([]string{"https://a.example"})
	r := s.Router()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://b.example")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("origin not allowed yet but got header %q", got)
	}

	// Hot-reload: b.example becomes allowed, a.example is dropped.
	s.SetCORSOrigins([]string{"https://b.example"})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://b.example" {
		t.Fatalf("hot-reloaded origin not applied, got header %q", got)
	}

	reqA := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
	reqA.Header.Set("Origin", "https://a.example")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, reqA)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("dropped origin still allowed, got header %q", got)
	}
}

// TestRateLimitsHotReload verifies that SetRateLimits changes take effect on
// subsequent requests: the middleware must read the current config instead of
// capturing it at build time.
//
// rateLimitMiddleware only runs on tenant-authenticated /v1 routes (which need
// a database-backed API key), so this test drives it through a minimal router
// with a tenant context injected directly. The middleware chain is built once
// — like the real Router() — and only the limiter config is swapped.
func TestRateLimitsHotReload(t *testing.T) {
	s := NewServer(nil, nil, "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.SetRateLimits(config.RateLimitConfig{
		Provider: 1, Environment: 1000, Credential: 1000, Endpoint: 1000,
		IP: 0, Window: time.Minute,
	})

	mini := chi.NewRouter()
	mini.Use(s.rateLimitMiddleware)
	mini.Get("/x", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tc := tenant.Ctx{
		ProviderID:    uuid.New(),
		EnvironmentID: uuid.New(),
		CredentialID:  uuid.New(),
	}
	req := func() *httptest.ResponseRecorder {
		rq := httptest.NewRequest(http.MethodGet, "/x", nil)
		rq = rq.WithContext(tenant.WithContext(rq.Context(), tc))
		w := httptest.NewRecorder()
		mini.ServeHTTP(w, rq)
		return w
	}

	// First request consumes the only provider token.
	if w := req(); w.Code != http.StatusOK {
		t.Fatalf("first request: status = %d, want 200", w.Code)
	}
	if w := req(); w.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: status = %d, want 429", w.Code)
	}

	// Hot-reload to a higher provider limit: requests pass again.
	s.SetRateLimits(config.RateLimitConfig{
		Provider: 10, Environment: 1000, Credential: 1000, Endpoint: 1000,
		IP: 0, Window: time.Minute,
	})
	if w := req(); w.Code != http.StatusOK {
		t.Fatalf("request after hot-reload: status = %d, want 200", w.Code)
	}
}

// TestSlowRequestThresholdHotReload verifies that SetSlowRequestThreshold is
// read per request and that the getter reflects the stored value.
func TestSlowRequestThresholdHotReload(t *testing.T) {
	s := NewServer(nil, nil, "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	if got := s.SlowRequestThreshold(); got != 0 {
		t.Fatalf("default threshold = %v, want 0", got)
	}
	s.SetSlowRequestThreshold(750 * time.Millisecond)
	if got := s.SlowRequestThreshold(); got != 750*time.Millisecond {
		t.Fatalf("threshold after set = %v, want 750ms", got)
	}
}

// TestRateLimitsGetterRoundTrip guards the getter used by the config reloader
// and by tests.
func TestRateLimitsGetterRoundTrip(t *testing.T) {
	s := NewServer(nil, nil, "", slog.New(slog.NewTextHandler(io.Discard, nil)))
	want := config.RateLimitConfig{
		Provider: 1, Environment: 2, Credential: 3, Endpoint: 4, IP: 5,
		Window: 30 * time.Second,
	}
	s.SetRateLimits(want)
	got := s.RateLimits()
	if got != want {
		t.Fatalf("RateLimits() = %+v, want %+v", got, want)
	}
}

// TestNewServerZeroValueUsage ensures a bare &Server{} (as used in tests) can
// still serve: the atomic getters must tolerate a never-Store'd field.
func TestNewServerZeroValueUsage(t *testing.T) {
	s := &Server{
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		limiter: ratelimit.New(),
		metrics: metrics.New(),
	}
	_ = s.RateLimits()
	_ = s.CORSOrigins()
	_ = s.SlowRequestThreshold()
}
