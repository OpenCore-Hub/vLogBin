package httpapi

import (
	"context"
	"net/http"
	"time"
)

// health is the liveness probe — returns 200 as long as the server
// process is alive and can accept connections. No dependency checks.
// Kubernetes uses this to decide whether to restart the pod.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"checked_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// dependencyPinger is implemented by ratelimit backends that can report
// whether their backing store is reachable. The in-memory limiter always
// succeeds; the Redis limiter does a real round trip so a Redis outage is
// visible to the readiness probe even though the limiter fails open at
// request time.
type dependencyPinger interface {
	Ping(context.Context) error
}

// readyDep describes one readiness dependency and its health check.
type readyDep struct {
	name string
	ping func(context.Context) error
}

// ready is the readiness probe — returns 200 only when every configured
// dependency is reachable. Kubernetes uses this to decide whether to route
// traffic to this pod; a 503 means the pod is up but not ready (e.g. DB or
// the Redis rate-limit backend is down). Dependencies are checked in
// parallel and the overall probe is bounded by readyTimeout (default 2s) so
// a hung dependency cannot block probe responses. The response body lists
// per-dependency status so operators can see exactly what is failing; the
// HTTP status code is the only signal probes rely on.
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if s.readyTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.readyTimeout)
		defer cancel()
	}

	deps := []readyDep{{name: "database", ping: s.store.Ping}}
	if p, ok := s.limiter.(dependencyPinger); ok {
		deps = append(deps, readyDep{name: "ratelimit", ping: p.Ping})
	}

	type depResult struct {
		name    string
		up      bool
		latency time.Duration
		err     error
	}
	results := make(chan depResult, len(deps))
	for _, d := range deps {
		go func() {
			start := time.Now()
			err := d.ping(ctx)
			results <- depResult{name: d.name, up: err == nil, latency: time.Since(start), err: err}
		}()
	}

	allUp := true
	var firstErr error
	depStatus := make(map[string]any, len(deps))
	for range deps {
		res := <-results
		status, detail := "up", map[string]any{"status": "up", "latency_ms": float64(res.latency.Microseconds()) / 1000.0}
		if !res.up {
			status = "down"
			detail = map[string]any{"status": "down", "latency_ms": float64(res.latency.Microseconds()) / 1000.0, "error": res.err.Error()}
			allUp = false
			if firstErr == nil {
				firstErr = res.err
			}
		}
		depStatus[res.name] = detail
		if s.metrics != nil {
			s.metrics.ReadyChecksTotal.WithLabelValues(res.name, status).Inc()
		}
	}

	checkedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if !allUp {
		s.log.Warn("readiness probe failed", "error", firstErr, "dependencies", depStatus)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":       "unavailable",
			"error":        "dependency unreachable",
			"checked_at":   checkedAt,
			"dependencies": depStatus,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ready",
		"checked_at":   checkedAt,
		"dependencies": depStatus,
	})
}

// startup is the startup probe — returns 200 only after every dependency
// (migrations, connection pool, billing, ZITADEL, background workers) is
// initialized. main calls SetStartupComplete once initialization finishes,
// so Kubernetes startupProbe stops receiving 503s at that point and the pod
// is marked Started. This gates traffic that depends on migrated schema.
func (s *Server) startup(w http.ResponseWriter, r *http.Request) {
	if !s.startupComplete.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "starting",
			"error":  "dependencies not ready",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "started",
		"checked_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
}
