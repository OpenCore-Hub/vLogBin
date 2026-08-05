package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/go-chi/chi/v5"
)

// ---- Console control plane: Customers (§8 M2) ----

// operatorCreateCustomer — POST /v1/operator/providers/{id}/customers?env=test
func (s *Server) operatorCreateCustomer(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	var req createCustomerRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	customer, err := s.svc.CreateCustomer(r.Context(), service.OperatorAuthContext(providerID, env),
		req.ExternalID, req.AccountType, req.DisplayName)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"customer": customer})
}

// operatorGetCustomer — GET /v1/operator/providers/{id}/customers/{externalId}?env=test
//
// Returns the customer plus subscriptions / usage events / invoices in one
// request (DB-side customer filter), so the detail page tabs need no fan-out.
func (s *Server) operatorGetCustomer(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	externalID := chi.URLParam(r, "externalId")
	detail, err := s.svc.GetCustomerDetail(r.Context(), service.OperatorAuthContext(providerID, env), externalID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
