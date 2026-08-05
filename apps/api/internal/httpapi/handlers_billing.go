package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ---- catalog ----

type createCatalogVersionRequest struct {
	FromVersionID *uuid.UUID `json:"from_version_id,omitempty"`
}

func (s *Server) createCatalogVersion(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req createCatalogVersionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	version, err := s.svc.CreateCatalogVersion(r.Context(), tc, req.FromVersionID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"version": version})
}

func (s *Server) listCatalogVersions(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	versions, err := s.svc.ListCatalogVersions(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

func (s *Server) getCatalogVersion(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	id, err := parseUUIDParam(w, r, "versionId")
	if err != nil {
		return
	}
	detail, err := s.svc.GetCatalogVersion(r.Context(), tc, id)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) replaceCatalogContent(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	id, err := parseUUIDParam(w, r, "versionId")
	if err != nil {
		return
	}
	var content domain.CatalogContent
	if !decodeJSON(w, r, &content) {
		return
	}
	detail, err := s.svc.ReplaceCatalogContent(r.Context(), tc, id, content)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) validateCatalogVersion(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	id, err := parseUUIDParam(w, r, "versionId")
	if err != nil {
		return
	}
	version, err := s.svc.ValidateCatalogVersion(r.Context(), tc, id)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": version})
}

func (s *Server) publishCatalogVersion(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	id, err := parseUUIDParam(w, r, "versionId")
	if err != nil {
		return
	}
	version, err := s.svc.PublishCatalogVersion(r.Context(), tc, id)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": version})
}

func (s *Server) retireCatalogVersion(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	id, err := parseUUIDParam(w, r, "versionId")
	if err != nil {
		return
	}
	version, err := s.svc.RetireCatalogVersion(r.Context(), tc, id)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": version})
}

// ---- catalog plans ----
//
// Plan-level CRUD operates on the current draft version (auto-created by
// cloning the latest published version when absent). Read operations fall
// back to the latest published version when no draft exists.

func (s *Server) listCatalogPlans(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	plans, err := s.svc.ListPlans(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": plans})
}

func (s *Server) createCatalogPlan(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var input domain.PlanInput
	if !decodeJSON(w, r, &input) {
		return
	}
	detail, err := s.svc.CreatePlan(r.Context(), tc, input)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

func (s *Server) updateCatalogPlan(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	code := chi.URLParam(r, "code")
	var input domain.PlanInput
	if !decodeJSON(w, r, &input) {
		return
	}
	detail, err := s.svc.UpdatePlan(r.Context(), tc, code, input)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) deleteCatalogPlan(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	code := chi.URLParam(r, "code")
	if err := s.svc.DeletePlan(r.Context(), tc, code); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- catalog policies ----
//
// Policies are plan-level entitlement grants managed independently of plan
// pricing, operating on the current draft version like plan CRUD.

func (s *Server) listPlanEntitlements(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	code := chi.URLParam(r, "code")
	grants, err := s.svc.ListPlanEntitlements(r.Context(), tc, code)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entitlements": grants})
}

func (s *Server) setPlanEntitlement(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	code := chi.URLParam(r, "code")
	key := chi.URLParam(r, "key")
	var input domain.EntitlementInput
	if !decodeJSON(w, r, &input) {
		return
	}
	grant, err := s.svc.SetPlanEntitlement(r.Context(), tc, code, key, input)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entitlement": grant})
}

func (s *Server) deletePlanEntitlement(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	code := chi.URLParam(r, "code")
	key := chi.URLParam(r, "key")
	if err := s.svc.DeletePlanEntitlement(r.Context(), tc, code, key); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- customers ----

type createCustomerRequest struct {
	ExternalID  string `json:"external_id"`
	AccountType string `json:"account_type"`
	DisplayName string `json:"display_name"`
}

func (s *Server) createCustomer(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req createCustomerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	customer, err := s.svc.CreateCustomer(r.Context(), tc, req.ExternalID, req.AccountType, req.DisplayName)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"customer": customer})
}

func (s *Server) listCustomers(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	customers, err := s.svc.ListCustomers(r.Context(), tc, queryLimit(r, 100))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"customers": customers})
}

// ---- subscriptions ----

type createSubscriptionRequest struct {
	ExternalID         string    `json:"external_id"`
	CustomerExternalID string    `json:"customer_external_id"`
	CatalogVersionID   uuid.UUID `json:"catalog_version_id"`
	PlanCode           string    `json:"plan_code"`
}

func (s *Server) createSubscription(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req createSubscriptionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	sub, err := s.svc.CreateSubscription(r.Context(), tc, req.ExternalID, req.CustomerExternalID, req.CatalogVersionID, req.PlanCode)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"subscription": sub})
}

func (s *Server) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	subs, err := s.svc.ListSubscriptions(r.Context(), tc, queryLimit(r, 100))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscriptions": subs})
}

func (s *Server) terminateSubscription(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	id, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	sub, err := s.svc.TerminateSubscription(r.Context(), tc, id)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscription": sub})
}

// ---- usage ----

type ingestUsageRequest struct {
	TransactionID      string          `json:"transaction_id"`
	MetricCode         string          `json:"metric_code"`
	CustomerExternalID string          `json:"customer_external_id"`
	Timestamp          string          `json:"timestamp"`
	Properties         json.RawMessage `json:"properties,omitempty"`
}

func (s *Server) ingestUsage(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req ingestUsageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ts, err := time.Parse(time.RFC3339Nano, req.Timestamp)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_timestamp", "timestamp must be an RFC3339 string", reqIDFromRequest(r))
		return
	}
	result, err := s.svc.IngestUsage(r.Context(), tc, service.UsageIngestInput{
		TransactionID:      req.TransactionID,
		MetricCode:         req.MetricCode,
		CustomerExternalID: req.CustomerExternalID,
		Timestamp:          ts,
		Properties:         req.Properties,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	status := http.StatusCreated
	if result.Status == service.UsageStatusDuplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"status": result.Status, "event": result.Event})
}

type reverseUsageRequest struct {
	OriginalTransactionID string `json:"original_transaction_id"`
	ReversalTransactionID string `json:"reversal_transaction_id,omitempty"`
	Reason                string `json:"reason,omitempty"`
}

func (s *Server) reverseUsage(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req reverseUsageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	event, err := s.svc.ReverseUsage(r.Context(), tc, req.OriginalTransactionID, req.ReversalTransactionID, req.Reason)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"event": event})
}

func (s *Server) listUsageEvents(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	events, err := s.svc.ListUsageEvents(r.Context(), tc, queryLimit(r, 100))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

// ---- entitlements ----

type upsertEntitlementOverrideRequest struct {
	Key       string          `json:"key"`
	ValueType string          `json:"value_type"`
	Value     json.RawMessage `json:"value"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
	Reason    string          `json:"reason,omitempty"`
}

func (s *Server) upsertEntitlementOverride(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	subID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	var req upsertEntitlementOverrideRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	override, err := s.svc.UpsertEntitlementOverride(r.Context(), tc, service.UpsertEntitlementOverrideInput{
		SubscriptionID: subID,
		Key:            req.Key,
		ValueType:      req.ValueType,
		Value:          req.Value,
		ExpiresAt:      req.ExpiresAt,
		Reason:         req.Reason,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"override": override})
}

func (s *Server) listEntitlementOverrides(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	subID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	overrides, err := s.svc.ListEntitlementOverrides(r.Context(), tc, subID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"overrides": overrides})
}

func (s *Server) deleteEntitlementOverride(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	subID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "invalid_key", "key is required", reqIDFromRequest(r))
		return
	}
	if err := s.svc.DeleteEntitlementOverride(r.Context(), tc, subID, key); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getEntitlementSnapshot(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	customerExternalID := chi.URLParam(r, "customerExternalId")
	if customerExternalID == "" {
		writeError(w, http.StatusBadRequest, "invalid_customer", "customer external id is required", reqIDFromRequest(r))
		return
	}
	result, err := s.svc.GetEntitlementSnapshot(r.Context(), tc, customerExternalID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshot": result})
}

// ---- invoices ----

func (s *Server) syncInvoices(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	n, err := s.svc.SyncInvoices(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"synced": n})
}

func (s *Server) listInvoices(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	invoices, err := s.svc.ListInvoices(r.Context(), tc, queryLimit(r, 100))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invoices": invoices})
}

func (s *Server) getInvoice(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	id, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	detail, err := s.svc.GetInvoice(r.Context(), tc, id)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// ---- operator invoice views ----

func (s *Server) operatorListInvoices(w http.ResponseWriter, r *http.Request) {
	providerID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	invoices, err := s.svc.ListInvoicesByProvider(r.Context(), providerID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invoices": invoices})
}

// ---- operator billing views ----

func (s *Server) operatorListCatalogVersions(w http.ResponseWriter, r *http.Request) {
	providerID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	versions, err := s.svc.ListCatalogVersionsByProvider(r.Context(), providerID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
}

func (s *Server) operatorGetCatalogVersion(w http.ResponseWriter, r *http.Request) {
	versionID, err := parseUUIDParam(w, r, "versionId")
	if err != nil {
		return
	}
	detail, err := s.svc.GetCatalogVersionOperator(r.Context(), versionID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) operatorListSubscriptions(w http.ResponseWriter, r *http.Request) {
	providerID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	subs, err := s.svc.ListSubscriptionsByProvider(r.Context(), providerID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscriptions": subs})
}

func (s *Server) operatorListCustomers(w http.ResponseWriter, r *http.Request) {
	// Console customers page passes ?env= to read a single environment;
	// without it the operator billing view keeps returning cross-environment.
	if r.URL.Query().Get("env") != "" {
		providerID, env, ok := s.providerEnvFromRequest(w, r)
		if !ok {
			return
		}
		customers, err := s.svc.ListCustomers(r.Context(), service.OperatorAuthContext(providerID, env), queryLimit(r, 100))
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"customers": customers})
		return
	}
	providerID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	customers, err := s.svc.ListCustomersByProvider(r.Context(), providerID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"customers": customers})
}

func (s *Server) operatorListUsageEvents(w http.ResponseWriter, r *http.Request) {
	providerID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	events, err := s.svc.ListUsageEventsByProvider(r.Context(), providerID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"usage_events": events})
}

// operatorOverviewStats returns cross-provider aggregates for the console
// overview in a single request (R29: eliminates the web-side N+1 fan-out).
func (s *Server) operatorOverviewStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.svc.OverviewStats(r.Context())
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// parseUUIDParam reads a chi URL param and parses it as a UUID, writing a
// 400 error on failure. Returns the parsed UUID or an error.
func parseUUIDParam(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", param+" must be a uuid", reqIDFromRequest(r))
		return uuid.Nil, err
	}
	return id, nil
}
