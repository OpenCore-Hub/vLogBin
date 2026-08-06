package httpapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/keys"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/reqid"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/zitadel"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// maxBodySize limits request bodies to 1 MB to prevent DoS via large
// payloads. Billing API requests are small JSON objects; 1 MB is generous.
const maxBodySize = 1 << 20

// bodyLimitMiddleware wraps r.Body with an http.MaxBytesReader so that
// requests exceeding maxBodySize are rejected with 413.
func bodyLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		next.ServeHTTP(w, r)
	})
}

// securityHeadersMiddleware applies baseline security headers to every
// response and disables caching. API responses may embed provider-scoped
// data, so browsers and intermediate caches must not retain them; no-store
// also keeps operator/provider payloads out of shared caches.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// recoverMiddleware catches panics in handlers, logs them with the request
// ID, and returns 500. Without this, a single handler panic would crash
// the entire server process.
func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				reqID := reqid.FromContext(r.Context())
				s.log.Error("panic recovered",
					"error", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", reqID,
				)
				writeError(w, http.StatusInternalServerError, "internal", "internal server error", reqID)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// rateLimitMiddleware enforces 4-level rate limiting (spec Section 7.3):
// Provider, Environment, Credential, and Endpoint. Each level has its own
// limit; if any level is exceeded, the request is rejected with 429.
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc, ok := tenant.FromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		// Get the route pattern (e.g. /v1/usage/ingest) so that
		// different UUIDs in the path don't create separate buckets.
		routePattern := chi.RouteContext(r.Context()).RoutePattern()
		if routePattern == "" {
			routePattern = r.URL.Path
		}
		endpointKey := r.Method + ":" + routePattern

		rl := s.RateLimits()
		limits := []struct {
			key   string
			level string
			limit int
		}{
			{"provider:" + tc.ProviderID.String(), "provider", rl.Provider},
			{"env:" + tc.EnvironmentID.String(), "environment", rl.Environment},
			{"cred:" + tc.CredentialID.String(), "credential", rl.Credential},
			{"cred_ep:" + tc.CredentialID.String() + ":" + endpointKey, "endpoint", rl.Endpoint},
		}

		for _, lm := range limits {
			ok, retry := s.limiter.AllowRetryAfter(lm.key, lm.limit, rl.Window)
			if ok {
				continue
			}
			// Retry-After reflects the actual remaining time in the fixed
			// window, rounded up to whole seconds (never 0/negative).
			secs := max(int64(retry/time.Second), 1)
			w.Header().Set("Retry-After", strconv.FormatInt(secs, 10))
			writeError(w, http.StatusTooManyRequests, "rate_limited",
				"rate limit exceeded; retry after the window resets",
				reqIDFromRequest(r),
				map[string]any{"retry_after": strconv.FormatInt(secs, 10)})
			s.log.Warn("rate limit exceeded",
				"key", lm.key,
				"level", lm.level,
				"limit", lm.limit,
				"request_id", reqIDFromRequest(r),
			)
			s.metrics.HTTPRateLimitedTotal.WithLabelValues(lm.level).Inc()
			return
		}

		next.ServeHTTP(w, r)
	})
}

// clientIP returns the client source IP used for the per-IP rate limit: the
// first entry of X-Forwarded-For when present (production sits behind a
// reverse proxy), otherwise the remote address host. The trusted proxy must
// overwrite X-Forwarded-For so clients cannot spoof it.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = xff[:i]
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ipRateLimitMiddleware applies a global per-source-IP safety net before
// authentication. A client that rotates credentials would otherwise get a
// fresh bucket per credential, so this layer guarantees a hard ceiling per
// source address and also protects unauthenticated endpoints (health,
// metrics) from naive DoS. Disabled when RateLimits.IP is 0.
func (s *Server) ipRateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rl := s.RateLimits()
		if rl.IP <= 0 {
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIP(r)
		if ip == "" {
			next.ServeHTTP(w, r)
			return
		}
		ok, retry := s.limiter.AllowRetryAfter("ip:"+ip, rl.IP, rl.Window)
		if ok {
			next.ServeHTTP(w, r)
			return
		}
		secs := max(int64(retry/time.Second), 1)
		w.Header().Set("Retry-After", strconv.FormatInt(secs, 10))
		writeError(w, http.StatusTooManyRequests, "rate_limited",
			"rate limit exceeded; retry after the window resets",
			reqIDFromRequest(r),
			map[string]any{"retry_after": strconv.FormatInt(secs, 10)})
		s.log.Warn("per-ip rate limit exceeded",
			"ip", ip,
			"limit", rl.IP,
			"request_id", reqIDFromRequest(r),
		)
		s.metrics.HTTPRateLimitedTotal.WithLabelValues("ip").Inc()
	})
}

// corsMiddleware handles Cross-Origin Resource Sharing. In development the
// default "*" allows the Next.js dev server (port 3000) to call the API
// (port 8080). In production, CORS_ALLOWED_ORIGINS should be set to the
// specific frontend origin(s).
//
// It takes a provider function instead of a static slice: the provider is
// invoked inside the per-request handler, so the current origin list is
// resolved atomically on every request. This keeps CORS hot-reloadable even
// though chi caches the middleware chain after the first request.
func corsMiddleware(origins func() []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				for _, o := range origins() {
					if o == "*" || o == origin {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
						w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
						w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
						w.Header().Set("Access-Control-Max-Age", "3600")
						break
					}
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// corsMiddleware is the Server variant used in the router. It hands the
// CORSOrigins accessor to corsMiddleware; because the accessor is called
// per-request, SetCORSOrigins takes effect without a router rebuild/restart.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return corsMiddleware(s.CORSOrigins)(next)
}

// requestIDMiddleware assigns every request a request id, propagated into
// responses and available for audit correlation.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := reqid.WithValue(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// statusRecorder wraps http.ResponseWriter to capture the response status
// code for structured request logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// metricsMiddleware records HTTP request count and latency into the
// Prometheus registry. Probes (/health, /ready, /startup) and the /metrics
// endpoint itself are skipped to keep scrape and health traffic out of API
// metrics.
func (s *Server) metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/startup" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)

		var route string
		if rc := chi.RouteContext(r.Context()); rc != nil {
			route = rc.RoutePattern()
		}
		if route == "" {
			// Outside a chi router (or for unmatched paths) fall back to the
			// raw path so no request goes unrecorded.
			route = r.URL.Path
		}
		s.metrics.HTTPRequestsTotal.WithLabelValues(r.Method, route, fmt.Sprintf("%d", sr.status)).Inc()
		s.metrics.HTTPRequestSeconds.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

// requestTimeoutMiddleware bounds the whole handler lifetime. A request whose
// handler outlives the deadline gets its context cancelled; the service layer
// then fails fast and serviceError maps context.DeadlineExceeded to
// 504 upstream_timeout. This prevents a hung upstream (e.g. a stuck Postgres
// query) from pinning a goroutine and request slot forever.
func (s *Server) requestTimeoutMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.requestTimeout <= 0 {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requestLogMiddleware logs every request (except health probes) with
// method, path, status, duration, and request_id for production
// observability and audit correlation. Credential-like query parameters are
// redacted before logging, and requests slower than the configured threshold
// are escalated to Warn with slow=true so tail latency is visible without
// noisy debug logging.
func (s *Server) requestLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip health-check endpoints to avoid log noise from probes.
		if r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/startup" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(sr, r)

		reqID := reqid.FromContext(r.Context())
		duration := time.Since(start)

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", sr.status,
			"duration_ms", duration.Milliseconds(),
			"request_id", reqID,
			"remote_addr", remoteIP(r),
		}
		if q := redactQuery(r.URL.RawQuery); q != "" {
			attrs = append(attrs, "query", q)
		}

		thr := s.SlowRequestThreshold()
		slow := thr > 0 && duration >= thr
		if slow {
			attrs = append(attrs, "slow", true, "threshold_ms", thr.Milliseconds())
		}

		switch {
		case sr.status >= 500:
			s.log.Error("request", attrs...)
		case slow || sr.status >= 400:
			s.log.Warn("request", attrs...)
		default:
			s.log.Info("request", attrs...)
		}
	})
}

// sensitiveQueryKeys lists credential-like query parameter names whose values
// must never reach the logs. Matching is case-insensitive and also covers
// suffixed variants (e.g. access_token, x-api-key).
var sensitiveQueryKeys = []string{
	"token", "key", "secret", "password", "passwd",
	"api_key", "apikey", "access_token", "refresh_token",
	"signature", "sig", "code",
}

func isSensitiveQueryKey(k string) bool {
	k = strings.ToLower(k)
	if slices.Contains(sensitiveQueryKeys, k) {
		return true
	}
	for _, suffix := range []string{"token", "key", "secret", "password", "signature"} {
		if strings.HasSuffix(k, "_"+suffix) || strings.HasSuffix(k, "-"+suffix) {
			return true
		}
	}
	return false
}

// redactQuery returns a log-safe copy of a raw query string: values of
// credential-like parameters are replaced with "REDACTED" and keys are sorted
// for stable output. An empty input yields an empty string; an unparseable
// input yields a placeholder so nothing is ever logged verbatim.
func redactQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "REDACTED_INVALID_QUERY"
	}
	for k := range values {
		if isSensitiveQueryKey(k) {
			values[k] = []string{"REDACTED"}
		}
	}
	// Encode sorts keys and escapes values, giving stable, log-safe output.
	return values.Encode()
}

// operatorAuth authenticates /v1/operator/* with either a ZITADEL OIDC
// token (when oidcVerifier is configured) or the static OPERATOR_TOKEN
// (backward compatible fallback).
func (s *Server) operatorAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or malformed bearer token", reqIDFromRequest(r))
			return
		}

		if s.oidcVerifier != nil {
			// OIDC mode: verify JWT access token from ZITADEL.
			claims, err := s.oidcVerifier.Verify(r.Context(), token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid_token", err.Error(), reqIDFromRequest(r))
				return
			}
			ctx := context.WithValue(r.Context(), operatorClaimsKey{}, claims)
			next.ServeHTTP(w, r.WithContext(tenant.WithOperator(ctx)))
			return
		}

		// Legacy mode: constant-time token comparison.
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.operatorToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid operator token", reqIDFromRequest(r))
			return
		}
		next.ServeHTTP(w, r.WithContext(tenant.WithOperator(r.Context())))
	})
}

// operatorClaimsKey is the context key for OIDC operator claims.
type operatorClaimsKey struct{}

// operatorIdentity extracts the operator identity from the request context.
// In OIDC mode, this is the JWT subject (sub). In legacy mode (static token),
// it falls back to "operator".
func operatorIdentity(r *http.Request) string {
	if claims, ok := r.Context().Value(operatorClaimsKey{}).(*zitadel.Claims); ok && claims != nil {
		if claims.Sub != "" {
			return claims.Sub
		}
	}
	return "operator"
}

// apiKeyAuth authenticates provider routes: resolves the presented API key
// to its credential row, checks revocation/expiry/CIDR, and derives the
// tenant context from the credential — never from request input.
func (s *Server) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, ok := bearerToken(r)
		if !ok || (!strings.HasPrefix(key, keys.PrefixTest) && !strings.HasPrefix(key, keys.PrefixLive)) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or malformed API key", reqIDFromRequest(r))
			return
		}
		var row storegen.ResolveCredentialByKeyHashRow
		err := s.store.WithOperator(r.Context(), func(tx pgx.Tx, q *store.Queries) error {
			var err error
			row, err = q.ResolveCredentialByKeyHash(r.Context(), keys.Hash(key))
			return err
		})
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid API key", reqIDFromRequest(r))
			return
		}
		if row.RevokedAt != nil {
			writeError(w, http.StatusUnauthorized, "credential_revoked", "API key has been revoked", reqIDFromRequest(r))
			return
		}
		if row.ExpiresAt != nil && time.Now().After(*row.ExpiresAt) {
			writeError(w, http.StatusUnauthorized, "credential_expired", "API key has expired", reqIDFromRequest(r))
			return
		}
		if !cidrAllowed(row.AllowedCidrs, remoteIP(r)) {
			writeError(w, http.StatusForbidden, "cidr_not_allowed", "source IP is not allowed for this API key", reqIDFromRequest(r))
			return
		}
		// Best-effort usage stamp; failures must not block the request.
		_ = s.store.WithOperator(r.Context(), func(tx pgx.Tx, q *store.Queries) error {
			return q.TouchCredentialLastUsed(r.Context(), row.CredentialID)
		})

		tc := tenant.Ctx{
			CredentialID:    row.CredentialID,
			ProviderID:      row.ProviderID,
			ProviderSlug:    row.ProviderSlug,
			LifecycleState:  row.LifecycleState,
			EnvironmentID:   row.EnvironmentID,
			EnvironmentKind: row.EnvironmentKind,
			Issuer:          row.Issuer,
			Scopes:          row.Scopes,
		}
		next.ServeHTTP(w, r.WithContext(tenant.WithContext(r.Context(), tc)))
	})
}

// requireScope rejects requests whose credential lacks the given scope.
func requireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tc, ok := tenant.FromContext(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "unauthorized", "missing tenant context", reqIDFromRequest(r))
				return
			}
			if !tc.HasScope(scope) {
				writeError(w, http.StatusForbidden, "insufficient_scope", "credential lacks required scope "+scope, reqIDFromRequest(r))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// tenantGuard rejects any attempt to override the credential-derived tenant
// context via query parameters or JSON body fields. Conflicts produce a 403
// and an audit record.
func (s *Server) tenantGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc, ok := tenant.FromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		if field, presented, conflict := tenantConflict(r, tc); conflict {
			if err := s.svc.RecordTenantOverrideAttempt(r.Context(), tc, field, presented); err != nil {
				s.log.Error("audit tenant override attempt failed", "error", err)
			}
			writeError(w, http.StatusForbidden, "tenant_context_override",
				"provider_id/environment_id must come from the credential, not the request", reqIDFromRequest(r))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// environmentHeaderMiddleware enforces the documented X-Environment contract
// for provider-domain requests: when the client sends the header it must
// match the environment bound to its API key. Environment isolation is still
// enforced by the credential itself (tenant context always comes from the
// key); this makes accidental cross-environment calls fail fast instead of
// silently succeeding, and gives SDKs a deterministic signal when they wired
// the wrong environment.
func (s *Server) environmentHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tc, ok := tenant.FromContext(r.Context())
		if ok {
			if header := r.Header.Get("X-Environment"); header != "" && header != tc.EnvironmentKind {
				writeError(w, http.StatusBadRequest, "environment_mismatch",
					"X-Environment does not match the credential environment", reqIDFromRequest(r))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// lifecycleWriteGuard enforces a read-only mode on the provider API while the
// provider is not in a writable lifecycle state (REGISTERED, SUSPENDED,
// OFFBOARDING). Write methods (POST/PUT/PATCH/DELETE) are rejected with 409
// provider_not_writable; read methods (GET/HEAD/OPTIONS) always pass so a
// suspended provider can still inspect its own data for audit/forensics.
// The lifecycle state comes from the credential-derived tenant context, so
// the guard is real-time and requires no extra query.
func (s *Server) lifecycleWriteGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		tc, ok := tenant.FromContext(r.Context())
		if ok && !domain.CanWrite(domain.LifecycleState(tc.LifecycleState)) {
			writeError(w, http.StatusConflict, "provider_not_writable",
				fmt.Sprintf("provider %s is in state %s: write operations are disabled",
					tc.ProviderID, tc.LifecycleState), reqIDFromRequest(r))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tenantConflict inspects query params and (for JSON requests) the body for
// provider_id / environment_id values conflicting with the tenant context.
// The body is restored for downstream handlers.
func tenantConflict(r *http.Request, tc tenant.Ctx) (field, presented string, conflict bool) {
	for _, f := range []string{"provider_id", "environment_id"} {
		if v := r.URL.Query().Get(f); v != "" {
			if f == "provider_id" && v != tc.ProviderID.String() {
				return f, v, true
			}
			if f == "environment_id" && v != tc.EnvironmentID.String() {
				return f, v, true
			}
		}
	}
	if r.Body == nil || !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		return "", "", false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	_ = r.Body.Close()
	if err != nil {
		return "", "", false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var parsed struct {
		ProviderID    string `json:"provider_id"`
		EnvironmentID string `json:"environment_id"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", "", false // malformed JSON is the handler's 400, not an override attempt
	}
	if parsed.ProviderID != "" && parsed.ProviderID != tc.ProviderID.String() {
		return "provider_id", parsed.ProviderID, true
	}
	if parsed.EnvironmentID != "" && parsed.EnvironmentID != tc.EnvironmentID.String() {
		return "environment_id", parsed.EnvironmentID, true
	}
	return "", "", false
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	return token, token != ""
}

func remoteIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// cidrAllowed reports whether ip is permitted by the CIDR allowlist. An
// empty/nil allowlist permits every source.
func cidrAllowed(cidrs []string, ip string) bool {
	if len(cidrs) == 0 {
		return true
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, c := range cidrs {
		if prefix, err := netip.ParsePrefix(c); err == nil && prefix.Contains(addr) {
			return true
		}
	}
	return false
}
