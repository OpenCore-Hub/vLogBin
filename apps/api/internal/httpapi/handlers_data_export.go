package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type requestDataExportRequest struct {
	ExportType string `json:"export_type"`
}

// requestDataExport — POST /v1/data-exports
func (s *Server) requestDataExport(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req requestDataExportRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ExportType == "" {
		req.ExportType = "full"
	}
	export, err := s.svc.RequestDataExport(r.Context(), tc, req.ExportType)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, export)
}

// listDataExports — GET /v1/data-exports
func (s *Server) listDataExports(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	exports, err := s.svc.ListDataExports(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if exports == nil {
		exports = []storegen.DataExport{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data_exports": exports})
}

// getDataExport — GET /v1/data-exports/{id}
func (s *Server) getDataExport(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	exportID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "export id must be a uuid", reqIDFromRequest(r))
		return
	}
	export, err := s.svc.GetDataExport(r.Context(), tc, exportID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, export)
}

type requestDeletionRequest struct {
	Reason string `json:"reason"`
}

// requestDeletion — POST /v1/data-deletion
func (s *Server) requestDeletion(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req requestDeletionRequest
	_ = decodeJSON(w, r, &req) // body is optional
	proof, err := s.svc.RequestDeletion(r.Context(), tc, req.Reason)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, proof)
}

// getDeletionProof — GET /v1/deletion-proofs/{id}
func (s *Server) getDeletionProof(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	proofID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "proof id must be a uuid", reqIDFromRequest(r))
		return
	}
	proof, err := s.svc.GetDeletionProof(r.Context(), tc, proofID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, proof)
}

// listDeletionProofs — GET /v1/deletion-proofs
func (s *Server) listDeletionProofs(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	proofs, err := s.svc.ListDeletionProofs(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if proofs == nil {
		proofs = []storegen.DeletionProof{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"deletion_proofs": proofs})
}
