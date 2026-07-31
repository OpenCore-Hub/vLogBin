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
