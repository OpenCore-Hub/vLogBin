package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
)

type setNotificationConfigRequest struct {
	Channel      string         `json:"channel"`
	ProviderType string         `json:"provider_type"`
	Config       map[string]any `json:"config"`
	FromAddress  string         `json:"from_address"`
	Enabled      bool           `json:"enabled"`
}

func (s *Server) setNotificationConfig(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req setNotificationConfigRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.svc.SetNotificationConfig(r.Context(), tc, service.NotificationConfigInput{
		Channel:      req.Channel,
		ProviderType: req.ProviderType,
		Config:       req.Config,
		FromAddress:  req.FromAddress,
		Enabled:      req.Enabled,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getNotificationConfig(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	channel := chi.URLParam(r, "channel")
	result, err := s.svc.GetNotificationConfig(r.Context(), tc, channel)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listNotificationConfigs(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	configs, err := s.svc.ListNotificationConfigs(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if configs == nil {
		configs = []service.NotificationConfigResult{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notification_configs": configs})
}

func (s *Server) deleteNotificationConfig(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	channel := chi.URLParam(r, "channel")
	if err := s.svc.DeleteNotificationConfig(r.Context(), tc, channel); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
