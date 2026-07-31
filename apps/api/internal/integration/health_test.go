package integration

import (
	"net/http"
	"testing"
)

func TestHealthEndpoints(t *testing.T) {
	// Liveness probe — always 200 when the process is alive.
	status, body := apiReq(t, "GET", "/health", "", nil)
	if status != http.StatusOK {
		t.Fatalf("health: status %d, want 200", status)
	}
	if body["status"] != "ok" {
		t.Fatalf("health status = %v, want ok", body["status"])
	}

	// Readiness probe — 200 only when DB is reachable.
	status, body = apiReq(t, "GET", "/ready", "", nil)
	if status != http.StatusOK {
		t.Fatalf("ready: status %d, want 200", status)
	}
	if body["status"] != "ready" {
		t.Fatalf("ready status = %v, want ready", body["status"])
	}

	// Health endpoints do not require authentication.
	// (No Authorization header was sent — the requests succeeded.)
}
