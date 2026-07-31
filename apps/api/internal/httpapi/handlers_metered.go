package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type setMeteredPricingRuleRequest struct {
	MetricCode        string           `json:"metric_code"`
	PricingModel      string           `json:"pricing_model"`
	BasePriceCents    int64            `json:"base_price_cents"`
	TierConfig        []map[string]any `json:"tier_config"`
	MinimumSpendCents int64            `json:"minimum_spend_cents"`
	Enabled           bool             `json:"enabled"`
}

func (s *Server) setMeteredPricingRule(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req setMeteredPricingRuleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	rule, err := s.svc.SetMeteredPricingRule(r.Context(), tc, service.MeteredPricingRuleInput{
		MetricCode:        req.MetricCode,
		PricingModel:      req.PricingModel,
		BasePriceCents:    req.BasePriceCents,
		TierConfig:        req.TierConfig,
		MinimumSpendCents: req.MinimumSpendCents,
		Enabled:           req.Enabled,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) getMeteredPricingRule(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	metricCode := chi.URLParam(r, "metric")
	rule, err := s.svc.GetMeteredPricingRule(r.Context(), tc, metricCode)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) listMeteredPricingRules(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	rules, err := s.svc.ListMeteredPricingRules(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if rules == nil {
		rules = []storegen.MeteredPricingRule{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"metered_pricing_rules": rules})
}

func (s *Server) deleteMeteredPricingRule(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	metricCode := chi.URLParam(r, "metric")
	if err := s.svc.DeleteMeteredPricingRule(r.Context(), tc, metricCode); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type createBudgetAlertRequest struct {
	SubscriptionID string  `json:"subscription_id"`
	MetricCode     string  `json:"metric_code"`
	BudgetCents    int64   `json:"budget_cents"`
	ThresholdPct   float64 `json:"threshold_pct"`
}

func (s *Server) createBudgetAlert(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req createBudgetAlertRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	var subID *uuid.UUID
	if req.SubscriptionID != "" {
		id, err := uuid.Parse(req.SubscriptionID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_subscription_id", "subscription_id must be a uuid", reqIDFromRequest(r))
			return
		}
		subID = &id
	}
	alert, err := s.svc.CreateBudgetAlert(r.Context(), tc, service.BudgetAlertInput{
		SubscriptionID: subID,
		MetricCode:     req.MetricCode,
		BudgetCents:    req.BudgetCents,
		ThresholdPct:   req.ThresholdPct,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, alert)
}

func (s *Server) getBudgetAlert(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	alertID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "alert id must be a uuid", reqIDFromRequest(r))
		return
	}
	alert, err := s.svc.GetBudgetAlert(r.Context(), tc, alertID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, alert)
}

func (s *Server) listBudgetAlerts(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	alerts, err := s.svc.ListBudgetAlerts(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if alerts == nil {
		alerts = []storegen.BudgetAlert{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"budget_alerts": alerts})
}

func (s *Server) deleteBudgetAlert(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	alertID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "alert id must be a uuid", reqIDFromRequest(r))
		return
	}
	if err := s.svc.DeleteBudgetAlert(r.Context(), tc, alertID); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
