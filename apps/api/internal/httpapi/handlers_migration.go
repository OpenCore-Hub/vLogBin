package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type createMigrationJobRequest struct {
	SourceSystem string `json:"source_system"`
	DryRun       bool   `json:"dry_run"`
}

// createMigrationJob — POST /v1/migrations
func (s *Server) createMigrationJob(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req createMigrationJobRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	job, err := s.svc.CreateMigrationJob(r.Context(), tc, service.CreateMigrationJobInput{
		SourceSystem: req.SourceSystem,
		DryRun:       req.DryRun,
		CreatedBy:    tc.CredentialID.String(),
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, job)
}

// listMigrationJobs — GET /v1/migrations
func (s *Server) listMigrationJobs(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	jobs, err := s.svc.ListMigrationJobs(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if jobs == nil {
		jobs = []storegen.MigrationJob{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"migration_jobs": jobs})
}

// getMigrationJob — GET /v1/migrations/{id}
func (s *Server) getMigrationJob(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "migration job id must be a uuid", reqIDFromRequest(r))
		return
	}
	job, err := s.svc.GetMigrationJob(r.Context(), tc, jobID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

type addMigrationRecordsRequest struct {
	Records []service.MigrationRecordInput `json:"records"`
}

// addMigrationRecords — POST /v1/migrations/{id}/records
func (s *Server) addMigrationRecords(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "migration job id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req addMigrationRecordsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	added, err := s.svc.AddMigrationRecords(r.Context(), tc, jobID, req.Records)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": added})
}

// validateMigrationJob — POST /v1/migrations/{id}/validate
func (s *Server) validateMigrationJob(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "migration job id must be a uuid", reqIDFromRequest(r))
		return
	}
	job, err := s.svc.ValidateMigrationJob(r.Context(), tc, jobID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// startMigration — POST /v1/migrations/{id}/start
func (s *Server) startMigration(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "migration job id must be a uuid", reqIDFromRequest(r))
		return
	}
	job, err := s.svc.StartMigration(r.Context(), tc, jobID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// completeMigration — POST /v1/migrations/{id}/complete
func (s *Server) completeMigration(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "migration job id must be a uuid", reqIDFromRequest(r))
		return
	}
	job, err := s.svc.CompleteMigration(r.Context(), tc, jobID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// rollbackMigration — POST /v1/migrations/{id}/rollback
func (s *Server) rollbackMigration(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "migration job id must be a uuid", reqIDFromRequest(r))
		return
	}
	job, err := s.svc.RollbackMigration(r.Context(), tc, jobID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// listMigrationRecords — GET /v1/migrations/{id}/records
func (s *Server) listMigrationRecords(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "migration job id must be a uuid", reqIDFromRequest(r))
		return
	}
	records, err := s.svc.ListMigrationRecords(r.Context(), tc, jobID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if records == nil {
		records = []storegen.MigrationRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"migration_records": records})
}

// listInvalidRecords — GET /v1/migrations/{id}/invalid-records
func (s *Server) listInvalidRecords(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	jobID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "migration job id must be a uuid", reqIDFromRequest(r))
		return
	}
	records, err := s.svc.ListInvalidRecords(r.Context(), tc, jobID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if records == nil {
		records = []storegen.MigrationRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"invalid_records": records})
}
