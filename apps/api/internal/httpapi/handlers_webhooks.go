package httpapi

import (
	"net/http"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ---- webhook endpoints ----

type createWebhookRequest struct {
	URL       string   `json:"url"`
	Secret    string   `json:"secret,omitempty"`
	Events    []string `json:"events,omitempty"`
	CreatedBy string   `json:"created_by,omitempty"`
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req createWebhookRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	endpoint, err := s.svc.CreateWebhookEndpoint(r.Context(), tc, service.CreateWebhookEndpointInput{
		URL:    req.URL,
		Secret: req.Secret,
		Events: req.Events,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"endpoint": endpoint})
}

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	endpoints, err := s.svc.ListWebhookEndpoints(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoints": endpoints})
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	id, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	if err := s.svc.DeleteWebhookEndpoint(r.Context(), tc, id); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	deliveries, err := s.svc.ListWebhookDeliveries(r.Context(), tc, queryLimit(r, 100))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": deliveries})
}

// ---- operator views (cross-environment) ----

// operatorWebhookEndpointView is the operator-facing webhook endpoint
// representation. The signing secret is only included in the create response
// (returned exactly once); list views never expose it.
type operatorWebhookEndpointView struct {
	ID                uuid.UUID `json:"id"`
	ProviderID        uuid.UUID `json:"provider_id"`
	EnvironmentID     uuid.UUID `json:"environment_id"`
	EnvironmentKind   string    `json:"environment_kind,omitempty"`
	EnvironmentIssuer string    `json:"environment_issuer,omitempty"`
	URL               string    `json:"url"`
	Secret            string    `json:"secret,omitempty"`
	Enabled           bool      `json:"enabled"`
	Events            []string  `json:"events"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func newOperatorWebhookEndpointView(
	ep storegen.WebhookEndpoint,
	includeSecret bool,
	envKind, envIssuer string,
) operatorWebhookEndpointView {
	v := operatorWebhookEndpointView{
		ID:                ep.ID,
		ProviderID:        ep.ProviderID,
		EnvironmentID:     ep.EnvironmentID,
		EnvironmentKind:   envKind,
		EnvironmentIssuer: envIssuer,
		URL:               ep.Url,
		Enabled:           ep.Enabled,
		Events:            ep.Events,
		CreatedAt:         ep.CreatedAt,
		UpdatedAt:         ep.UpdatedAt,
	}
	if includeSecret {
		v.Secret = ep.Secret
	}
	return v
}

// operatorListWebhooks — GET /v1/operator/providers/{id}/webhooks[?env=test]
// With ?env= the Console sees one environment's endpoints; without it the
// legacy cross-environment operator view is returned.
func (s *Server) operatorListWebhooks(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("env") != "" {
		providerID, env, ok := s.providerEnvFromRequest(w, r)
		if !ok {
			return
		}
		endpoints, err := s.svc.ListWebhookEndpoints(r.Context(), service.OperatorAuthContext(providerID, env))
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		views := make([]operatorWebhookEndpointView, 0, len(endpoints))
		for _, ep := range endpoints {
			views = append(views, newOperatorWebhookEndpointView(ep, false, env.Kind, env.Issuer))
		}
		writeJSON(w, http.StatusOK, map[string]any{"endpoints": views})
		return
	}
	providerID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	endpoints, err := s.svc.ListWebhooksByProvider(r.Context(), providerID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	views := make([]operatorWebhookEndpointView, 0, len(endpoints))
	for _, ep := range endpoints {
		views = append(views, newOperatorWebhookEndpointView(ep, false, "", ""))
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoints": views})
}

func (s *Server) operatorListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("env") != "" {
		providerID, env, ok := s.providerEnvFromRequest(w, r)
		if !ok {
			return
		}
		deliveries, err := s.svc.ListWebhookDeliveries(r.Context(), service.OperatorAuthContext(providerID, env), queryLimit(r, 200))
		if err != nil {
			s.serviceError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deliveries": deliveries})
		return
	}
	providerID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	deliveries, err := s.svc.ListWebhookDeliveriesByProvider(r.Context(), providerID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": deliveries})
}

// operatorCreateWebhook — POST /v1/operator/providers/{id}/webhooks?env=test
func (s *Server) operatorCreateWebhook(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	var req createWebhookRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	endpoint, err := s.svc.CreateWebhookEndpointByProvider(r.Context(), providerID, env, service.CreateWebhookEndpointInput{
		URL:    req.URL,
		Secret: req.Secret,
		Events: req.Events,
	}, req.CreatedBy)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"endpoint": newOperatorWebhookEndpointView(*endpoint, true, env.Kind, env.Issuer),
	})
}

// operatorDeleteWebhook — DELETE /v1/operator/providers/{id}/webhooks/{webhookId}?env=test
func (s *Server) operatorDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	providerID, env, ok := s.providerEnvFromRequest(w, r)
	if !ok {
		return
	}
	endpointID, err := uuid.Parse(chi.URLParam(r, "webhookId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_webhook_id", "webhook id must be a uuid", reqIDFromRequest(r))
		return
	}
	if err := s.svc.DeleteWebhookEndpointByProvider(r.Context(), providerID, env.ID, endpointID, "operator"); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type replayWebhookDeliveryRequest struct {
	Actor string `json:"actor,omitempty"`
}

// operatorReplayWebhookDelivery requeues a terminal (dead_letter | failed)
// webhook delivery for immediate redelivery. The body is optional and only
// carries an audit actor; the replay itself is an operator-only action.
func (s *Server) operatorReplayWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	providerID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	deliveryID, err := parseUUIDParam(w, r, "deliveryId")
	if err != nil {
		return
	}
	var req replayWebhookDeliveryRequest
	if r.ContentLength > 0 {
		if !decodeJSON(w, r, &req) {
			return
		}
	}
	delivery, err := s.svc.ReplayWebhookDeliveryByProvider(r.Context(), providerID, deliveryID, req.Actor)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"delivery": delivery})
}
