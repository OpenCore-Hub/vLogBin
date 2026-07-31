package httpapi

import (
	"net/http"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type createCellMigrationRequest struct {
	ProviderID  string `json:"provider_id"`
	FromCellID  string `json:"from_cell_id"`
	ToCellID    string `json:"to_cell_id"`
	Reason      string `json:"reason"`
	InitiatedBy string `json:"initiated_by"`
	ScheduledAt string `json:"scheduled_at"`
}

// createCellMigration — POST /v1/operator/cell-migrations
func (s *Server) createCellMigration(w http.ResponseWriter, r *http.Request) {
	var req createCellMigrationRequest
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
	var scheduledAt *time.Time
	if req.ScheduledAt != "" {
		t, err := time.Parse(time.RFC3339, req.ScheduledAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_scheduled_at", "scheduled_at must be RFC3339", reqIDFromRequest(r))
			return
		}
		scheduledAt = &t
	}
	migration, err := s.svc.CreateCellMigration(r.Context(), service.CellMigrationInput{
		ProviderID:  providerID,
		FromCellID:  fromCellID,
		ToCellID:    toCellID,
		Reason:      req.Reason,
		InitiatedBy: req.InitiatedBy,
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, migration)
}

// getCellMigration — GET /v1/operator/cell-migrations/{id}
func (s *Server) getCellMigration(w http.ResponseWriter, r *http.Request) {
	migrationID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "migration id must be a uuid", reqIDFromRequest(r))
		return
	}
	migration, err := s.svc.GetCellMigration(r.Context(), migrationID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, migration)
}

// listCellMigrations — GET /v1/operator/cell-migrations?provider_id=<uuid>
func (s *Server) listCellMigrations(w http.ResponseWriter, r *http.Request) {
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
	migrations, err := s.svc.ListCellMigrations(r.Context(), providerID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if migrations == nil {
		migrations = []storegen.CellMigration{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"cell_migrations": migrations})
}

// precheckMigration — POST /v1/operator/cell-migrations/{id}/precheck
func (s *Server) precheckMigration(w http.ResponseWriter, r *http.Request) {
	migrationID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "migration id must be a uuid", reqIDFromRequest(r))
		return
	}
	migration, err := s.svc.PrecheckMigration(r.Context(), migrationID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, migration)
}

// executeMigration — POST /v1/operator/cell-migrations/{id}/execute
func (s *Server) executeMigration(w http.ResponseWriter, r *http.Request) {
	migrationID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "migration id must be a uuid", reqIDFromRequest(r))
		return
	}
	migration, err := s.svc.ExecuteMigration(r.Context(), migrationID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, migration)
}

// cancelMigration — POST /v1/operator/cell-migrations/{id}/cancel
func (s *Server) cancelMigration(w http.ResponseWriter, r *http.Request) {
	migrationID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "migration id must be a uuid", reqIDFromRequest(r))
		return
	}
	migration, err := s.svc.CancelMigration(r.Context(), migrationID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, migration)
}
