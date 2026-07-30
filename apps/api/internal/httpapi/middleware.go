package httpapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/keys"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type requestIDKey struct{}

// requestIDMiddleware assigns every request a request id, propagated into
// responses and available for audit correlation.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// operatorAuth authenticates /v1/operator/* with the configured operator
// token (constant-time compare).
func (s *Server) operatorAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(s.operatorToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid operator token")
			return
		}
		next.ServeHTTP(w, r.WithContext(tenant.WithOperator(r.Context())))
	})
}

// apiKeyAuth authenticates provider routes: resolves the presented API key
// to its credential row, checks revocation/expiry/CIDR, and derives the
// tenant context from the credential — never from request input.
func (s *Server) apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, ok := bearerToken(r)
		if !ok || (!strings.HasPrefix(key, keys.PrefixTest) && !strings.HasPrefix(key, keys.PrefixLive)) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "missing or malformed API key")
			return
		}
		var row storegen.ResolveCredentialByKeyHashRow
		err := s.store.WithOperator(r.Context(), func(tx pgx.Tx, q *store.Queries) error {
			var err error
			row, err = q.ResolveCredentialByKeyHash(r.Context(), keys.Hash(key))
			return err
		})
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid API key")
			return
		}
		if row.RevokedAt != nil {
			writeError(w, http.StatusUnauthorized, "credential_revoked", "API key has been revoked")
			return
		}
		if row.ExpiresAt != nil && time.Now().After(*row.ExpiresAt) {
			writeError(w, http.StatusUnauthorized, "credential_expired", "API key has expired")
			return
		}
		if !cidrAllowed(row.AllowedCidrs, remoteIP(r)) {
			writeError(w, http.StatusForbidden, "cidr_not_allowed", "source IP is not allowed for this API key")
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
				writeError(w, http.StatusUnauthorized, "unauthorized", "missing tenant context")
				return
			}
			if !tc.HasScope(scope) {
				writeError(w, http.StatusForbidden, "insufficient_scope", "credential lacks required scope "+scope)
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
				"provider_id/environment_id must come from the credential, not the request")
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
