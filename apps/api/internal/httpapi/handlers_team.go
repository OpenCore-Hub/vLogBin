package httpapi

import (
	"net/http"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type inviteTeamMemberRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// inviteTeamMember — POST /v1/team-members
func (s *Server) inviteTeamMember(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req inviteTeamMemberRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.svc.InviteTeamMember(r.Context(), tc, req.Email, req.DisplayName, req.Role, tc.CredentialID.String())
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// listTeamMembers — GET /v1/team-members
func (s *Server) listTeamMembers(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	members, err := s.svc.ListTeamMembers(r.Context(), tc)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	if members == nil {
		members = []storegen.TeamMember{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"team_members": members})
}

// getTeamMember — GET /v1/team-members/{id}
func (s *Server) getTeamMember(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	memberID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "member id must be a uuid", reqIDFromRequest(r))
		return
	}
	member, err := s.svc.GetTeamMember(r.Context(), tc, memberID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, member)
}

type updateTeamMemberRoleRequest struct {
	Role string `json:"role"`
}

// updateTeamMemberRole — PATCH /v1/team-members/{id}
func (s *Server) updateTeamMemberRole(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	memberID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "member id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req updateTeamMemberRoleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	member, err := s.svc.UpdateTeamMemberRole(r.Context(), tc, memberID, req.Role)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, member)
}

// suspendTeamMember — POST /v1/team-members/{id}/suspend
func (s *Server) suspendTeamMember(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	memberID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "member id must be a uuid", reqIDFromRequest(r))
		return
	}
	member, err := s.svc.SuspendTeamMember(r.Context(), tc, memberID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, member)
}

// reactivateTeamMember — POST /v1/team-members/{id}/reactivate
func (s *Server) reactivateTeamMember(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	memberID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "member id must be a uuid", reqIDFromRequest(r))
		return
	}
	result, err := s.svc.ReactivateTeamMember(r.Context(), tc, memberID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// removeTeamMember — DELETE /v1/team-members/{id}
func (s *Server) removeTeamMember(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	memberID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "member id must be a uuid", reqIDFromRequest(r))
		return
	}
	if _, err := s.svc.RemoveTeamMember(r.Context(), tc, memberID); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
