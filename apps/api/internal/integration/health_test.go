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

	// Readiness probe — 200 only when every dependency is reachable. The
	// response must list each dependency with an up/down status so operators
	// can see exactly what a 503 is caused by.
	status, body = apiReq(t, "GET", "/ready", "", nil)
	if status != http.StatusOK {
		t.Fatalf("ready: status %d, want 200", status)
	}
	if body["status"] != "ready" {
		t.Fatalf("ready status = %v, want ready", body["status"])
	}
	deps, ok := body["dependencies"].(map[string]any)
	if !ok {
		t.Fatalf("ready body missing dependencies map: %v", body)
	}
	for _, name := range []string{"database", "ratelimit"} {
		dep, ok := deps[name].(map[string]any)
		if !ok {
			t.Fatalf("ready missing dependency %q: %v", name, deps)
		}
		if dep["status"] != "up" {
			t.Fatalf("dependency %q status = %v, want up", name, dep["status"])
		}
	}

	// Startup probe — 200 once migrations, the pool and the service are
	// initialized (SetStartupComplete is called by the test harness).
	status, body = apiReq(t, "GET", "/startup", "", nil)
	if status != http.StatusOK {
		t.Fatalf("startup: status %d, want 200", status)
	}
	if body["status"] != "started" {
		t.Fatalf("startup status = %v, want started", body["status"])
	}

	// Health endpoints do not require authentication.
	// (No Authorization header was sent — the requests succeeded.)
}

func TestSecurityHeaders(t *testing.T) {
	resp, err := http.Get(httpServer.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
		"Permissions-Policy":     "camera=(), microphone=(), geolocation=()",
		"Cache-Control":          "no-store",
	}
	for hdr, val := range want {
		if got := resp.Header.Get(hdr); got != val {
			t.Errorf("response header %s = %q, want %q", hdr, got, val)
		}
	}
}
