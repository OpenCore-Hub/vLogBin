package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/portal"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type portalClaimsKey struct{}

func portalClaimsFromContext(r *http.Request) (*portal.Claims, bool) {
	claims, ok := r.Context().Value(portalClaimsKey{}).(*portal.Claims)
	return claims, ok
}

// operatorIssuePortalToken — POST /v1/operator/providers/{id}/customers/{externalId}/portal-token?env=test
//
// Issues a short-lived customer portal token after verifying the customer
// exists in the provider environment. The token carries the exact data
// domain; the frontend uses it to start an isolated customer session.
func (s *Server) operatorIssuePortalToken(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	externalID := chi.URLParam(r, "externalId")
	if _, err := s.svc.GetCustomerDetail(r.Context(), service.OperatorAuthContext(providerID, env), externalID); err != nil {
		s.serviceError(w, r, err)
		return
	}
	if s.portalIssuer == nil {
		writeError(w, http.StatusServiceUnavailable, "portal_not_configured",
			"customer portal token signing is not configured", reqIDFromRequest(r))
		return
	}
	token, expiresAt, err := s.portalIssuer.Issue(providerID, env.ID, env.Kind, externalID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":       token,
		"expires_at":  expiresAt.UTC(),
		"portal_url":  "/portal/login?token=" + token,
		"customer_id": externalID,
		"environment": env.Kind,
	})
}

// portalSession — POST /v1/portal/sessions
//
// Validates a portal token presented by an invite link. The response echoes
// the token claims so the frontend can store the token in the customer
// session cookie and render the workspace name.
func (s *Server) portalSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if s.portalIssuer == nil {
		writeError(w, http.StatusServiceUnavailable, "portal_not_configured",
			"customer portal token signing is not configured", reqIDFromRequest(r))
		return
	}
	claims, err := s.portalIssuer.Verify(strings.TrimSpace(req.Token))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_portal_token",
			"portal token is invalid or expired", reqIDFromRequest(r))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"valid":                true,
		"provider_id":          claims.ProviderID,
		"environment_id":       claims.EnvironmentID,
		"environment_kind":     claims.EnvironmentKind,
		"customer_external_id": claims.CustomerExternalID,
		"expires_at":           claims.ExpiresAt,
	})
}

// portalAuthMiddleware authenticates a customer portal token and derives the
// tenant context from its claims — never from request input.
func (s *Server) portalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.portalIssuer == nil {
			writeError(w, http.StatusServiceUnavailable, "portal_not_configured",
				"customer portal token signing is not configured", reqIDFromRequest(r))
			return
		}
		raw := strings.TrimSpace(r.Header.Get("Authorization"))
		raw = strings.TrimPrefix(raw, "Bearer ")
		if raw == "" {
			writeError(w, http.StatusUnauthorized, "missing_portal_token",
				"portal token is required", reqIDFromRequest(r))
			return
		}
		claims, err := s.portalIssuer.Verify(raw)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_portal_token",
				"portal token is invalid or expired", reqIDFromRequest(r))
			return
		}
		providerID, err := uuid.Parse(claims.ProviderID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_portal_token",
				"portal token provider id is invalid", reqIDFromRequest(r))
			return
		}
		environmentID, err := uuid.Parse(claims.EnvironmentID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid_portal_token",
				"portal token environment id is invalid", reqIDFromRequest(r))
			return
		}
		kind := claims.EnvironmentKind
		if kind != domain.EnvKindTest && kind != domain.EnvKindLive {
			kind = domain.EnvKindTest
		}
		tc := tenant.Ctx{
			ProviderID:      providerID,
			EnvironmentID:   environmentID,
			EnvironmentKind: kind,
			LifecycleState:  string(domain.StateLiveActive),
		}
		ctx := tenant.WithContext(r.Context(), tc)
		ctx = contextWithPortalClaims(ctx, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func contextWithPortalClaims(ctx context.Context, claims *portal.Claims) context.Context {
	return context.WithValue(ctx, portalClaimsKey{}, claims)
}

// portalDashboard — GET /v1/portal/dashboard
//
// Returns the customer's own billing data (subscriptions, invoices, usage)
// plus the workspace name for branding. Data isolation is enforced by the
// tenant context derived from the portal token.
func (s *Server) portalDashboard(w http.ResponseWriter, r *http.Request) {
	claims, ok := portalClaimsFromContext(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_portal_token",
			"portal token is required", reqIDFromRequest(r))
		return
	}
	tc, _ := tenant.FromContext(r.Context())
	detail, err := s.svc.GetCustomerDetail(r.Context(), tc, claims.CustomerExternalID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	providerID, _ := uuid.Parse(claims.ProviderID)
	provider, err := s.svc.GetProvider(r.Context(), providerID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"provider_name": provider.Provider.Name,
		"provider_slug": provider.Provider.Slug,
		"customer":      detail.Customer,
		"subscriptions": detail.Subscriptions,
		"usage_events":  detail.UsageEvents,
		"invoices":      detail.Invoices,
	})
}
