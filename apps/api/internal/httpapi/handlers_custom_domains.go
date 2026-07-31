package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type registerCustomDomainRequest struct {
	Domain string `json:"domain"`
}

// registerCustomDomain — POST /v1/custom-domains
func (s *Server) registerCustomDomain(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req registerCustomDomainRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	domain, err := s.svc.RegisterCustomDomain(r.Context(), tc, req.Domain)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, domain)
}

// listCustomDomains — GET /v1/custom-domains
func (s *Server) listCustomDomains(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	domains, err := s.svc.ListCustomDomains(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if domains == nil {
		domains = []storegen.CustomDomain{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"custom_domains": domains})
}

// getCustomDomain — GET /v1/custom-domains/{id}
func (s *Server) getCustomDomain(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	domainID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "domain id must be a uuid", reqIDFromRequest(r))
		return
	}
	domain, err := s.svc.GetCustomDomain(r.Context(), tc, domainID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, domain)
}

// verifyCustomDomain — POST /v1/custom-domains/{id}/verify
func (s *Server) verifyCustomDomain(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	domainID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "domain id must be a uuid", reqIDFromRequest(r))
		return
	}
	domain, err := s.svc.VerifyCustomDomain(r.Context(), tc, domainID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, domain)
}

// revokeCustomDomain — POST /v1/custom-domains/{id}/revoke
func (s *Server) revokeCustomDomain(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	domainID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "domain id must be a uuid", reqIDFromRequest(r))
		return
	}
	domain, err := s.svc.RevokeCustomDomain(r.Context(), tc, domainID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, domain)
}

// deleteCustomDomain — DELETE /v1/custom-domains/{id}
func (s *Server) deleteCustomDomain(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	domainID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "domain id must be a uuid", reqIDFromRequest(r))
		return
	}
	if err := s.svc.DeleteCustomDomain(r.Context(), tc, domainID); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
