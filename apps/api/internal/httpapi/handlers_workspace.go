package httpapi

import (
	"net/http"
	"strings"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// signupRequest carries the optional identity hints a platform user can
// attach to their default workspace at signup (design baseline §3.1 R11).
type signupRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// signup provisions the caller's default workspace and grants the first
// user provider_admin. Idempotent: returning users get their existing
// workspace back. One of the three R11 steps (create ZITADEL user → create
// default workspace → grant provider_admin) failing rolls back the whole
// signup at the service layer.
func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := s.svc.ProvisionWorkspace(r.Context(), operatorIdentity(r), req.Email, req.Name)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace":  result.Workspace,
		"membership": result.Membership,
		"provider":   result.Provider,
	})
}

// meWorkspaces lists the workspaces the authenticated platform user belongs
// to (used by the console to pick the active workspace).
func (s *Server) meWorkspaces(w http.ResponseWriter, r *http.Request) {
	rows, err := s.svc.ListMyWorkspaces(r.Context(), operatorIdentity(r))
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": rows})
}

// updateWorkspaceRequest carries the optional fields a provider_admin can
// change on their workspace.
type updateWorkspaceRequest struct {
	Name *string `json:"name"`
	Slug *string `json:"slug"`
}

// inviteWorkspaceMemberRequest carries the subject and role of the invited
// platform user.
type inviteWorkspaceMemberRequest struct {
	UserSub string `json:"user_sub"`
	Role    string `json:"role"`
}

// updateWorkspaceMemberRoleRequest carries the new role for a member.
type updateWorkspaceMemberRoleRequest struct {
	Role string `json:"role"`
}

// getWorkspace returns one workspace the caller is an active member of.
func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request) {
	id, ok := workspaceIDParam(w, r)
	if !ok {
		return
	}
	ws, err := s.svc.GetWorkspace(r.Context(), operatorIdentity(r), id)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace": ws})
}

// updateWorkspace updates the workspace name and/or slug. provider_admin only.
func (s *Server) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	id, ok := workspaceIDParam(w, r)
	if !ok {
		return
	}
	var req updateWorkspaceRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ws, err := s.svc.UpdateWorkspace(r.Context(), operatorIdentity(r), id, service.UpdateWorkspaceInput{
		Name: req.Name,
		Slug: req.Slug,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace": ws})
}

// listWorkspaceMembers returns the workspace member list. Any active member
// can read it.
func (s *Server) listWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := workspaceIDParam(w, r)
	if !ok {
		return
	}
	members, err := s.svc.ListWorkspaceMembers(r.Context(), operatorIdentity(r), id)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

// inviteWorkspaceMember adds or re-activates a member. provider_admin only.
func (s *Server) inviteWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	id, ok := workspaceIDParam(w, r)
	if !ok {
		return
	}
	var req inviteWorkspaceMemberRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	m, err := s.svc.InviteWorkspaceMember(r.Context(), operatorIdentity(r), id, req.UserSub, req.Role)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"member": m})
}

// updateWorkspaceMemberRole changes a member's role. provider_admin only.
func (s *Server) updateWorkspaceMemberRole(w http.ResponseWriter, r *http.Request) {
	id, ok := workspaceIDParam(w, r)
	if !ok {
		return
	}
	userSub, ok := memberSubParam(w, r)
	if !ok {
		return
	}
	var req updateWorkspaceMemberRoleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	m, err := s.svc.UpdateWorkspaceMemberRole(r.Context(), operatorIdentity(r), id, userSub, req.Role)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"member": m})
}

// removeWorkspaceMember removes a member from the workspace. provider_admin
// only; the last active provider_admin can never be removed.
func (s *Server) removeWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	id, ok := workspaceIDParam(w, r)
	if !ok {
		return
	}
	userSub, ok := memberSubParam(w, r)
	if !ok {
		return
	}
	if err := s.svc.RemoveWorkspaceMember(r.Context(), operatorIdentity(r), id, userSub); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// workspaceIDParam parses the {id} path parameter as a UUID.
func workspaceIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "workspace id must be a uuid", reqIDFromRequest(r))
		return uuid.Nil, false
	}
	return id, true
}

// memberSubParam returns the {userSub} path parameter.
func memberSubParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	sub := strings.TrimSpace(chi.URLParam(r, "userSub"))
	if sub == "" {
		writeError(w, http.StatusBadRequest, "invalid_user_sub", "member user_sub must not be empty", reqIDFromRequest(r))
		return "", false
	}
	return sub, true
}
