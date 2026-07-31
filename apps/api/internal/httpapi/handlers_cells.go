package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type createCellRequest struct {
	RegionID       string         `json:"region_id"`
	Code           string         `json:"code"`
	CellType       string         `json:"cell_type"`
	Status         string         `json:"status"`
	CapacityLimits map[string]any `json:"capacity_limits"`
}

// createCell — POST /v1/operator/cells
func (s *Server) createCell(w http.ResponseWriter, r *http.Request) {
	var req createCellRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	regionID, err := uuid.Parse(req.RegionID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_region_id", "region_id must be a uuid", reqIDFromRequest(r))
		return
	}
	cell, err := s.svc.CreateCell(r.Context(), service.CreateCellInput{
		RegionID:       regionID,
		Code:           req.Code,
		CellType:       req.CellType,
		Status:         req.Status,
		CapacityLimits: req.CapacityLimits,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, cell)
}

// getCell — GET /v1/operator/cells/{id}
func (s *Server) getCell(w http.ResponseWriter, r *http.Request) {
	cellID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "cell id must be a uuid", reqIDFromRequest(r))
		return
	}
	cell, err := s.svc.GetCell(r.Context(), cellID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cell)
}

type updateCellStatusRequest struct {
	Status string `json:"status"`
}

// updateCellStatus — PATCH /v1/operator/cells/{id}
func (s *Server) updateCellStatus(w http.ResponseWriter, r *http.Request) {
	cellID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "cell id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req updateCellStatusRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	cell, err := s.svc.UpdateCellStatus(r.Context(), cellID, req.Status)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cell)
}

type assignProviderCellRequest struct {
	CellID string `json:"cell_id"`
}

// assignProviderCell — POST /v1/operator/providers/{id}/cell
func (s *Server) assignProviderCell(w http.ResponseWriter, r *http.Request) {
	providerID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "provider id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req assignProviderCellRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	cellID, err := uuid.Parse(req.CellID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cell_id", "cell_id must be a uuid", reqIDFromRequest(r))
		return
	}
	if err := s.svc.AssignProviderCell(r.Context(), providerID, cellID); err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assigned": true})
}

// getProviderCell — GET /v1/cell
func (s *Server) getProviderCell(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	cell, err := s.svc.GetProviderCell(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cell)
}
