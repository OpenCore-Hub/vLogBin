package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/go-chi/chi/v5"
)

type authVaultCreateRequest struct {
	UserSub      string   `json:"userSub"`
	Email        string   `json:"email"`
	Name         string   `json:"name"`
	Roles        []string `json:"roles"`
	WorkspaceID  string   `json:"workspaceId"`
	Env          string   `json:"env"`
	AccessToken  string   `json:"accessToken"`
	RefreshToken string   `json:"refreshToken"`
	TokenExp     int64    `json:"tokenExp"`
	TTLSeconds   int64    `json:"ttlSeconds"`
}

func (s *Server) authVaultAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authVaultToken == "" {
			writeError(w, http.StatusServiceUnavailable, "not_configured", "auth vault service token not configured", reqIDFromRequest(r))
			return
		}
		token, ok := bearerToken(r)
		if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(s.authVaultToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid auth vault token", reqIDFromRequest(r))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authVaultCreate(w http.ResponseWriter, r *http.Request) {
	var req authVaultCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), reqIDFromRequest(r))
		return
	}
	if req.UserSub == "" || req.AccessToken == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "userSub and accessToken are required", reqIDFromRequest(r))
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	vault, err := s.svc.CreateAuthVault(r.Context(), service.CreateAuthVaultInput{
		UserSub:      req.UserSub,
		Email:        req.Email,
		Name:         req.Name,
		Roles:        req.Roles,
		WorkspaceID:  req.WorkspaceID,
		Env:          req.Env,
		AccessToken:  req.AccessToken,
		RefreshToken: req.RefreshToken,
		TokenExp:     req.TokenExp,
		TTL:          ttl,
	})
	if err != nil {
		s.authVaultMetric("create", false)
		writeServiceError(w, r, err)
		return
	}
	s.authVaultMetric("create", true)
	writeJSON(w, http.StatusCreated, map[string]any{
		"vault": authVaultView(vault),
	})
}

func (s *Server) authVaultGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	vault, err := s.svc.GetAuthVault(r.Context(), id)
	if err != nil {
		s.authVaultMetric("get", false)
		writeServiceError(w, r, err)
		return
	}
	s.authVaultMetric("get", true)
	writeJSON(w, http.StatusOK, map[string]any{
		"vault": authVaultView(vault),
	})
}

func (s *Server) authVaultDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.svc.DeleteAuthVault(r.Context(), id); err != nil {
		s.authVaultMetric("delete", false)
		writeServiceError(w, r, err)
		return
	}
	s.authVaultMetric("delete", true)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) authVaultMetric(operation string, ok bool) {
	result := "success"
	if !ok {
		result = "error"
	}
	s.metrics.AuthVaultOperationsTotal.WithLabelValues(operation, result).Inc()
}

func authVaultView(v service.AuthVault) map[string]any {
	return map[string]any{
		"id":           v.ID,
		"userSub":      v.UserSub,
		"email":        v.Email,
		"name":         v.Name,
		"roles":        v.Roles,
		"workspaceId":  v.WorkspaceID,
		"env":          v.Env,
		"accessToken":  v.AccessToken,
		"refreshToken": v.RefreshToken,
		"tokenExp":     v.TokenExp,
		"expiresAt":    v.ExpiresAt,
	}
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, service.ErrAuthVaultNotFound) {
		writeError(w, http.StatusNotFound, "not_found", err.Error(), reqIDFromRequest(r))
		return
	}
	if errors.Is(err, service.ErrAuthVaultEncryptionDisabled) {
		writeError(w, http.StatusServiceUnavailable, "not_configured", err.Error(), reqIDFromRequest(r))
		return
	}
	writeError(w, http.StatusInternalServerError, "internal", err.Error(), reqIDFromRequest(r))
}
