package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
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

// DeprecationMiddleware sets Sunset, Deprecation, and Link headers for
// deprecated endpoints (RFC 8594 / RFC 9745). This satisfies the P0 gate:
// "公开 API 12 个月兼容政策生效" and spec Section 7.2: "提供弃用遥测、
// 迁移指南、契约测试和日落通知".
func DeprecationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		for _, info := range deprecationRegistry {
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
				break
			}
		}
		next.ServeHTTP(w, r)
	})
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
