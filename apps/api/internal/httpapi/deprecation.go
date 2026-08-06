package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// APIVersion is the current API version.
const APIVersion = "v1"

// CompatibilityPolicy is the human-readable API compatibility policy
// (spec Section 7.2: "破坏性版本至少并行 12 个月").
const CompatibilityPolicy = "Breaking API versions run in parallel for at least 12 months. Deprecated endpoints return Sunset and Deprecation headers indicating the removal date."

// DeprecationInfo describes a deprecated endpoint.
type DeprecationInfo struct {
	PathPattern  string    // HTTP method + space + path pattern (e.g., "POST /v1/old-endpoint")
	DeprecatedAt time.Time // When the endpoint was deprecated
	SunsetAt     time.Time // When the endpoint will be removed (>= 12 months after DeprecatedAt)
	Replacement  string    // The replacement endpoint (empty if none)
}

// deprecationRegistry tracks deprecated endpoints. Endpoints are registered
// at startup. The middleware checks each request against the registry and
// sets Sunset, Deprecation, and Link headers for deprecated endpoints.
var deprecationRegistry = []DeprecationInfo{
	// No deprecated endpoints yet — the registry is ready for future use.
	// When an endpoint is deprecated, add an entry here with a sunset date
	// at least 12 months in the future.
}

func validateDeprecationRegistry(registry []DeprecationInfo) error {
	for _, info := range registry {
		if info.PathPattern == "" {
			return fmt.Errorf("deprecated endpoint path pattern is required")
		}
		if info.DeprecatedAt.IsZero() || info.SunsetAt.IsZero() {
			return fmt.Errorf("deprecated endpoint %q requires deprecated_at and sunset_at", info.PathPattern)
		}
		if info.SunsetAt.Before(info.DeprecatedAt.AddDate(0, 12, 0)) {
			return fmt.Errorf("deprecated endpoint %q sunset must be at least 12 months after deprecation", info.PathPattern)
		}
	}
	return nil
}

// ValidateDeprecationRegistry verifies the startup deprecation registry
// enforces the 12-month parallel compatibility policy.
func ValidateDeprecationRegistry() error {
	return validateDeprecationRegistry(deprecationRegistry)
}

func deprecationMiddlewareWithRegistry(
	next http.Handler,
	registry []DeprecationInfo,
	counter *prometheus.CounterVec,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		for _, info := range registry {
			if matchDeprecatedPath(key, info.PathPattern) {
				w.Header().Set("Deprecation", "true")
				w.Header().Set("Sunset", info.SunsetAt.Format(time.RFC1123))
				if info.Replacement != "" {
					w.Header().Set("Link", fmt.Sprintf("<%s>; rel=\"successor-version\"", info.Replacement))
				}
				// Deprecation telemetry: log each hit to a deprecated endpoint
				// (spec Section 7.2: "提供弃用遥测"). In production, this would
				// feed into a metrics pipeline (Prometheus, Datadog, etc.).
				slog.Info("deprecated endpoint hit",
					"method", r.Method,
					"path", r.URL.Path,
					"pattern", info.PathPattern,
					"sunset", info.SunsetAt.Format(time.RFC3339),
					"replacement", info.Replacement,
					"request_id", w.Header().Get("X-Request-ID"),
				)
				if counter != nil {
					counter.WithLabelValues(r.URL.Path).Inc()
				}
				break
			}
		}
		next.ServeHTTP(w, r)
	})
}

// DeprecationMiddleware sets Sunset, Deprecation, and Link headers for
// deprecated endpoints (RFC 8594 / RFC 9745).
func DeprecationMiddleware(next http.Handler) http.Handler {
	return deprecationMiddlewareWithRegistry(next, deprecationRegistry, nil)
}

// DeprecationMiddlewareWithCounter also records deprecated endpoint usage
// into the provided Prometheus counter for sunset migration telemetry.
func DeprecationMiddlewareWithCounter(next http.Handler, counter *prometheus.CounterVec) http.Handler {
	return deprecationMiddlewareWithRegistry(next, deprecationRegistry, counter)
}

func (s *Server) deprecationMiddleware(next http.Handler) http.Handler {
	return DeprecationMiddlewareWithCounter(next, s.metrics.HTTPDeprecatedUsageTotal)
}

// matchDeprecatedPath checks if the request key matches the deprecation
// pattern. Supports exact matching and wildcard suffix matching (e.g.,
// "GET /v1/old/*" matches "GET /v1/old/anything").
func matchDeprecatedPath(key, pattern string) bool {
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(key, prefix)
	}
	return key == pattern
}

// GetDeprecatedEndpoints returns the list of deprecated endpoints for
// the API version info endpoint.
func GetDeprecatedEndpoints() []map[string]any {
	var out []map[string]any
	for _, info := range deprecationRegistry {
		out = append(out, map[string]any{
			"path":          info.PathPattern,
			"deprecated_at": info.DeprecatedAt.Format(time.RFC3339),
			"sunset_at":     info.SunsetAt.Format(time.RFC3339),
			"replacement":   info.Replacement,
		})
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out
}
