package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPoolMetricsRegistered(t *testing.T) {
	m := New()
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))

	body := w.Body.String()
	for _, name := range []string{
		"db_pool_max_conns",
		"db_pool_acquired_conns",
		"db_pool_idle_conns",
		"db_pool_constructing_conns",
		"db_pool_acquire_total",
		"db_pool_acquire_seconds_total",
		"db_pool_empty_acquire_total",
		"db_query_slow_total",
		"http_requests_rate_limited_total",
		"rate_limiter_backend_errors_total",
		"circuit_breaker_state",
		"circuit_breaker_requests_total",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("metrics output missing %q", name)
		}
	}
	// Pre-initialized counters must render a zero value on the first scrape
	// so the families exist before the pool-reporter's first sweep.
	if !strings.Contains(body, "db_pool_acquire_total 0") {
		t.Errorf("db_pool_acquire_total not pre-initialized to 0:\n%s", body)
	}
	if !strings.Contains(body, "db_pool_empty_acquire_total 0") {
		t.Errorf("db_pool_empty_acquire_total not pre-initialized to 0:\n%s", body)
	}
	// Every rate-limit level must be present (zeroed) on the first scrape.
	for _, level := range []string{"ip", "provider", "environment", "credential", "endpoint"} {
		if !strings.Contains(body, `level="`+level+`"`) {
			t.Errorf("rate-limit level %q not pre-initialized:\n%s", level, body)
		}
	}
	if !strings.Contains(body, "rate_limiter_backend_errors_total 0") {
		t.Errorf("rate_limiter_backend_errors_total not pre-initialized to 0:\n%s", body)
	}
}
