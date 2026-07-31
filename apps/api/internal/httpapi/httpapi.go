// Package httpapi exposes the platform v1 HTTP API: operator routes
// (operator token) and provider routes (API-key credentials) with a
// consistent error shape {"error":{"code","message"}}.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/config"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/ratelimit"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/zitadel"
	"github.com/go-chi/chi/v5"
)

type Server struct {
	store         *store.Store
	svc           *service.Service
	operatorToken string
	log           *slog.Logger
	corsOrigins   []string
	limiter       *ratelimit.Limiter
	rateLimits    config.RateLimitConfig
	oidcVerifier  *zitadel.Verifier
}

func NewServer(st *store.Store, svc *service.Service, operatorToken string, log *slog.Logger) *Server {
	return &Server{
		store:         st,
		svc:           svc,
		operatorToken: operatorToken,
		log:           log,
		corsOrigins:   []string{"*"},
		limiter:       ratelimit.New(),
		rateLimits: config.RateLimitConfig{
			Provider: 1000, Environment: 500, Credential: 200, Endpoint: 60,
			Window: time.Minute,
		},
	}
}

// SetRateLimits configures the 4-level rate limit settings.
func (s *Server) SetRateLimits(rl config.RateLimitConfig) {
	s.rateLimits = rl
}

// SetOIDCVerifier configures ZITADEL OIDC token verification for
// operator auth. When set, operator tokens are verified as JWTs.
func (s *Server) SetOIDCVerifier(v *zitadel.Verifier) {
	s.oidcVerifier = v
}

// SetCORSOrigins configures the allowed CORS origins. Must be called
// before Router().
func (s *Server) SetCORSOrigins(origins []string) {
	if len(origins) > 0 {
		s.corsOrigins = origins
	}
}

func (s *Server) Router() chi.Router {
	r := chi.NewRouter()
	r.Use(s.recoverMiddleware)
	r.Use(corsMiddleware(s.corsOrigins))
	r.Use(bodyLimitMiddleware)
	r.Use(requestIDMiddleware)
	r.Use(s.requestLogMiddleware)

	// Health-check endpoints (no auth — Kubernetes probes).
	r.Get("/health", s.health)
	r.Get("/ready", s.ready)

	r.Route("/v1", func(r chi.Router) {
		r.Route("/operator", func(r chi.Router) {
			r.Use(s.operatorAuth)
			r.Post("/providers", s.createProvider)
			r.Get("/providers", s.listProviders)
			r.Get("/providers/{id}", s.getProvider)
			r.Post("/providers/{id}/lifecycle", s.transitionLifecycle)
			r.Get("/regions", s.listRegions)
			r.Get("/cells", s.listCells)
			// Operator billing views (cross-environment, read-only).
			r.Get("/providers/{id}/catalog/versions", s.operatorListCatalogVersions)
			r.Get("/providers/{id}/catalog/versions/{versionId}", s.operatorGetCatalogVersion)
			r.Get("/providers/{id}/subscriptions", s.operatorListSubscriptions)
			r.Get("/providers/{id}/customers", s.operatorListCustomers)
			r.Get("/providers/{id}/usage-events", s.operatorListUsageEvents)
			r.Get("/providers/{id}/invoices", s.operatorListInvoices)
			// Provider capability grants (operator-managed).
			r.Get("/providers/{id}/capabilities", s.operatorListCapabilities)
			r.Post("/providers/{id}/capabilities/{capability}/grant", s.operatorGrantCapability)
			r.Post("/providers/{id}/capabilities/{capability}/revoke", s.operatorRevokeCapability)
			// Reconciliation results (operator monitoring).
			r.Get("/reconciliation-results", s.operatorListReconciliationResults)
			// Webhook monitoring (operator view, cross-environment).
			r.Get("/providers/{id}/webhooks", s.operatorListWebhooks)
			r.Get("/providers/{id}/webhook-deliveries", s.operatorListWebhookDeliveries)
		})
		r.Group(func(r chi.Router) {
			r.Use(s.apiKeyAuth)
			r.Use(s.tenantGuard)
			r.Use(s.rateLimitMiddleware)
			r.With(requireScope(domain.ScopeRead)).Get("/whoami", s.whoami)
			r.With(requireScope(domain.ScopeRead)).Get("/credentials", s.listCredentials)
			r.With(requireScope(domain.ScopeCredentialsManage)).Post("/credentials", s.createCredential)
			r.With(requireScope(domain.ScopeCredentialsManage)).Post("/credentials/{id}/revoke", s.revokeCredential)
			r.With(requireScope(domain.ScopeAuditRead)).Get("/audit-events", s.listAuditEvents)
			r.With(requireScope(domain.ScopeRead)).Get("/outbox-events", s.listOutboxEvents)
			// Catalog (write scope for mutations, read for gets).
			r.With(requireScope(domain.ScopeWrite)).Post("/catalog/versions", s.createCatalogVersion)
			r.With(requireScope(domain.ScopeRead)).Get("/catalog/versions", s.listCatalogVersions)
			r.With(requireScope(domain.ScopeRead)).Get("/catalog/versions/{versionId}", s.getCatalogVersion)
			r.With(requireScope(domain.ScopeWrite)).Put("/catalog/versions/{versionId}/content", s.replaceCatalogContent)
			r.With(requireScope(domain.ScopeWrite)).Post("/catalog/versions/{versionId}/validate", s.validateCatalogVersion)
			r.With(requireScope(domain.ScopeWrite)).Post("/catalog/versions/{versionId}/publish", s.publishCatalogVersion)
			r.With(requireScope(domain.ScopeWrite)).Post("/catalog/versions/{versionId}/retire", s.retireCatalogVersion)
			// Customers.
			r.With(requireScope(domain.ScopeWrite)).Post("/customers", s.createCustomer)
			r.With(requireScope(domain.ScopeRead)).Get("/customers", s.listCustomers)
			// Subscriptions.
			r.With(requireScope(domain.ScopeWrite)).Post("/subscriptions", s.createSubscription)
			r.With(requireScope(domain.ScopeRead)).Get("/subscriptions", s.listSubscriptions)
			r.With(requireScope(domain.ScopeWrite)).Post("/subscriptions/{id}/terminate", s.terminateSubscription)
			// Invoices (sync pull + read views).
			r.With(requireScope(domain.ScopeWrite)).Post("/invoices/sync", s.syncInvoices)
			r.With(requireScope(domain.ScopeRead)).Get("/invoices", s.listInvoices)
			r.With(requireScope(domain.ScopeRead)).Get("/invoices/{id}", s.getInvoice)
			// Usage.
			r.With(requireScope(domain.ScopeWrite)).Post("/usage/ingest", s.ingestUsage)
			r.With(requireScope(domain.ScopeWrite)).Post("/usage/reverse", s.reverseUsage)
			r.With(requireScope(domain.ScopeRead)).Get("/usage/events", s.listUsageEvents)
			// Entitlements.
			r.With(requireScope(domain.ScopeWrite)).Put("/subscriptions/{id}/entitlement-overrides", s.upsertEntitlementOverride)
			r.With(requireScope(domain.ScopeRead)).Get("/subscriptions/{id}/entitlement-overrides", s.listEntitlementOverrides)
			r.With(requireScope(domain.ScopeWrite)).Delete("/subscriptions/{id}/entitlement-overrides/{key}", s.deleteEntitlementOverride)
			r.With(requireScope(domain.ScopeRead)).Get("/entitlements/{customerExternalId}", s.getEntitlementSnapshot)
			// Capabilities (provider views own).
			r.With(requireScope(domain.ScopeRead)).Get("/capabilities", s.listMyCapabilities)
			// PSP credentials (provider-managed, encrypted at rest).
			r.With(requireScope(domain.ScopeWrite)).Post("/psp-credentials", s.createPSPCredential)
			r.With(requireScope(domain.ScopeRead)).Get("/psp-credentials", s.listPSPCredentials)
			r.With(requireScope(domain.ScopeWrite)).Post("/psp-credentials/{id}/rotate", s.rotatePSPCredential)
			r.With(requireScope(domain.ScopeWrite)).Delete("/psp-credentials/{id}", s.revokePSPCredential)
			// Webhooks (signed delivery to provider endpoints).
			r.With(requireScope(domain.ScopeWrite)).Post("/webhooks", s.createWebhook)
			r.With(requireScope(domain.ScopeRead)).Get("/webhooks", s.listWebhooks)
			r.With(requireScope(domain.ScopeWrite)).Delete("/webhooks/{id}", s.deleteWebhook)
			r.With(requireScope(domain.ScopeRead)).Get("/webhook-deliveries", s.listWebhookDeliveries)
		})
	})
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message, requestID string) {
	body := map[string]any{"code": code, "message": message}
	if requestID != "" {
		body["request_id"] = requestID
	}
	writeJSON(w, status, map[string]any{"error": body})
}

// reqIDFromRequest extracts the request ID from the request context.
func reqIDFromRequest(r *http.Request) string {
	id, _ := r.Context().Value(requestIDKey{}).(string)
	return id
}

// serviceError maps service-layer errors onto the HTTP error shape.
// The request is used to extract the request_id for client-side correlation.
func (s *Server) serviceError(w http.ResponseWriter, r *http.Request, err error) {
	reqID, _ := r.Context().Value(requestIDKey{}).(string)
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), reqID)
	case errors.Is(err, service.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error(), reqID)
	case errors.Is(err, service.ErrUsageConflict):
		writeError(w, http.StatusConflict, "usage_conflict", err.Error(), reqID)
	case errors.Is(err, service.ErrUsageAlreadyInvoiced):
		writeError(w, http.StatusConflict, "usage_already_invoiced", err.Error(), reqID)
	case errors.Is(err, service.ErrValidation):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), reqID)
	case errors.Is(err, domain.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "invalid_transition", err.Error(), reqID)
	default:
		s.log.Error("internal error", "error", err, "request_id", reqID)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error", reqID)
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON", reqIDFromRequest(r))
		return false
	}
	return true
}
