// Package httpapi exposes the platform v1 HTTP API: operator routes
// (operator token) and provider routes (API-key credentials) with a
// consistent error shape {"error":{"code","message"}}.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/go-chi/chi/v5"
)

type Server struct {
	store         *store.Store
	svc           *service.Service
	operatorToken string
	log           *slog.Logger
}

func NewServer(st *store.Store, svc *service.Service, operatorToken string, log *slog.Logger) *Server {
	return &Server{store: st, svc: svc, operatorToken: operatorToken, log: log}
}

func (s *Server) Router() chi.Router {
	r := chi.NewRouter()
	r.Use(requestIDMiddleware)
	r.Route("/v1", func(r chi.Router) {
		r.Route("/operator", func(r chi.Router) {
			r.Use(s.operatorAuth)
			r.Post("/providers", s.createProvider)
			r.Get("/providers", s.listProviders)
			r.Get("/providers/{id}", s.getProvider)
			r.Post("/providers/{id}/lifecycle", s.transitionLifecycle)
			r.Get("/regions", s.listRegions)
			r.Get("/cells", s.listCells)
		})
		r.Group(func(r chi.Router) {
			r.Use(s.apiKeyAuth)
			r.Use(s.tenantGuard)
			r.With(requireScope(domain.ScopeRead)).Get("/whoami", s.whoami)
			r.With(requireScope(domain.ScopeRead)).Get("/credentials", s.listCredentials)
			r.With(requireScope(domain.ScopeCredentialsManage)).Post("/credentials", s.createCredential)
			r.With(requireScope(domain.ScopeCredentialsManage)).Post("/credentials/{id}/revoke", s.revokeCredential)
			r.With(requireScope(domain.ScopeAuditRead)).Get("/audit-events", s.listAuditEvents)
			r.With(requireScope(domain.ScopeRead)).Get("/outbox-events", s.listOutboxEvents)
		})
	})
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// serviceError maps service-layer errors onto the HTTP error shape.
func (s *Server) serviceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, service.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, service.ErrValidation):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, domain.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "invalid_transition", err.Error())
	default:
		s.log.Error("internal error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return false
	}
	return true
}
