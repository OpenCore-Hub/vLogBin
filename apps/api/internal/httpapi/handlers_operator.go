package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type createProviderRequest struct {
	Slug           string `json:"slug"`
	Name           string `json:"name"`
	HomeRegionCode string `json:"home_region_code"`
}

type createProviderResponse struct {
	Provider        any    `json:"provider"`
	Environments    any    `json:"environments"`
	TestEnvironment any    `json:"test_environment"` // the auto-created test environment
	APIKey          string `json:"api_key"`          // plaintext, returned exactly once
}

func (s *Server) createProvider(w http.ResponseWriter, r *http.Request) {
	var req createProviderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	created, err := s.svc.CreateProvider(r.Context(), req.Slug, req.Name, req.HomeRegionCode)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	resp := createProviderResponse{
		Provider:     created.Provider,
		Environments: created.Environments,
		APIKey:       created.TestAPIKey,
	}
	if len(created.Environments) > 0 {
		resp.TestEnvironment = created.Environments[0]
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) listProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := s.svc.ListProviders(r.Context())
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": providers})
}

func (s *Server) getProvider(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "provider id must be a uuid", reqIDFromRequest(r))
		return
	}
	detail, err := s.svc.GetProvider(r.Context(), id)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"provider":     detail.Provider,
		"environments": detail.Environments,
	})
}

type lifecycleRequest struct {
	To     string `json:"to"`
	Reason string `json:"reason"` // optional operator-supplied reason, recorded in audit + events
	Actor  string `json:"actor"`  // optional operator identity; empty falls back to "operator"
}

func (s *Server) transitionLifecycle(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "provider id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req lifecycleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	res, err := s.svc.TransitionLifecycle(r.Context(), id, service.LifecycleTransitionInput{
		To:     domain.LifecycleState(req.To),
		Reason: req.Reason,
		Actor:  req.Actor,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	resp := map[string]any{"provider": res.Provider}
	if res.LiveEnvironment != nil {
		resp["environment"] = res.LiveEnvironment
	}
	if res.LiveAPIKey != "" {
		resp["api_key"] = res.LiveAPIKey // plaintext live key, returned exactly once
	}
	writeJSON(w, http.StatusOK, resp)
}

type activateProviderRequest struct {
	HomeRegionCode string `json:"home_region_code"`
	Reason         string `json:"reason"` // optional operator-supplied reason, recorded in audit + events
	Actor          string `json:"actor"`  // optional operator identity; empty falls back to "operator"
}

// activateProvider moves a signup-created REGISTERED provider to TEST_ACTIVE,
// assigning the operator-chosen home region and provisioning the test
// environment with its initial API key (design baseline §2.1). Activating a
// provider that is not REGISTERED yields 409; a missing region code yields 400.
func (s *Server) activateProvider(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "provider id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req activateProviderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	created, err := s.svc.ActivateProvider(r.Context(), id, req.HomeRegionCode, req.Reason, req.Actor)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	resp := createProviderResponse{
		Provider:     created.Provider,
		Environments: created.Environments,
		APIKey:       created.TestAPIKey,
	}
	if len(created.Environments) > 0 {
		resp.TestEnvironment = created.Environments[0]
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) listRegions(w http.ResponseWriter, r *http.Request) {
	regions, err := s.svc.ListRegions(r.Context())
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"regions": regions})
}

func (s *Server) listCells(w http.ResponseWriter, r *http.Request) {
	cells, err := s.svc.ListCells(r.Context())
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cells": cells})
}

// listProviderAuditEvents exposes a provider's cross-environment audit trail
// to the operator console. The provider-facing /v1/audit-events endpoint is
// scoped to a single tenant; this operator view reads across all environments.
// The JSONB metadata column is decoded so clients receive plain JSON instead
// of a base64-encoded []byte.
func (s *Server) listProviderAuditEvents(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "provider id must be a uuid", reqIDFromRequest(r))
		return
	}
	q, err := auditQueryFromRequest(r, 200)
	if err != nil {
		writeAuditQueryError(w, r, err)
		return
	}
	events, next, err := s.svc.ListProviderAuditEvents(r.Context(), id, q)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	views := make([]auditEventView, 0, len(events))
	for _, e := range events {
		views = append(views, newAuditEventView(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"audit_events": views, "next_cursor": next})
}

// providerAuditEventStats renders the cross-environment audit dashboard
// aggregates for one provider. It reuses the same bounded-window validation as
// the tenant-facing stats endpoint.
func (s *Server) providerAuditEventStats(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "provider id must be a uuid", reqIDFromRequest(r))
		return
	}
	q, err := auditStatsQueryFromRequest(r)
	if err != nil {
		writeAuditQueryError(w, r, err)
		return
	}
	stats, err := s.svc.ProviderAuditEventStats(r.Context(), id, q)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// providerAuditEventExport streams a provider's cross-environment audit trail
// as a downloadable file (operator view). Same bounded-window guards as the
// tenant-facing export: unknown providers 404, missing or oversized windows are
// rejected before any bytes are written.
func (s *Server) providerAuditEventExport(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "provider id must be a uuid", reqIDFromRequest(r))
		return
	}
	q, format, err := auditExportQueryFromRequest(r)
	if err != nil {
		writeAuditQueryError(w, r, err)
		return
	}
	// Probe provider existence before streaming: once the CSV/JSON header row
	// is written the status code is locked in, so unknown providers must 404
	// here with a proper JSON body.
	if _, err := s.svc.GetProvider(r.Context(), id); err != nil {
		s.serviceError(w, r, err)
		return
	}
	s.streamAuditExport(w, r, format, func(emit func(storegen.AuditEvent) error) error {
		return s.svc.ExportProviderAuditEvents(r.Context(), id, q, emit)
	})
}

type auditEventView struct {
	ID            int64           `json:"id"`
	ProviderID    uuid.UUID       `json:"provider_id"`
	EnvironmentID *uuid.UUID      `json:"environment_id,omitempty"`
	ActorType     string          `json:"actor_type"`
	ActorID       string          `json:"actor_id"`
	Action        string          `json:"action"`
	TargetType    string          `json:"target_type,omitempty"`
	TargetID      string          `json:"target_id,omitempty"`
	Metadata      json.RawMessage `json:"metadata"`
	RequestID     string          `json:"request_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

func newAuditEventView(e storegen.AuditEvent) auditEventView {
	v := auditEventView{
		ID:         e.ID,
		ProviderID: e.ProviderID.UUID,
		ActorType:  e.ActorType,
		ActorID:    e.ActorID,
		Action:     e.Action,
		Metadata:   json.RawMessage(e.Metadata),
		CreatedAt:  e.CreatedAt,
	}
	if e.EnvironmentID.Valid {
		v.EnvironmentID = &e.EnvironmentID.UUID
	}
	if e.TargetType.Valid {
		v.TargetType = e.TargetType.String
	}
	if e.TargetID.Valid {
		v.TargetID = e.TargetID.String
	}
	if e.RequestID.Valid {
		v.RequestID = e.RequestID.String
	}
	return v
}

// providerCredentialView is the operator-facing representation of an API key.
// key_hash is deliberately absent: keys are identified by prefix only.
type providerCredentialView struct {
	ID                uuid.UUID  `json:"id"`
	Name              string     `json:"name"`
	KeyPrefix         string     `json:"key_prefix"`
	Scopes            []string   `json:"scopes"`
	AllowedCIDRs      []string   `json:"allowed_cidrs,omitempty"`
	EnvironmentID     uuid.UUID  `json:"environment_id"`
	EnvironmentKind   string     `json:"environment_kind"`
	EnvironmentIssuer string     `json:"environment_issuer"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	RevokedAt         *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt        *time.Time `json:"last_used_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

func newProviderCredentialView(c storegen.ListCredentialsByProviderRow) providerCredentialView {
	return providerCredentialView{
		ID:                c.ID,
		Name:              c.Name,
		KeyPrefix:         c.KeyPrefix,
		Scopes:            c.Scopes,
		AllowedCIDRs:      c.AllowedCidrs,
		EnvironmentID:     c.EnvironmentID,
		EnvironmentKind:   c.EnvironmentKind,
		EnvironmentIssuer: c.EnvironmentIssuer,
		ExpiresAt:         c.ExpiresAt,
		RevokedAt:         c.RevokedAt,
		LastUsedAt:        c.LastUsedAt,
		CreatedAt:         c.CreatedAt,
	}
}

func newProviderCredentialRowView(c storegen.GetCredentialByProviderRow) providerCredentialView {
	return providerCredentialView{
		ID:                c.ID,
		Name:              c.Name,
		KeyPrefix:         c.KeyPrefix,
		Scopes:            c.Scopes,
		AllowedCIDRs:      c.AllowedCidrs,
		EnvironmentID:     c.EnvironmentID,
		EnvironmentKind:   c.EnvironmentKind,
		EnvironmentIssuer: c.EnvironmentIssuer,
		ExpiresAt:         c.ExpiresAt,
		RevokedAt:         c.RevokedAt,
		LastUsedAt:        c.LastUsedAt,
		CreatedAt:         c.CreatedAt,
	}
}

func newProviderCredentialEnvView(c storegen.Credential, envKind, envIssuer string) providerCredentialView {
	return providerCredentialView{
		ID:                c.ID,
		Name:              c.Name,
		KeyPrefix:         c.KeyPrefix,
		Scopes:            c.Scopes,
		AllowedCIDRs:      c.AllowedCidrs,
		EnvironmentID:     c.EnvironmentID,
		EnvironmentKind:   envKind,
		EnvironmentIssuer: envIssuer,
		ExpiresAt:         c.ExpiresAt,
		RevokedAt:         c.RevokedAt,
		LastUsedAt:        c.LastUsedAt,
		CreatedAt:         c.CreatedAt,
	}
}

// listProviderCredentials — GET /v1/operator/providers/{id}/credentials
// Operator view of a provider's API keys across all environments. The raw key
// hash is never returned; keys are identified by their key_prefix.
func (s *Server) listProviderCredentials(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("env") != "" {
		providerID, env, ok := s.providerEnvFromRequest(w, r)
		if !ok {
			return
		}
		creds, err := s.svc.ListProviderCredentialsByEnv(r.Context(), providerID, env.ID)
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		views := make([]providerCredentialView, 0, len(creds))
		for _, c := range creds {
			views = append(views, newProviderCredentialEnvView(c, env.Kind, env.Issuer))
		}
		writeJSON(w, http.StatusOK, map[string]any{"credentials": views})
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "provider id must be a uuid", reqIDFromRequest(r))
		return
	}
	creds, err := s.svc.ListProviderCredentials(r.Context(), id)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	views := make([]providerCredentialView, 0, len(creds))
	for _, c := range creds {
		views = append(views, newProviderCredentialView(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"credentials": views})
}

// revokeProviderCredential — POST /v1/operator/providers/{id}/credentials/{credentialId}/revoke
// Revokes a provider API key immediately. The optional revoked_by field
// records who performed the action on the provider's audit trail and defaults
// to "operator".
func (s *Server) revokeProviderCredential(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "provider id must be a uuid", reqIDFromRequest(r))
		return
	}
	credentialID, err := uuid.Parse(chi.URLParam(r, "credentialId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_credential_id", "credential id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req struct {
		RevokedBy string `json:"revoked_by"`
	}
	if r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.RevokedBy == "" {
		req.RevokedBy = "operator"
	}
	cred, err := s.svc.RevokeProviderCredential(r.Context(), id, credentialID, req.RevokedBy)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"credential": newProviderCredentialRowView(*cred)})
}
