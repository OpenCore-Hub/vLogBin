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
	r.Use(DeprecationMiddleware)

	// Health-check endpoints (no auth — Kubernetes probes).
	r.Get("/health", s.health)
	r.Get("/ready", s.ready)
	// API version info (no auth — public compatibility policy).
	r.Get("/v1/api-version", s.getAPIVersion)

	// SCIM 2.0 endpoints (standard SCIM path, provider API key auth).
	r.Route("/scim/v2", func(r chi.Router) {
		r.Use(s.apiKeyAuth)
		r.With(requireScope(domain.ScopeSCIMManage)).Post("/Users", s.scimCreateUser)
		r.With(requireScope(domain.ScopeSCIMManage)).Get("/Users", s.scimListUsers)
		r.With(requireScope(domain.ScopeSCIMManage)).Get("/Users/{id}", s.scimGetUser)
		r.With(requireScope(domain.ScopeSCIMManage)).Put("/Users/{id}", s.scimUpdateUser)
		r.With(requireScope(domain.ScopeSCIMManage)).Patch("/Users/{id}", s.scimPatchUser)
		r.With(requireScope(domain.ScopeSCIMManage)).Delete("/Users/{id}", s.scimDeleteUser)
		// SCIM Groups (spec Section 4.2).
		r.With(requireScope(domain.ScopeSCIMManage)).Post("/Groups", s.scimCreateGroup)
		r.With(requireScope(domain.ScopeSCIMManage)).Get("/Groups", s.scimListGroups)
		r.With(requireScope(domain.ScopeSCIMManage)).Get("/Groups/{id}", s.scimGetGroup)
		r.With(requireScope(domain.ScopeSCIMManage)).Patch("/Groups/{id}", s.scimPatchGroup)
		r.With(requireScope(domain.ScopeSCIMManage)).Delete("/Groups/{id}", s.scimDeleteGroup)
	})

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
			// Cell management (operator-only).
			r.Post("/cells", s.createCell)
			r.Get("/cells/{id}", s.getCell)
			r.Patch("/cells/{id}", s.updateCellStatus)
			r.Post("/providers/{id}/cell", s.assignProviderCell)
			// Hot Standby + Failover (spec Section 14).
			r.Post("/failovers", s.initiateFailover)
			r.Get("/failovers", s.listFailovers)
			r.Get("/failovers/{id}", s.getFailover)
			r.Post("/failovers/{id}/fence", s.fenceFailover)
			r.Post("/failovers/{id}/switch", s.switchFailover)
			r.Post("/failovers/{id}/complete", s.completeFailover)
			r.Post("/failovers/{id}/abort", s.abortFailover)
			// Cell Migration (planned, spec Section 14, Phase 3).
			r.Post("/cell-migrations", s.createCellMigration)
			r.Get("/cell-migrations", s.listCellMigrations)
			r.Get("/cell-migrations/{id}", s.getCellMigration)
			r.Post("/cell-migrations/{id}/precheck", s.precheckMigration)
			r.Post("/cell-migrations/{id}/execute", s.executeMigration)
			r.Post("/cell-migrations/{id}/cancel", s.cancelMigration)
			// JIT Support Access (operator-managed).
			r.Post("/providers/{id}/support-sessions", s.requestSupportSession)
			r.Get("/providers/{id}/support-sessions", s.operatorListSupportSessions)
			r.Post("/support-sessions/{sessionId}/first-approve", s.firstApproveEmergency)
			r.Post("/support-sessions/{sessionId}/second-approve", s.secondApproveEmergency)
			r.Post("/support-sessions/{sessionId}/revoke", s.operatorRevokeSupportSession)
		})
		r.Group(func(r chi.Router) {
			r.Use(s.apiKeyAuth)
			r.Use(s.tenantGuard)
			r.Use(s.rateLimitMiddleware)
			r.With(requireScope(domain.ScopeRead)).Get("/whoami", s.whoami)
			r.With(requireScope(domain.ScopeRead)).Get("/credentials", s.listCredentials)
			r.With(requireScope(domain.ScopeCredentialsManage)).Post("/credentials", s.createCredential)
			r.With(requireScope(domain.ScopeCredentialsManage)).Post("/credentials/{id}/revoke", s.revokeCredential)
			// Delegated Administration (team member management).
			r.With(requireScope(domain.ScopeCredentialsManage)).Post("/team-members", s.inviteTeamMember)
			r.With(requireScope(domain.ScopeRead)).Get("/team-members", s.listTeamMembers)
			r.With(requireScope(domain.ScopeRead)).Get("/team-members/{id}", s.getTeamMember)
			r.With(requireScope(domain.ScopeCredentialsManage)).Patch("/team-members/{id}", s.updateTeamMemberRole)
			r.With(requireScope(domain.ScopeCredentialsManage)).Post("/team-members/{id}/suspend", s.suspendTeamMember)
			r.With(requireScope(domain.ScopeCredentialsManage)).Post("/team-members/{id}/reactivate", s.reactivateTeamMember)
			r.With(requireScope(domain.ScopeCredentialsManage)).Delete("/team-members/{id}", s.removeTeamMember)
			r.With(requireScope(domain.ScopeAuditRead)).Get("/audit-events", s.listAuditEvents)
			r.With(requireScope(domain.ScopeRead)).Get("/outbox-events", s.listOutboxEvents)
			// Enterprise Event Stream (cursor-based forward pagination).
			r.With(requireScope(domain.ScopeRead)).Get("/events", s.streamEvents)
			// Custom Auth Domains (DNS-verified branded auth).
			r.With(requireScope(domain.ScopeWrite)).Post("/custom-domains", s.registerCustomDomain)
			r.With(requireScope(domain.ScopeRead)).Get("/custom-domains", s.listCustomDomains)
			r.With(requireScope(domain.ScopeRead)).Get("/custom-domains/{id}", s.getCustomDomain)
			r.With(requireScope(domain.ScopeWrite)).Post("/custom-domains/{id}/verify", s.verifyCustomDomain)
			r.With(requireScope(domain.ScopeWrite)).Post("/custom-domains/{id}/revoke", s.revokeCustomDomain)
			r.With(requireScope(domain.ScopeWrite)).Delete("/custom-domains/{id}", s.deleteCustomDomain)
			// Notification configs (bring your own email/SMS).
			r.With(requireScope(domain.ScopeWrite)).Put("/notification-configs", s.setNotificationConfig)
			r.With(requireScope(domain.ScopeRead)).Get("/notification-configs", s.listNotificationConfigs)
			r.With(requireScope(domain.ScopeRead)).Get("/notification-configs/{channel}", s.getNotificationConfig)
			r.With(requireScope(domain.ScopeWrite)).Delete("/notification-configs/{channel}", s.deleteNotificationConfig)
			// SLA tiers (tiered SLA and reserved capacity).
			r.With(requireScope(domain.ScopeWrite)).Post("/sla-tiers", s.createSLATier)
			r.With(requireScope(domain.ScopeRead)).Get("/sla-tiers", s.listSLATiers)
			r.With(requireScope(domain.ScopeRead)).Get("/sla-tiers/{id}", s.getSLATier)
			r.With(requireScope(domain.ScopeWrite)).Patch("/sla-tiers/{id}", s.updateSLATier)
			r.With(requireScope(domain.ScopeWrite)).Delete("/sla-tiers/{id}", s.deleteSLATier)
			// Data export and deletion proof (Phase 3 offboarding).
			r.With(requireScope(domain.ScopeRead)).Post("/data-exports", s.requestDataExport)
			r.With(requireScope(domain.ScopeRead)).Get("/data-exports", s.listDataExports)
			r.With(requireScope(domain.ScopeRead)).Get("/data-exports/{id}", s.getDataExport)
			r.With(requireScope(domain.ScopeWrite)).Post("/data-deletion", s.requestDeletion)
			r.With(requireScope(domain.ScopeRead)).Get("/deletion-proofs", s.listDeletionProofs)
			r.With(requireScope(domain.ScopeRead)).Get("/deletion-proofs/{id}", s.getDeletionProof)
			// Cell info (provider can view their assigned cell).
			r.With(requireScope(domain.ScopeRead)).Get("/cell", s.getProviderCell)
			// Analytics (Phase 4: independent analytic plane, spec Section 18).
			r.With(requireScope(domain.ScopeRead)).Get("/analytics/dashboard", s.getProviderDashboard)
			r.With(requireScope(domain.ScopeRead)).Get("/analytics/revenue", s.getRevenueSummary)
			r.With(requireScope(domain.ScopeRead)).Get("/analytics/mau", s.getMAUSummary)
			r.With(requireScope(domain.ScopeRead)).Get("/analytics/conversion", s.getConversionSummary)
			r.With(requireScope(domain.ScopeRead)).Get("/analytics/churn", s.getChurnSummary)
			r.With(requireScope(domain.ScopeRead)).Get("/analytics/usage-breakdown", s.getUsageBreakdown)
			r.With(requireScope(domain.ScopeRead)).Get("/analytics/anomalies", s.getUsageAnomalies)
			// Metered Billing + FinOps (Phase 4, spec Section 18).
			r.With(requireScope(domain.ScopeWrite)).Put("/metered-pricing-rules", s.setMeteredPricingRule)
			r.With(requireScope(domain.ScopeRead)).Get("/metered-pricing-rules", s.listMeteredPricingRules)
			r.With(requireScope(domain.ScopeRead)).Get("/metered-pricing-rules/{metric}", s.getMeteredPricingRule)
			r.With(requireScope(domain.ScopeWrite)).Delete("/metered-pricing-rules/{metric}", s.deleteMeteredPricingRule)
			r.With(requireScope(domain.ScopeWrite)).Post("/budget-alerts", s.createBudgetAlert)
			r.With(requireScope(domain.ScopeRead)).Get("/budget-alerts", s.listBudgetAlerts)
			r.With(requireScope(domain.ScopeRead)).Get("/budget-alerts/{id}", s.getBudgetAlert)
			r.With(requireScope(domain.ScopeWrite)).Delete("/budget-alerts/{id}", s.deleteBudgetAlert)
			// Migration Plane (dry-run, resumable import, cutover lock, rollback).
			r.With(requireScope(domain.ScopeWrite)).Post("/migrations", s.createMigrationJob)
			r.With(requireScope(domain.ScopeRead)).Get("/migrations", s.listMigrationJobs)
			r.With(requireScope(domain.ScopeRead)).Get("/migrations/{id}", s.getMigrationJob)
			r.With(requireScope(domain.ScopeWrite)).Post("/migrations/{id}/records", s.addMigrationRecords)
			r.With(requireScope(domain.ScopeRead)).Get("/migrations/{id}/records", s.listMigrationRecords)
			r.With(requireScope(domain.ScopeRead)).Get("/migrations/{id}/invalid-records", s.listInvalidRecords)
			r.With(requireScope(domain.ScopeWrite)).Post("/migrations/{id}/validate", s.validateMigrationJob)
			r.With(requireScope(domain.ScopeWrite)).Post("/migrations/{id}/start", s.startMigration)
			r.With(requireScope(domain.ScopeWrite)).Post("/migrations/{id}/complete", s.completeMigration)
			r.With(requireScope(domain.ScopeWrite)).Post("/migrations/{id}/rollback", s.rollbackMigration)
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
			// Hard Quota (reserve/commit/release persistent ledger).
			r.With(requireScope(domain.ScopeWrite)).Put("/subscriptions/{id}/quota-limits/{key}", s.setQuotaLimit)
			r.With(requireScope(domain.ScopeRead)).Get("/subscriptions/{id}/quota-limits/{key}", s.getQuotaLimit)
			r.With(requireScope(domain.ScopeRead)).Get("/subscriptions/{id}/quota-limits", s.listQuotaLimits)
			r.With(requireScope(domain.ScopeWrite)).Delete("/subscriptions/{id}/quota-limits/{key}", s.deleteQuotaLimit)
			r.With(requireScope(domain.ScopeWrite)).Post("/subscriptions/{id}/quota/reserve", s.reserveQuota)
			r.With(requireScope(domain.ScopeWrite)).Post("/subscriptions/{id}/quota/commit", s.commitQuota)
			r.With(requireScope(domain.ScopeWrite)).Post("/subscriptions/{id}/quota/release", s.releaseQuota)
			r.With(requireScope(domain.ScopeRead)).Get("/subscriptions/{id}/quota/usage", s.getQuotaUsage)
			r.With(requireScope(domain.ScopeRead)).Get("/subscriptions/{id}/quota/reservations", s.listQuotaReservations)
			// Capabilities (provider views own).
			r.With(requireScope(domain.ScopeRead)).Get("/capabilities", s.listMyCapabilities)
			// PSP credentials (provider-managed, encrypted at rest).
			r.With(requireScope(domain.ScopeWrite)).Post("/psp-credentials", s.createPSPCredential)
			r.With(requireScope(domain.ScopeRead)).Get("/psp-credentials", s.listPSPCredentials)
			r.With(requireScope(domain.ScopeWrite)).Post("/psp-credentials/{id}/rotate", s.rotatePSPCredential)
			r.With(requireScope(domain.ScopeWrite)).Delete("/psp-credentials/{id}", s.revokePSPCredential)
			// Hosted Auth (ZITADEL OIDC project management).
			r.With(requireScope(domain.ScopeWrite)).Post("/auth/zitadel/setup", s.setupHostedAuth)
			r.With(requireScope(domain.ScopeRead)).Get("/auth/zitadel/config", s.getHostedAuthConfig)
			r.With(requireScope(domain.ScopeWrite)).Delete("/auth/zitadel", s.disableHostedAuth)
			// Webhooks (signed delivery to provider endpoints).
			r.With(requireScope(domain.ScopeWrite)).Post("/webhooks", s.createWebhook)
			r.With(requireScope(domain.ScopeRead)).Get("/webhooks", s.listWebhooks)
			r.With(requireScope(domain.ScopeWrite)).Delete("/webhooks/{id}", s.deleteWebhook)
			r.With(requireScope(domain.ScopeRead)).Get("/webhook-deliveries", s.listWebhookDeliveries)
			// JIT Support Access (provider can approve/deny/revoke and view).
			r.With(requireScope(domain.ScopeSupportApprove)).Post("/support-sessions/{id}/approve", s.providerApproveSupportSession)
			r.With(requireScope(domain.ScopeSupportApprove)).Post("/support-sessions/{id}/deny", s.providerDenySupportSession)
			r.With(requireScope(domain.ScopeSupportApprove)).Post("/support-sessions/{id}/revoke", s.providerRevokeSupportSession)
			r.With(requireScope(domain.ScopeRead)).Get("/support-sessions", s.providerListSupportSessions)
			r.With(requireScope(domain.ScopeRead)).Get("/support-sessions/active", s.providerListActiveSupportSessions)
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
	case errors.Is(err, service.ErrQuotaExceeded):
		writeError(w, http.StatusUnprocessableEntity, "quota_exceeded", err.Error(), reqID)
	case errors.Is(err, service.ErrCutoverLocked):
		writeError(w, http.StatusConflict, "cutover_locked", err.Error(), reqID)
	case errors.Is(err, service.ErrCellDraining):
		writeError(w, http.StatusConflict, "cell_draining", err.Error(), reqID)
	case errors.Is(err, service.ErrDomainTaken):
		writeError(w, http.StatusConflict, "domain_taken", err.Error(), reqID)
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
