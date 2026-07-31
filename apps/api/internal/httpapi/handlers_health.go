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
		"status":   "ok",
		"checked_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// ready is the readiness probe — returns 200 only when the database
// is reachable. Kubernetes uses this to decide whether to route traffic
// to this pod. A 503 means the pod is up but not ready (e.g. DB is down).
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.store.Ping(ctx); err != nil {
		s.log.Warn("readiness probe failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":  "unavailable",
			"error":   "database unreachable",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ready",
		"checked_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
}
