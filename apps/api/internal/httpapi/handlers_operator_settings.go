package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// operatorListCustomDomains — GET /v1/operator/providers/{id}/custom-domains?env=test
func (s *Server) operatorListCustomDomains(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	domains, err := s.svc.ListCustomDomainsByProviderEnv(r.Context(), providerID, env.ID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if domains == nil {
		domains = []storegen.CustomDomain{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"custom_domains": domains})
}

// operatorRegisterCustomDomain — POST /v1/operator/providers/{id}/custom-domains?env=test
func (s *Server) operatorRegisterCustomDomain(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	var req registerCustomDomainRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	domain, err := s.svc.RegisterCustomDomainByProvider(r.Context(), providerID, env, req.Domain, operatorIdentity(r))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, domain)
}

// operatorVerifyCustomDomain — POST /v1/operator/providers/{id}/custom-domains/{domainId}/verify?env=test
func (s *Server) operatorVerifyCustomDomain(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	domainID, err := uuid.Parse(chi.URLParam(r, "domainId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_domain_id", "domain id must be a uuid", reqIDFromRequest(r))
		return
	}
	domain, err := s.svc.VerifyCustomDomainByProvider(r.Context(), providerID, env, domainID, operatorIdentity(r))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, domain)
}

// operatorRevokeCustomDomain — POST /v1/operator/providers/{id}/custom-domains/{domainId}/revoke?env=test
func (s *Server) operatorRevokeCustomDomain(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	domainID, err := uuid.Parse(chi.URLParam(r, "domainId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_domain_id", "domain id must be a uuid", reqIDFromRequest(r))
		return
	}
	domain, err := s.svc.RevokeCustomDomainByProvider(r.Context(), providerID, env, domainID, operatorIdentity(r))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, domain)
}

// operatorDeleteCustomDomain — DELETE /v1/operator/providers/{id}/custom-domains/{domainId}?env=test
func (s *Server) operatorDeleteCustomDomain(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	domainID, err := uuid.Parse(chi.URLParam(r, "domainId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_domain_id", "domain id must be a uuid", reqIDFromRequest(r))
		return
	}
	if err := s.svc.DeleteCustomDomainByProvider(r.Context(), providerID, env, domainID, operatorIdentity(r)); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// operatorListNotificationConfigs — GET /v1/operator/providers/{id}/notification-configs?env=test
func (s *Server) operatorListNotificationConfigs(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	configs, err := s.svc.ListNotificationConfigsByProviderEnv(r.Context(), providerID, env.ID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if configs == nil {
		configs = []service.NotificationConfigResult{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"notification_configs": configs})
}

// operatorSetNotificationConfig — PUT /v1/operator/providers/{id}/notification-configs?env=test
func (s *Server) operatorSetNotificationConfig(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	var req setNotificationConfigRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.svc.SetNotificationConfigByProvider(r.Context(), providerID, env, service.NotificationConfigInput{
		Channel:      req.Channel,
		ProviderType: req.ProviderType,
		Config:       req.Config,
		FromAddress:  req.FromAddress,
		Enabled:      req.Enabled,
	}, operatorIdentity(r))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// operatorDeleteNotificationConfig — DELETE /v1/operator/providers/{id}/notification-configs/{channel}?env=test
func (s *Server) operatorDeleteNotificationConfig(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	channel := chi.URLParam(r, "channel")
	if err := s.svc.DeleteNotificationConfigByProvider(r.Context(), providerID, env, channel, operatorIdentity(r)); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
