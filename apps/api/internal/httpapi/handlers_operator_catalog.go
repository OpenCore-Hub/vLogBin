package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/go-chi/chi/v5"
)

// ---- Console control plane: Plans (§8 M2) ----
//
// The provider-domain /v1/catalog/plans endpoints require an API-key tenant
// context. The Console speaks operator-session auth, so these endpoints
// resolve the environment from an explicit ?env= query parameter and reuse
// the same service methods — the tenant context is constructed from the
// resolved provider environment instead of a credential.

// operatorListCatalogPlans — GET /v1/operator/providers/{id}/catalog/plans?env=test
func (s *Server) operatorListCatalogPlans(w http.ResponseWriter, r *http.Request) {
	_, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	providerID := env.ProviderID
	collection, err := s.svc.ListPlanDetails(r.Context(), service.OperatorAuthContext(providerID, env))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, collection)
}

// operatorGetCatalogPlan — GET /v1/operator/providers/{id}/catalog/plans/{code}?env=test
func (s *Server) operatorGetCatalogPlan(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	code := chi.URLParam(r, "code")
	detail, err := s.svc.GetPlanDetail(r.Context(), service.OperatorAuthContext(providerID, env), code)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": detail})
}

// operatorCreateCatalogPlan — POST /v1/operator/providers/{id}/catalog/plans?env=test
func (s *Server) operatorCreateCatalogPlan(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	var input domain.PlanInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if _, err := s.svc.CreatePlan(r.Context(), service.OperatorAuthContext(providerID, env), input); err != nil {
		s.serviceError(w, r, err)
		return
	}
	detail, err := s.svc.GetPlanDetail(r.Context(), service.OperatorAuthContext(providerID, env), input.Code)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"plan": detail})
}

// operatorUpdateCatalogPlan — PUT /v1/operator/providers/{id}/catalog/plans/{code}?env=test
func (s *Server) operatorUpdateCatalogPlan(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	code := chi.URLParam(r, "code")
	var input domain.PlanInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if _, err := s.svc.UpdatePlan(r.Context(), service.OperatorAuthContext(providerID, env), code, input); err != nil {
		s.serviceError(w, r, err)
		return
	}
	detail, err := s.svc.GetPlanDetail(r.Context(), service.OperatorAuthContext(providerID, env), code)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plan": detail})
}

// operatorDeleteCatalogPlan — DELETE /v1/operator/providers/{id}/catalog/plans/{code}?env=test
func (s *Server) operatorDeleteCatalogPlan(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	code := chi.URLParam(r, "code")
	if err := s.svc.DeletePlan(r.Context(), service.OperatorAuthContext(providerID, env), code); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
