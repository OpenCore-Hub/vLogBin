package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
)

// ---- webhook endpoints ----

type createWebhookRequest struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret,omitempty"`
	Events []string `json:"events,omitempty"`
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

func (s *Server) operatorListWebhooks(w http.ResponseWriter, r *http.Request) {
	providerID, err := parseUUIDParam(w, r, "id")
	if err != nil {
		return
	}
	endpoints, err := s.svc.ListWebhooksByProvider(r.Context(), providerID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoints": endpoints})
}

func (s *Server) operatorListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
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
