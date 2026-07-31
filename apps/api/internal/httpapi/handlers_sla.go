package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type createSLATierRequest struct {
	Code             string         `json:"code"`
	Name             string         `json:"name"`
	UptimeSLA        float64        `json:"uptime_sla"`
	PriorityLevel    int32          `json:"priority_level"`
	ReservedCapacity map[string]any `json:"reserved_capacity"`
}

func (s *Server) createSLATier(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req createSLATierRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	tier, err := s.svc.CreateSLATier(r.Context(), tc, service.SLATierInput{
		Code: req.Code, Name: req.Name, UptimeSLA: req.UptimeSLA,
		PriorityLevel: req.PriorityLevel, ReservedCapacity: req.ReservedCapacity,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, tier)
}

func (s *Server) getSLATier(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	tierID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "tier id must be a uuid", reqIDFromRequest(r))
		return
	}
	tier, err := s.svc.GetSLATier(r.Context(), tc, tierID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tier)
}

func (s *Server) listSLATiers(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	tiers, err := s.svc.ListSLATiers(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if tiers == nil {
		tiers = []storegen.SlaTier{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sla_tiers": tiers})
}

func (s *Server) updateSLATier(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	tierID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "tier id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req createSLATierRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	tier, err := s.svc.UpdateSLATier(r.Context(), tc, tierID, service.SLATierInput{
		Code: req.Code, Name: req.Name, UptimeSLA: req.UptimeSLA,
		PriorityLevel: req.PriorityLevel, ReservedCapacity: req.ReservedCapacity,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tier)
}

func (s *Server) deleteSLATier(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	tierID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "tier id must be a uuid", reqIDFromRequest(r))
		return
	}
	if err := s.svc.DeleteSLATier(r.Context(), tc, tierID); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
