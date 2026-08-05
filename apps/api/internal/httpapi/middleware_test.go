package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/config"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/metrics"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/ratelimit"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func testTenantCtx() tenant.Ctx {
	return tenant.Ctx{
		CredentialID:    uuid.New(),
		ProviderID:      uuid.New(),
		EnvironmentID:   uuid.New(),
		EnvironmentKind: "test",
		Scopes:          []string{"read"},
	}
}

func TestTenantConflictQueryParam(t *testing.T) {
	tc := testTenantCtx()

	r := httptest.NewRequest("GET", "/v1/credentials?provider_id="+uuid.NewString(), nil)
	if _, _, conflict := tenantConflict(r, tc); !conflict {
		t.Fatal("conflicting provider_id query param must be rejected")
	}

	r = httptest.NewRequest("GET", "/v1/credentials?environment_id="+uuid.NewString(), nil)
	if _, _, conflict := tenantConflict(r, tc); !conflict {
		t.Fatal("conflicting environment_id query param must be rejected")
	}

	// Matching values are tolerated (they carry no override).
	r = httptest.NewRequest("GET", "/v1/credentials?provider_id="+tc.ProviderID.String(), nil)
	if _, _, conflict := tenantConflict(r, tc); conflict {
		t.Fatal("matching provider_id must not be flagged")
	}

	r = httptest.NewRequest("GET", "/v1/credentials", nil)
	if _, _, conflict := tenantConflict(r, tc); conflict {
		t.Fatal("absent tenant fields must not be flagged")
	}
}

func TestTenantConflictJSONBody(t *testing.T) {
	tc := testTenantCtx()

	body := `{"name":"x","provider_id":"` + uuid.NewString() + `"}`
	r := httptest.NewRequest("POST", "/v1/credentials", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if _, _, conflict := tenantConflict(r, tc); !conflict {
		t.Fatal("conflicting provider_id in JSON body must be rejected")
	}

	body = `{"name":"x","environment_id":"` + uuid.NewString() + `"}`
	r = httptest.NewRequest("POST", "/v1/credentials", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if _, _, conflict := tenantConflict(r, tc); !conflict {
		t.Fatal("conflicting environment_id in JSON body must be rejected")
	}

	// Body without tenant fields passes, and the body must be restored.
	body = `{"name":"x"}`
	r = httptest.NewRequest("POST", "/v1/credentials", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if _, _, conflict := tenantConflict(r, tc); conflict {
		t.Fatal("body without tenant fields must pass")
	}
	buf := make([]byte, len(body))
	n, _ := r.Body.Read(buf)
	if string(buf[:n]) != body {
		t.Fatalf("body not restored: got %q", string(buf[:n]))
	}

	// Malformed JSON is not an override attempt (handler will 400 it).
	r = httptest.NewRequest("POST", "/v1/credentials", strings.NewReader(`{not json`))
	r.Header.Set("Content-Type", "application/json")
	if _, _, conflict := tenantConflict(r, tc); conflict {
		t.Fatal("malformed JSON must not be flagged as override")
	}

	// Non-JSON content type is not inspected.
	r = httptest.NewRequest("POST", "/v1/credentials", strings.NewReader("provider_id="+uuid.NewString()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, _, conflict := tenantConflict(r, tc); conflict {
		t.Fatal("non-JSON body must not be inspected")
	}
}

func TestCIDRAllowed(t *testing.T) {
	if !cidrAllowed(nil, "1.2.3.4") {
		t.Fatal("nil allowlist must allow any IP")
	}
	if !cidrAllowed([]string{}, "1.2.3.4") {
		t.Fatal("empty allowlist must allow any IP")
	}
	if !cidrAllowed([]string{"10.0.0.0/8"}, "10.1.2.3") {
		t.Fatal("IP inside allowlisted CIDR must be allowed")
	}
	if cidrAllowed([]string{"10.0.0.0/8"}, "192.168.1.1") {
		t.Fatal("IP outside allowlisted CIDR must be denied")
	}
	if cidrAllowed([]string{"10.0.0.0/8"}, "not-an-ip") {
		t.Fatal("unparseable IP must be denied")
	}
}

func TestRequireScope(t *testing.T) {
	handler := requireScope("audit:read")(http404())
	tc := testTenantCtx() // scopes: read

	r := httptest.NewRequest("GET", "/v1/audit-events", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r.WithContext(tenant.WithContext(r.Context(), tc)))
	if w.Code != 403 {
		t.Fatalf("missing scope must yield 403, got %d", w.Code)
	}

	tc.Scopes = append(tc.Scopes, "audit:read")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r.WithContext(tenant.WithContext(r.Context(), tc)))
	if w.Code != 404 { // passed through to inner handler
		t.Fatalf("sufficient scope must pass through, got %d", w.Code)
	}
}

func http404() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
}

func testIPRateLimitServer(ipLimit int) *Server {
	s := &Server{
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		limiter: ratelimit.New(),
		metrics: metrics.New(),
	}
	s.SetRateLimits(config.RateLimitConfig{IP: ipLimit, Window: time.Minute})
	return s
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")
	if got := clientIP(r); got != "203.0.113.7" {
		t.Fatalf("clientIP with XFF = %q, want 203.0.113.7", got)
	}

	r = httptest.NewRequest("GET", "/", nil)
	if got := clientIP(r); got != "192.0.2.1" {
		t.Fatalf("clientIP from RemoteAddr = %q, want 192.0.2.1", got)
	}

	r.RemoteAddr = "malformed"
	if got := clientIP(r); got != "malformed" {
		t.Fatalf("clientIP on malformed RemoteAddr = %q, want fallback", got)
	}
}

func TestIPRateLimitMiddlewareRejects(t *testing.T) {
	s := testIPRateLimitServer(2)

	ok := func(remote string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		w := httptest.NewRecorder()
		s.ipRateLimitMiddleware(http404()).ServeHTTP(w, r)
		return w
	}

	if w := ok("203.0.113.1:1234"); w.Code != 404 {
		t.Fatalf("first request must pass through, got %d", w.Code)
	}
	if w := ok("203.0.113.1:1234"); w.Code != 404 {
		t.Fatalf("second request must pass through, got %d", w.Code)
	}

	w := ok("203.0.113.1:1234")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("third request must be 429, got %d", w.Code)
	}
	ra, err := strconv.Atoi(w.Header().Get("Retry-After"))
	if err != nil || ra < 1 {
		t.Fatalf("Retry-After = %q, want a positive integer", w.Header().Get("Retry-After"))
	}

	// A different source IP is unaffected.
	if w := ok("198.51.100.9:1234"); w.Code != 404 {
		t.Fatalf("different IP must not share the bucket, got %d", w.Code)
	}
}

func TestIPRateLimitMiddlewareDisabled(t *testing.T) {
	s := testIPRateLimitServer(0) // 0 disables the layer
	for range 3 {
		r := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		s.ipRateLimitMiddleware(http404()).ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatalf("disabled per-IP limit must pass through, got %d", w.Code)
		}
	}
}

func TestIPRateLimitMiddlewareCountsMetric(t *testing.T) {
	s := testIPRateLimitServer(1)
	r := httptest.NewRequest("GET", "/", nil)
	s.ipRateLimitMiddleware(http404()).ServeHTTP(httptest.NewRecorder(), r) // allowed
	r = httptest.NewRequest("GET", "/", nil)
	s.ipRateLimitMiddleware(http404()).ServeHTTP(httptest.NewRecorder(), r) // rejected

	if got := testutil.ToFloat64(s.metrics.HTTPRateLimitedTotal.WithLabelValues("ip")); got != 1 {
		t.Fatalf("ip rate-limited metric = %v, want 1", got)
	}
}
