package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) whoami(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"provider_id":      tc.ProviderID,
		"slug":             tc.ProviderSlug,
		"environment_kind": tc.EnvironmentKind,
		"environment_id":   tc.EnvironmentID,
		"issuer":           tc.Issuer,
		"lifecycle_state":  tc.LifecycleState,
		"scopes":           tc.Scopes,
	})
}

// credentialView is the safe external representation of a credential: the
// key hash is never exposed.
type credentialView struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	KeyPrefix    string     `json:"key_prefix"`
	Scopes       []string   `json:"scopes"`
	AllowedCIDRs []string   `json:"allowed_cidrs,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

func toCredentialView(c storegen.Credential) credentialView {
	return credentialView{
		ID:           c.ID,
		Name:         c.Name,
		KeyPrefix:    c.KeyPrefix,
		Scopes:       c.Scopes,
		AllowedCIDRs: c.AllowedCidrs,
		ExpiresAt:    c.ExpiresAt,
		RevokedAt:    c.RevokedAt,
		LastUsedAt:   c.LastUsedAt,
		CreatedAt:    c.CreatedAt,
	}
}

func (s *Server) listCredentials(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	creds, err := s.svc.ListCredentials(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	views := make([]credentialView, 0, len(creds))
	for _, c := range creds {
		views = append(views, toCredentialView(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"credentials": views})
}

type createCredentialRequest struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (s *Server) createCredential(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req createCredentialRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	created, err := s.svc.CreateCredential(r.Context(), tc, req.Name, req.Scopes, req.ExpiresAt)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"credential": toCredentialView(created.Credential),
		"api_key":    created.APIKey, // plaintext, returned exactly once
	})
}

func (s *Server) revokeCredential(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "credential id must be a uuid", reqIDFromRequest(r))
		return
	}
	cred, err := s.svc.RevokeCredential(r.Context(), tc, id)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"credential": toCredentialView(*cred)})
}

func (s *Server) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	events, err := s.svc.ListAuditEvents(r.Context(), tc, queryLimit(r, 100))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"audit_events": events})
}

func (s *Server) listOutboxEvents(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	events, err := s.svc.ListOutboxEvents(r.Context(), tc, queryLimit(r, 100))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"outbox_events": events})
}

func queryLimit(r *http.Request, fallback int32) int32 {
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			return int32(n)
		}
	}
	return fallback
}
