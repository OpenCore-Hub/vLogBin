package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
)

// setupHostedAuth — POST /v1/auth/zitadel/setup
func (s *Server) setupHostedAuth(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req struct {
		Name         string   `json:"name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	cfg, err := s.svc.SetupHostedAuth(r.Context(), tc, service.SetupHostedAuthInput{
		Name:         req.Name,
		RedirectURIs: req.RedirectURIs,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, cfg)
}

// getHostedAuthConfig — GET /v1/auth/zitadel/config
func (s *Server) getHostedAuthConfig(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	cfg, err := s.svc.GetHostedAuthConfig(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// disableHostedAuth — DELETE /v1/auth/zitadel
func (s *Server) disableHostedAuth(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	if err := s.svc.DisableHostedAuth(r.Context(), tc); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listHostedAuthConfigs — GET /v1/auth/zitadel/apps
func (s *Server) listHostedAuthConfigs(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	cfgs, err := s.svc.ListHostedAuthConfigs(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": cfgs})
}

// rotateHostedAuthSecret — POST /v1/auth/zitadel/rotate-secret
func (s *Server) rotateHostedAuthSecret(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	cfg, err := s.svc.RotateHostedAuthSecret(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// updateHostedAuthRedirectURIs — PUT /v1/auth/zitadel/redirect-uris
func (s *Server) updateHostedAuthRedirectURIs(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req struct {
		RedirectURIs []string `json:"redirect_uris"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	cfg, err := s.svc.UpdateHostedAuthRedirectURIs(r.Context(), tc, service.UpdateHostedAuthRedirectURIsInput{
		RedirectURIs: req.RedirectURIs,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}
