package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
)

// operatorListReconciliationResults — GET /v1/operator/reconciliation-results
func (s *Server) operatorListReconciliationResults(w http.ResponseWriter, r *http.Request) {
	// Operator-only endpoint: tenant context is not required but the
	// middleware has already validated the operator token.
	_, _ = tenant.FromContext(r.Context())
	results, err := s.svc.ListReconciliationResults(r.Context(), 100)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reconciliation_results": results})
}
