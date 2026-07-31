package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
)

// createPSPCredential — POST /v1/psp-credentials
func (s *Server) createPSPCredential(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req struct {
		PSPType       string `json:"psp_type"`
		Label         string `json:"label"`
		APIKey        string `json:"api_key"`
		WebhookSecret string `json:"webhook_secret"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.svc.CreatePSPCredential(r.Context(), tc, service.CreatePSPCredentialInput{
		PSPType:       req.PSPType,
		Label:         req.Label,
		APIKey:        req.APIKey,
		WebhookSecret: req.WebhookSecret,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// listPSPCredentials — GET /v1/psp-credentials
func (s *Server) listPSPCredentials(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	creds, err := s.svc.ListPSPCredentials(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"credentials": creds})
}

// rotatePSPCredential — POST /v1/psp-credentials/{id}/rotate
func (s *Server) rotatePSPCredential(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	id, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	var req struct {
		NewAPIKey        string `json:"new_api_key"`
		NewWebhookSecret string `json:"new_webhook_secret"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.svc.RotatePSPCredential(r.Context(), tc, id, req.NewAPIKey, req.NewWebhookSecret)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// revokePSPCredential — DELETE /v1/psp-credentials/{id}
func (s *Server) revokePSPCredential(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	id, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	if err := s.svc.RevokePSPCredential(r.Context(), tc, id); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
