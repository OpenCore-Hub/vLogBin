package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type initiateFailoverRequest struct {
	ProviderID  string `json:"provider_id"`
	FromCellID  string `json:"from_cell_id"`
	ToCellID    string `json:"to_cell_id"`
	Reason      string `json:"reason"`
	InitiatedBy string `json:"initiated_by"`
}

// initiateFailover — POST /v1/operator/failovers
func (s *Server) initiateFailover(w http.ResponseWriter, r *http.Request) {
	var req initiateFailoverRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	providerID, err := uuid.Parse(req.ProviderID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_provider_id", "provider_id must be a uuid", reqIDFromRequest(r))
		return
	}
	fromCellID, err := uuid.Parse(req.FromCellID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_from_cell_id", "from_cell_id must be a uuid", reqIDFromRequest(r))
		return
	}
	toCellID, err := uuid.Parse(req.ToCellID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_to_cell_id", "to_cell_id must be a uuid", reqIDFromRequest(r))
		return
	}
	failover, err := s.svc.InitiateFailover(r.Context(), service.FailoverInput{
		ProviderID:  providerID,
		FromCellID:  fromCellID,
		ToCellID:    toCellID,
		Reason:      req.Reason,
		InitiatedBy: req.InitiatedBy,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, failover)
}

// getFailover — GET /v1/operator/failovers/{id}
func (s *Server) getFailover(w http.ResponseWriter, r *http.Request) {
	failoverID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "failover id must be a uuid", reqIDFromRequest(r))
		return
	}
	failover, err := s.svc.GetFailover(r.Context(), failoverID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, failover)
}

// listFailovers — GET /v1/operator/failovers?provider_id=<uuid>
func (s *Server) listFailovers(w http.ResponseWriter, r *http.Request) {
	providerIDStr := r.URL.Query().Get("provider_id")
	if providerIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing_provider_id", "provider_id query parameter is required", reqIDFromRequest(r))
		return
	}
	providerID, err := uuid.Parse(providerIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_provider_id", "provider_id must be a uuid", reqIDFromRequest(r))
		return
	}
	failovers, err := s.svc.ListFailovers(r.Context(), providerID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if failovers == nil {
		failovers = []storegen.CellFailover{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"failovers": failovers})
}

// fenceFailover — POST /v1/operator/failovers/{id}/fence
func (s *Server) fenceFailover(w http.ResponseWriter, r *http.Request) {
	failoverID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "failover id must be a uuid", reqIDFromRequest(r))
		return
	}
	failover, err := s.svc.FenceFailover(r.Context(), failoverID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, failover)
}

// switchFailover — POST /v1/operator/failovers/{id}/switch
func (s *Server) switchFailover(w http.ResponseWriter, r *http.Request) {
	failoverID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "failover id must be a uuid", reqIDFromRequest(r))
		return
	}
	failover, err := s.svc.SwitchFailover(r.Context(), failoverID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, failover)
}

// completeFailover — POST /v1/operator/failovers/{id}/complete
func (s *Server) completeFailover(w http.ResponseWriter, r *http.Request) {
	failoverID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "failover id must be a uuid", reqIDFromRequest(r))
		return
	}
	failover, err := s.svc.CompleteFailover(r.Context(), failoverID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, failover)
}

// abortFailover — POST /v1/operator/failovers/{id}/abort
func (s *Server) abortFailover(w http.ResponseWriter, r *http.Request) {
	failoverID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "failover id must be a uuid", reqIDFromRequest(r))
		return
	}
	failover, err := s.svc.AbortFailover(r.Context(), failoverID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, failover)
}
