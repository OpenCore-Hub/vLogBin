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

// ---- Operator routes (operator token) ----

type requestSupportSessionRequest struct {
	EnvironmentID   string   `json:"environment_id"`
	AccessType      string   `json:"access_type"`
	Reason          string   `json:"reason"`
	RequestedScopes []string `json:"requested_scopes"`
	DurationSeconds int      `json:"duration_seconds"`
}

// requestSupportSession — POST /v1/operator/providers/{id}/support-sessions
func (s *Server) requestSupportSession(w http.ResponseWriter, r *http.Request) {
	providerID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "provider id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req requestSupportSessionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	envID, err := uuid.Parse(req.EnvironmentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_environment_id", "environment_id must be a uuid", reqIDFromRequest(r))
		return
	}
	if req.DurationSeconds <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_duration", "duration_seconds must be positive", reqIDFromRequest(r))
		return
	}
	session, err := s.svc.RequestSupportSession(r.Context(), service.RequestSupportSessionInput{
		ProviderID:      providerID,
		EnvironmentID:   envID,
		AccessType:      req.AccessType,
		RequestedBy:     operatorIdentity(r),
		Reason:          req.Reason,
		RequestedScopes: req.RequestedScopes,
		Duration:        time.Duration(req.DurationSeconds) * time.Second,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

// firstApproveEmergency — POST /v1/operator/support-sessions/{sessionId}/first-approve
func (s *Server) firstApproveEmergency(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "session id must be a uuid", reqIDFromRequest(r))
		return
	}
	session, err := s.svc.EmergencyFirstApprove(r.Context(), sessionID, operatorIdentity(r))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// secondApproveEmergency — POST /v1/operator/support-sessions/{sessionId}/second-approve
func (s *Server) secondApproveEmergency(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "session id must be a uuid", reqIDFromRequest(r))
		return
	}
	session, err := s.svc.EmergencySecondApprove(r.Context(), sessionID, operatorIdentity(r))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

type revokeSupportSessionRequest struct {
	Reason string `json:"reason"`
}

// operatorRevokeSupportSession — POST /v1/operator/support-sessions/{sessionId}/revoke
func (s *Server) operatorRevokeSupportSession(w http.ResponseWriter, r *http.Request) {
	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "session id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req revokeSupportSessionRequest
	_ = decodeJSON(w, r, &req) // body is optional; reason may be empty
	session, err := s.svc.RevokeSupportSession(r.Context(), sessionID, operatorIdentity(r), req.Reason)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// operatorListSupportSessions — GET /v1/operator/providers/{id}/support-sessions
func (s *Server) operatorListSupportSessions(w http.ResponseWriter, r *http.Request) {
	providerID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "provider id must be a uuid", reqIDFromRequest(r))
		return
	}
	sessions, err := s.svc.ListSupportSessionsByProvider(r.Context(), providerID, 100)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if sessions == nil {
		sessions = []storegen.SupportSession{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"support_sessions": sessions})
}

// operatorListAllSupportSessions — GET /v1/operator/support-sessions
func (s *Server) operatorListAllSupportSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.svc.ListAllSupportSessions(r.Context(), queryLimit(r, 500))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if sessions == nil {
		sessions = []storegen.SupportSession{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"support_sessions": sessions})
}

// ---- Provider routes (API key with support:approve scope) ----

// providerApproveSupportSession — POST /v1/support-sessions/{id}/approve
func (s *Server) providerApproveSupportSession(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "session id must be a uuid", reqIDFromRequest(r))
		return
	}
	session, err := s.svc.ApproveSupportSession(r.Context(), tc, sessionID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// providerDenySupportSession — POST /v1/support-sessions/{id}/deny
func (s *Server) providerDenySupportSession(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "session id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req revokeSupportSessionRequest
	_ = decodeJSON(w, r, &req) // body is optional
	session, err := s.svc.DenySupportSession(r.Context(), tc, sessionID, req.Reason)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// providerRevokeSupportSession — POST /v1/support-sessions/{id}/revoke
func (s *Server) providerRevokeSupportSession(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	sessionID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "session id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req revokeSupportSessionRequest
	_ = decodeJSON(w, r, &req) // body is optional
	session, err := s.svc.RevokeSupportSessionAsProvider(r.Context(), tc, sessionID, req.Reason)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

// providerListSupportSessions — GET /v1/support-sessions
func (s *Server) providerListSupportSessions(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	sessions, err := s.svc.ListSupportSessions(r.Context(), tc, 100)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if sessions == nil {
		sessions = []storegen.SupportSession{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"support_sessions": sessions})
}

// providerListActiveSupportSessions — GET /v1/support-sessions/active
func (s *Server) providerListActiveSupportSessions(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	sessions, err := s.svc.ListActiveSupportSessions(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if sessions == nil {
		sessions = []storegen.SupportSession{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"support_sessions": sessions})
}
