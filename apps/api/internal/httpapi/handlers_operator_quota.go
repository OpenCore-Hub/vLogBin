package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/go-chi/chi/v5"
)

// operatorQuotaLimits — GET /v1/operator/providers/{id}/subscriptions/{subscriptionId}/quota?env=
func (s *Server) operatorQuotaLimits(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	subscriptionID, err := parseUUIDParam(w, r, "subscriptionId")
	if err != nil {
		return
	}
	limits, err := s.svc.ListQuotaLimitsWithUsage(r.Context(), service.OperatorAuthContext(providerID, env), subscriptionID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"quota_limits": limits})
}

// operatorListQuotaReservations — GET /v1/operator/providers/{id}/subscriptions/{subscriptionId}/quota/reservations?env=
func (s *Server) operatorListQuotaReservations(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	subscriptionID, err := parseUUIDParam(w, r, "subscriptionId")
	if err != nil {
		return
	}
	reservations, err := s.svc.ListQuotaReservations(r.Context(), service.OperatorAuthContext(providerID, env), subscriptionID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"quota_reservations": reservations})
}

// operatorSetQuotaLimit — PUT /v1/operator/providers/{id}/subscriptions/{subscriptionId}/quota-limits/{key}?env=
func (s *Server) operatorSetQuotaLimit(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	subscriptionID, err := parseUUIDParam(w, r, "subscriptionId")
	if err != nil {
		return
	}
	var req setQuotaLimitRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	key := chi.URLParam(r, "key")
	if key == "" {
		key = req.QuotaKey
	}
	limit, err := s.svc.SetQuotaLimit(r.Context(), service.OperatorAuthContext(providerID, env), service.SetQuotaLimitInput{
		SubscriptionID: subscriptionID,
		QuotaKey:       key,
		LimitValue:     req.LimitValue,
		PeriodType:     req.PeriodType,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, limit)
}

// operatorDeleteQuotaLimit — DELETE /v1/operator/providers/{id}/subscriptions/{subscriptionId}/quota-limits/{key}?env=
func (s *Server) operatorDeleteQuotaLimit(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	subscriptionID, err := parseUUIDParam(w, r, "subscriptionId")
	if err != nil {
		return
	}
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "missing_key", "quota key is required", reqIDFromRequest(r))
		return
	}
	if err := s.svc.DeleteQuotaLimit(r.Context(), service.OperatorAuthContext(providerID, env), subscriptionID, key); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
