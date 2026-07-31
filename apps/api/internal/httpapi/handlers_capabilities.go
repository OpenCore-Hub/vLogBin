package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// parseCapabilityParam extracts and validates the provider ID and capability
// from the URL. Returns (providerID, capability, true) on success, or
// (uuid.Nil, "", false) after writing an error response on failure.
func parseCapabilityParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, string, bool) {
	providerID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return uuid.Nil, "", false
	}
	capability := chi.URLParam(r, "capability")
	if !domain.ValidCapability(capability) {
		writeError(w, http.StatusBadRequest, "invalid_capability", "unknown capability: "+capability, reqIDFromRequest(r))
		return uuid.Nil, "", false
	}
	return providerID, capability, true
}

// operatorGrantCapability — POST /v1/operator/providers/{id}/capabilities/{capability}/grant
func (s *Server) operatorGrantCapability(w http.ResponseWriter, r *http.Request) {
	providerID, capability, ok := parseCapabilityParam(w, r)
	if !ok {
		return
	}
	var req struct {
		GrantedBy string `json:"granted_by"`
		Reason    string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.GrantedBy == "" {
		req.GrantedBy = "operator"
	}
	cap, err := s.svc.GrantCapability(r.Context(), providerID, capability, req.GrantedBy, req.Reason)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"capability": cap})
}

// operatorRevokeCapability — POST /v1/operator/providers/{id}/capabilities/{capability}/revoke
func (s *Server) operatorRevokeCapability(w http.ResponseWriter, r *http.Request) {
	providerID, capability, ok := parseCapabilityParam(w, r)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = decodeJSON(w, r, &req) // body is optional for revoke
	cap, err := s.svc.RevokeCapability(r.Context(), providerID, capability, req.Reason)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"capability": cap})
}

// operatorListCapabilities — GET /v1/operator/providers/{id}/capabilities
func (s *Server) operatorListCapabilities(w http.ResponseWriter, r *http.Request) {
	providerID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	caps, err := s.svc.ListCapabilitiesByProvider(r.Context(), providerID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"capabilities": caps})
}

// listMyCapabilities — GET /v1/capabilities (provider-scoped)
func (s *Server) listMyCapabilities(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	caps, err := s.svc.ListMyCapabilities(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"capabilities": caps})
}
