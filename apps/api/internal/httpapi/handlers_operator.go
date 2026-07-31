package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
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
	To string `json:"to"`
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
	res, err := s.svc.TransitionLifecycle(r.Context(), id, domain.LifecycleState(req.To))
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
