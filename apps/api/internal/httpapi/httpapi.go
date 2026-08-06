// Package httpapi exposes the platform v1 HTTP API: operator routes
// (operator token) and provider routes (API-key credentials) with a
// consistent error shape {"error":{"code","message"}}.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/config"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/metrics"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/portal"
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
	// corsOrigins (atomic.Value of []string), rateLimits (atomic.Value of
	// config.RateLimitConfig) and slowRequestThreshold (atomic.Int64 of
	// nanoseconds) are read on every request and swapped by the config
	// reloader, so they must stay atomic: a runtime update races with
	// concurrent request handling.
	corsOrigins          atomic.Value // []string
	limiter              ratelimit.Backend
	rateLimits           atomic.Value // config.RateLimitConfig
	oidcVerifier         *zitadel.Verifier
	metrics              *metrics.Metrics
	requestTimeout       time.Duration
	readyTimeout         time.Duration
	startupComplete      atomic.Bool
	slowRequestThreshold atomic.Int64 // nanoseconds
	idempotencyTTL       time.Duration
	portalIssuer         *portal.Issuer
}

func NewServer(st *store.Store, svc *service.Service, operatorToken string, log *slog.Logger) *Server {
	s := &Server{
		store:          st,
		svc:            svc,
		operatorToken:  operatorToken,
		log:            log,
		limiter:        ratelimit.New(),
		metrics:        metrics.New(),
		requestTimeout: 30 * time.Second,
		readyTimeout:   2 * time.Second,
	}
	s.SetCORSOrigins([]string{"*"})
	s.SetRateLimits(config.RateLimitConfig{
		Provider: 1000, Environment: 500, Credential: 200, Endpoint: 60,
		Window: time.Minute,
	})
	s.SetIdempotencyTTL(0) // 24h default; main may override via IDEMPOTENCY_TTL
	return s
}

// SetRequestTimeout overrides the per-request handler timeout. A non-positive
// value disables the timeout. Must be called before Handler is built.
func (s *Server) SetRequestTimeout(d time.Duration) {
	s.requestTimeout = d
}

// SetReadyTimeout overrides the /ready database ping timeout. A non-positive
// value disables the timeout. Must be called before Handler is built.
func (s *Server) SetReadyTimeout(d time.Duration) {
	s.readyTimeout = d
}

// SetStartupComplete marks the server as fully initialized. /startup flips
// from 503 to 200 once this is called; main calls it only after migrations,
// the connection pool, billing, ZITADEL and all workers are ready.
func (s *Server) SetStartupComplete() {
	s.startupComplete.Store(true)
}

// SetSlowRequestThreshold escalates request logs to Warn with slow=true once
// a request exceeds d. A non-positive value disables escalation. May be
// called at any time; the value is read on every request (atomic).
func (s *Server) SetSlowRequestThreshold(d time.Duration) {
	s.slowRequestThreshold.Store(int64(d))
}

// SlowRequestThreshold returns the current slow-request escalation threshold.
func (s *Server) SlowRequestThreshold() time.Duration {
	return time.Duration(s.slowRequestThreshold.Load())
}

// Metrics exposes the Prometheus metric families so background reporters
// (backlog gauges, sweeper counters) can update them.
func (s *Server) Metrics() *metrics.Metrics {
	return s.metrics
}

// SetRateLimits configures the 4-level rate limit settings. May be called at
// any time; the config is read on every request (atomic).
func (s *Server) SetRateLimits(rl config.RateLimitConfig) {
	s.rateLimits.Store(rl)
}

// SetRateLimiter swaps the rate-limiter backend (in-memory or Redis-backed).
// Must be called before Router() is built: the middleware captures the
// limiter reference when the route chain is constructed, so swapping later
// has no effect. Defaults to the in-memory limiter.
func (s *Server) SetRateLimiter(l ratelimit.Backend) {
	s.limiter = l
}

// RateLimits returns the current rate limit configuration.
func (s *Server) RateLimits() config.RateLimitConfig {
	v := s.rateLimits.Load()
	if v == nil {
		return config.RateLimitConfig{Window: time.Minute}
	}
	return v.(config.RateLimitConfig)
}

// SetOIDCVerifier configures ZITADEL OIDC token verification for
// operator auth. When set, operator tokens are verified as JWTs.
func (s *Server) SetOIDCVerifier(v *zitadel.Verifier) {
	s.oidcVerifier = v
}

// SetPortalIssuer enables customer portal token endpoints. Must be called
// before Router is built; nil disables the portal.
func (s *Server) SetPortalIssuer(issuer *portal.Issuer) {
	s.portalIssuer = issuer
}

// SetCORSOrigins configures the allowed CORS origins. May be called at any
// time; the middleware reads the current value on every request (atomic).
func (s *Server) SetCORSOrigins(origins []string) {
	if len(origins) > 0 {
		s.corsOrigins.Store(origins)
	}
}

// CORSOrigins returns the current allowed CORS origins.
func (s *Server) CORSOrigins() []string {
	v := s.corsOrigins.Load()
	if v == nil {
		return nil
	}
	return v.([]string)
}

func (s *Server) Router() chi.Router {
	r := chi.NewRouter()
	// OpenTelemetry tracing outermost so it also captures early failures and
	// panics recovered below; no-op (negligible) when tracing is disabled.
	r.Use(s.tracingMiddleware)
	r.Use(s.recoverMiddleware)
	r.Use(securityHeadersMiddleware)
	r.Use(s.corsMiddleware)
	r.Use(bodyLimitMiddleware)
	r.Use(requestIDMiddleware)
	r.Use(s.requestLogMiddleware)
	r.Use(s.metricsMiddleware)
	// Per-IP safety net runs before authentication so a single source
	// cannot exhaust the authenticated buckets by rotating credentials,
	// and unauthenticated endpoints stay protected from naive DoS. Its
	// 429 responses still flow through metricsMiddleware above.
	r.Use(s.ipRateLimitMiddleware)
	r.Use(s.requestTimeoutMiddleware)
	r.Use(DeprecationMiddleware)

	// Prometheus metrics (no auth — scrape target, see docs/DEPLOYMENT.md).
	r.Get("/metrics", s.metrics.Handler().ServeHTTP)

	// Health-check endpoints (no auth — Kubernetes probes).
	r.Get("/health", s.health)
	r.Get("/ready", s.ready)
	r.Get("/startup", s.startup)
	// API version info (no auth — public compatibility policy).
	r.Get("/v1/api-version", s.getAPIVersion)

	// SCIM 2.0 endpoints (standard SCIM path, provider API key auth).
	r.Route("/scim/v2", func(r chi.Router) {
		r.Use(s.apiKeyAuth)
		r.Use(s.lifecycleWriteGuard)
		r.Use(s.idempotencyMiddleware)
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
			r.Use(s.idempotencyMiddleware)
			r.Post("/providers", s.createProvider)
			r.Get("/providers", s.listProviders)
			r.Get("/providers/{id}", s.getProvider)
			r.Post("/providers/{id}/lifecycle", s.transitionLifecycle)
			r.Post("/providers/{id}/activate", s.activateProvider)
			r.Get("/providers/{id}/audit", s.listProviderAuditEvents)
			r.Get("/providers/{id}/audit/stats", s.providerAuditEventStats)
			r.Get("/providers/{id}/audit/export", s.providerAuditEventExport)
			r.Get("/providers/{id}/credentials", s.listProviderCredentials)
			// Developers control plane (§8 M3): API keys, webhooks, event stream.
			r.Post("/providers/{id}/credentials", s.operatorCreateCredential)
			r.Post("/providers/{id}/credentials/{credentialId}/revoke", s.revokeProviderCredential)
			r.Post("/providers/{id}/credentials/{credentialId}/rotate", s.operatorRotateCredential)
			r.Get("/regions", s.listRegions)
			r.Get("/cells", s.listCells)
			// Operator billing views (cross-environment, read-only).
			r.Get("/overview-stats", s.operatorOverviewStats)
			r.Get("/providers/{id}/catalog/versions", s.operatorListCatalogVersions)
			r.Get("/providers/{id}/catalog/versions/{versionId}", s.operatorGetCatalogVersion)
			// Console Plans control plane (§8 M2), environment-scoped via ?env=.
			r.Get("/providers/{id}/catalog/plans", s.operatorListCatalogPlans)
			r.Get("/providers/{id}/catalog/plans/{code}", s.operatorGetCatalogPlan)
			r.Post("/providers/{id}/catalog/plans", s.operatorCreateCatalogPlan)
			r.Put("/providers/{id}/catalog/plans/{code}", s.operatorUpdateCatalogPlan)
			r.Delete("/providers/{id}/catalog/plans/{code}", s.operatorDeleteCatalogPlan)
			// Console Policies control plane (plan-level entitlement grants).
			r.Get("/providers/{id}/catalog/plans/{code}/entitlements", s.operatorListPlanEntitlements)
			r.Put("/providers/{id}/catalog/plans/{code}/entitlements/{key}", s.operatorSetPlanEntitlement)
			r.Delete("/providers/{id}/catalog/plans/{code}/entitlements/{key}", s.operatorDeletePlanEntitlement)
			// Console Analytics control plane (M4).
			r.Get("/providers/{id}/analytics/dashboard", s.operatorAnalyticsDashboard)
			r.Get("/providers/{id}/subscriptions", s.operatorListSubscriptions)
			r.Get("/providers/{id}/customers", s.operatorListCustomers)
			// Console Customers control plane (§8 M2), environment-scoped via ?env=.
			r.Post("/providers/{id}/customers", s.operatorCreateCustomer)
			r.Get("/providers/{id}/customers/{externalId}", s.operatorGetCustomer)
			// Customer portal invite token (§8 M3).
			r.Post("/providers/{id}/customers/{externalId}/portal-token", s.operatorIssuePortalToken)
			r.Get("/providers/{id}/usage-events", s.operatorListUsageEvents)
			r.Get("/providers/{id}/invoices", s.operatorListInvoices)
			// Console Invoices control plane (§8 M2), environment-scoped via ?env=.
			r.Get("/providers/{id}/invoices/{invoiceId}", s.operatorGetInvoice)
			// Console Events stream (§8 M3), environment-scoped via ?env=.
			r.Get("/providers/{id}/events", s.operatorStreamEvents)
			// Console Settings (§8 M3): custom domains + notification configs.
			r.Get("/providers/{id}/custom-domains", s.operatorListCustomDomains)
			r.Post("/providers/{id}/custom-domains", s.operatorRegisterCustomDomain)
			r.Post("/providers/{id}/custom-domains/{domainId}/verify", s.operatorVerifyCustomDomain)
			r.Post("/providers/{id}/custom-domains/{domainId}/revoke", s.operatorRevokeCustomDomain)
			r.Delete("/providers/{id}/custom-domains/{domainId}", s.operatorDeleteCustomDomain)
			r.Get("/providers/{id}/notification-configs", s.operatorListNotificationConfigs)
			r.Put("/providers/{id}/notification-configs", s.operatorSetNotificationConfig)
			r.Delete("/providers/{id}/notification-configs/{channel}", s.operatorDeleteNotificationConfig)
			// Provider capability grants (operator-managed).
			r.Get("/providers/{id}/capabilities", s.operatorListCapabilities)
			r.Post("/providers/{id}/capabilities/{capability}/grant", s.operatorGrantCapability)
			r.Post("/providers/{id}/capabilities/{capability}/revoke", s.operatorRevokeCapability)
			// Provider risk review (go-live gate, architecture §15).
			r.Post("/providers/{id}/risk-review", s.operatorSubmitRiskReview)
			r.Get("/providers/{id}/risk-reviews", s.operatorListRiskReviews)
			// OIDC Application management (Console control plane, §8 M2).
			r.Get("/providers/{id}/auth/zitadel/apps", s.operatorListHostedAuthConfigs)
			r.Post("/providers/{id}/auth/zitadel/setup", s.operatorSetupHostedAuth)
			r.Post("/providers/{id}/auth/zitadel/rotate-secret", s.operatorRotateHostedAuthSecret)
			r.Put("/providers/{id}/auth/zitadel/redirect-uris", s.operatorUpdateHostedAuthRedirectURIs)
			r.Delete("/providers/{id}/auth/zitadel", s.operatorDisableHostedAuth)
			// Reconciliation results (operator monitoring).
			r.Get("/reconciliation-results", s.operatorListReconciliationResults)
			// Tamper-evident audit chain (operator-only, migration 0031).
			r.Get("/audit/chain", s.auditChainState)
			r.Get("/audit/chain/verify", s.auditChainVerify)
			r.Post("/audit/chain/anchor", s.auditChainAnchor)
			// Webhook monitoring (operator view, cross-environment).
			r.Get("/providers/{id}/webhooks", s.operatorListWebhooks)
			r.Post("/providers/{id}/webhooks", s.operatorCreateWebhook)
			r.Delete("/providers/{id}/webhooks/{webhookId}", s.operatorDeleteWebhook)
			r.Get("/providers/{id}/webhook-deliveries", s.operatorListWebhookDeliveries)
			r.Post("/providers/{id}/webhook-deliveries/{deliveryId}/replay", s.operatorReplayWebhookDelivery)
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
		// Control plane: platform-user self-service (design baseline §3.1 R11).
		r.Route("/signup", func(r chi.Router) {
			r.Use(s.operatorAuth)
			r.Use(s.idempotencyMiddleware)
			r.Post("/", s.signup)
		})
		r.Route("/me", func(r chi.Router) {
			r.Use(s.operatorAuth)
			r.Get("/workspaces", s.meWorkspaces)
			r.Get("/workspaces/{id}", s.getWorkspace)
			r.Patch("/workspaces/{id}", s.updateWorkspace)
			r.Get("/workspaces/{id}/members", s.listWorkspaceMembers)
			r.Post("/workspaces/{id}/members", s.inviteWorkspaceMember)
			r.Patch("/workspaces/{id}/members/{userSub}", s.updateWorkspaceMemberRole)
			r.Delete("/workspaces/{id}/members/{userSub}", s.removeWorkspaceMember)
		})
		// Customer Portal (§8 M3): independent customer session, data domain
		// isolated by portal token claims.
		r.Route("/portal", func(r chi.Router) {
			r.Post("/sessions", s.portalSession)
			r.Group(func(r chi.Router) {
				r.Use(s.portalAuthMiddleware)
				r.Get("/dashboard", s.portalDashboard)
			})
		})
		r.Group(func(r chi.Router) {
			r.Use(s.apiKeyAuth)
			r.Use(s.tenantGuard)
			r.Use(s.environmentHeaderMiddleware)
			r.Use(s.rateLimitMiddleware)
			r.Use(s.idempotencyMiddleware)
			r.Use(s.lifecycleWriteGuard)
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
			r.With(requireScope(domain.ScopeAuditRead)).Get("/audit-events/stats", s.auditEventStats)
			r.With(requireScope(domain.ScopeAuditRead)).Get("/audit-events/export", s.auditEventExport)
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
			// Catalog plans (plan-level CRUD on the current draft version).
			r.With(requireScope(domain.ScopeRead)).Get("/catalog/plans", s.listCatalogPlans)
			r.With(requireScope(domain.ScopeWrite)).Post("/catalog/plans", s.createCatalogPlan)
			r.With(requireScope(domain.ScopeWrite)).Put("/catalog/plans/{code}", s.updateCatalogPlan)
			r.With(requireScope(domain.ScopeWrite)).Delete("/catalog/plans/{code}", s.deleteCatalogPlan)
			// Catalog policies (plan-level entitlement grants on the current draft).
			r.With(requireScope(domain.ScopeRead)).Get("/catalog/plans/{code}/entitlements", s.listPlanEntitlements)
			r.With(requireScope(domain.ScopeWrite)).Put("/catalog/plans/{code}/entitlements/{key}", s.setPlanEntitlement)
			r.With(requireScope(domain.ScopeWrite)).Delete("/catalog/plans/{code}/entitlements/{key}", s.deletePlanEntitlement)
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
			r.With(requireScope(domain.ScopeRead)).Get("/auth/zitadel/apps", s.listHostedAuthConfigs)
			r.With(requireScope(domain.ScopeWrite)).Post("/auth/zitadel/rotate-secret", s.rotateHostedAuthSecret)
			r.With(requireScope(domain.ScopeWrite)).Put("/auth/zitadel/redirect-uris", s.updateHostedAuthRedirectURIs)
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

// writeAuditQueryError maps audit query parameter errors to 400 responses;
// anything else falls through as an internal error (should not happen).
func writeAuditQueryError(w http.ResponseWriter, r *http.Request, err error) {
	var qe auditQueryError
	if errors.As(err, &qe) {
		writeError(w, http.StatusBadRequest, qe.code, qe.msg, reqIDFromRequest(r))
		return
	}
	writeError(w, http.StatusInternalServerError, "internal", "internal error", reqIDFromRequest(r))
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
	case errors.Is(err, service.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", err.Error(), reqID)
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
	case errors.Is(err, service.ErrProviderNotWritable):
		writeError(w, http.StatusConflict, "provider_not_writable", err.Error(), reqID)
	case errors.Is(err, service.ErrLifecycleConflict):
		writeError(w, http.StatusConflict, "lifecycle_conflict", err.Error(), reqID)
	case errors.Is(err, service.ErrLiveReviewRequired):
		writeError(w, http.StatusConflict, "live_review_required", err.Error(), reqID)
	case errors.Is(err, service.ErrRiskReviewConflict):
		writeError(w, http.StatusConflict, "risk_review_conflict", err.Error(), reqID)
	case errors.Is(err, service.ErrWebhookReplayConflict):
		writeError(w, http.StatusConflict, "replay_invalid_state", err.Error(), reqID)
	case errors.Is(err, service.ErrDomainTaken):
		writeError(w, http.StatusConflict, "domain_taken", err.Error(), reqID)
	case errors.Is(err, service.ErrValidation):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), reqID)
	case errors.Is(err, domain.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "invalid_transition", err.Error(), reqID)
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "upstream_timeout", "request exceeded timeout", reqID)
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
