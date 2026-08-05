package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// hostedAuthConfigView is the operator-facing shape of an OIDC application.
// The encrypted client secret is never returned; redirect_uris is decoded
// from its JSONB storage so clients receive plain JSON instead of base64.
type hostedAuthConfigView struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	ClientID     string   `json:"client_id"`
	Enabled      bool     `json:"enabled"`
	RedirectURIs []string `json:"redirect_uris"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
	ClientSecret *string  `json:"client_secret,omitempty"` // plaintext, shown exactly once
	IssuerURL    string   `json:"issuer_url,omitempty"`
}

func newHostedAuthConfigView(cfg storegen.ProviderAuthConfig) hostedAuthConfigView {
	var redirectURIs []string
	if len(cfg.RedirectUris) > 0 {
		_ = json.Unmarshal(cfg.RedirectUris, &redirectURIs)
	}
	return hostedAuthConfigView{
		ID:           cfg.ID.String(),
		Name:         cfg.Name,
		ClientID:     cfg.ZitadelClientID,
		Enabled:      cfg.Enabled,
		RedirectURIs: redirectURIs,
		CreatedAt:    cfg.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    cfg.UpdatedAt.Format(time.RFC3339),
	}
}

// providerEnvFromRequest resolves the provider path param and the explicit
// ?env= query parameter (test/live). Unknown providers or environments yield
// 404; malformed input yields 400.
func (s *Server) providerEnvFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, *storegen.Environment, bool) {
	providerID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "provider id must be a uuid", reqIDFromRequest(r))
		return uuid.Nil, nil, false
	}
	kind := r.URL.Query().Get("env")
	if kind != "test" && kind != "live" {
		writeError(w, http.StatusBadRequest, "invalid_env", "env must be test or live", reqIDFromRequest(r))
		return uuid.Nil, nil, false
	}
	env, err := s.svc.ResolveProviderEnvironment(r.Context(), providerID, kind)
	if err != nil {
		s.serviceError(w, r, err)
		return uuid.Nil, nil, false
	}
	return providerID, env, true
}

// operatorListHostedAuthConfigs — GET /v1/operator/providers/{id}/auth/zitadel/apps?env=test
func (s *Server) operatorListHostedAuthConfigs(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	cfgs, err := s.svc.ListHostedAuthConfigs(r.Context(), service.OperatorAuthContext(providerID, env))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	views := make([]hostedAuthConfigView, 0, len(cfgs))
	for _, cfg := range cfgs {
		views = append(views, newHostedAuthConfigView(cfg))
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": views})
}

// operatorSetupHostedAuth — POST /v1/operator/providers/{id}/auth/zitadel/setup?env=test
func (s *Server) operatorSetupHostedAuth(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		Name         string   `json:"name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	cfg, err := s.svc.SetupHostedAuth(r.Context(), service.OperatorAuthContext(providerID, env), service.SetupHostedAuthInput{
		Name:         req.Name,
		RedirectURIs: req.RedirectURIs,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	view := newHostedAuthConfigView(cfg.Config)
	view.IssuerURL = cfg.IssuerURL
	writeJSON(w, http.StatusCreated, map[string]any{"app": view})
}

// operatorRotateHostedAuthSecret — POST /v1/operator/providers/{id}/auth/zitadel/rotate-secret?env=test
func (s *Server) operatorRotateHostedAuthSecret(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	cfg, err := s.svc.RotateHostedAuthSecret(r.Context(), service.OperatorAuthContext(providerID, env))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	view := newHostedAuthConfigView(cfg.Config)
	view.ClientSecret = &cfg.ClientSecret
	view.IssuerURL = cfg.IssuerURL
	writeJSON(w, http.StatusOK, map[string]any{"app": view})
}

// operatorUpdateHostedAuthRedirectURIs — PUT /v1/operator/providers/{id}/auth/zitadel/redirect-uris?env=test
func (s *Server) operatorUpdateHostedAuthRedirectURIs(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	var req struct {
		RedirectURIs []string `json:"redirect_uris"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	cfg, err := s.svc.UpdateHostedAuthRedirectURIs(r.Context(), service.OperatorAuthContext(providerID, env), service.UpdateHostedAuthRedirectURIsInput{
		RedirectURIs: req.RedirectURIs,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	view := newHostedAuthConfigView(cfg.Config)
	view.IssuerURL = cfg.IssuerURL
	writeJSON(w, http.StatusOK, map[string]any{"app": view})
}

// operatorDisableHostedAuth — DELETE /v1/operator/providers/{id}/auth/zitadel?env=test
func (s *Server) operatorDisableHostedAuth(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	if err := s.svc.DisableHostedAuth(r.Context(), service.OperatorAuthContext(providerID, env)); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
