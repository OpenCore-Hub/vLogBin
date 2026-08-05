package integration

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestMetricsEndpoint verifies the Prometheus /metrics endpoint is mounted
// on the HTTP router and exposes the platform metric families promised by
// docs/DEPLOYMENT.md §5.2-5.3 (HTTP 5xx rate, P99 latency, outbox backlog).
func TestMetricsEndpoint(t *testing.T) {
	// Warm up at least one API request so http_requests_total has a sample
	// with the route label populated (api-version is public, no auth).
	apiReq(t, "GET", "/v1/api-version", "", nil)

	resp, err := http.Get(httpServer.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read /metrics body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics: status %d, body: %.300s", resp.StatusCode, body)
	}
	text := string(body)

	for _, want := range []string{
		"go_goroutines",
		"http_requests_total",
		"http_request_duration_seconds",
		"webhook_deliveries_total",
		"webhook_delivery_duration_seconds",
		"outbox_events_total",
		"webhook_deliveries",
		"sweep_deleted_total",
		"db_pool_max_conns",
		"db_pool_acquired_conns",
		"db_pool_idle_conns",
		"db_pool_constructing_conns",
		"db_pool_acquire_total",
		"db_pool_acquire_seconds_total",
		"db_pool_empty_acquire_total",
		"db_query_slow_total",
		"http_requests_rate_limited_total",
		"circuit_breaker_state",
		"circuit_breaker_requests_total",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("/metrics missing metric family %q", want)
		}
	}

	// The warm-up request must show up in http_requests_total with the
	// route pattern (low cardinality) and the 2xx status label.
	if !strings.Contains(text, `http_requests_total{method="GET",route="/v1/api-version",status="200"}`) {
		t.Errorf("/metrics missing warm-up sample for /v1/api-version:\n%.2000s", text)
	}
}
