package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// structToMap converts a struct to a map[string]any via JSON round-trip.
func structToMap(v any) map[string]any {
	data, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]any{}
	}
	return m
}

const scimUserSchema = "urn:ietf:params:scim:schemas:core:2.0:User"
const scimListSchema = "urn:ietf:params:scim:api:messages:2.0:ListResponse"

type scimCreateUserRequest struct {
	ExternalID  string `json:"externalId"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	Active      bool   `json:"active"`
}

// scimUserResponse formats a SCIM user in the standard SCIM 2.0 format.
func scimUserResponse(u any) map[string]any {
	m := structToMap(u)
	return map[string]any{
		"schemas":    []string{scimUserSchema},
		"id":         m["id"],
		"externalId": m["external_id"],
		"userName":   m["email"],
		"name":       map[string]any{"displayName": m["display_name"]},
		"emails":     []map[string]any{{"value": m["email"], "primary": true}},
		"active":     m["active"],
		"meta": map[string]any{
			"resourceType": "User",
			"created":      m["created_at"],
			"lastModified": m["updated_at"],
		},
	}
}

func (s *Server) scimCreateUser(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req scimCreateUserRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := s.svc.CreateSCIMUser(r.Context(), tc, service.SCIMUserInput{
		ExternalID: req.ExternalID, DisplayName: req.DisplayName,
		Email: req.Email, Active: req.Active,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, scimUserResponse(user))
}

func (s *Server) scimGetUser(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	userID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "user id must be a uuid", reqIDFromRequest(r))
		return
	}
	user, err := s.svc.GetSCIMUser(r.Context(), tc, userID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, scimUserResponse(user))
}

func (s *Server) scimListUsers(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	activeFilter := service.SCIMFilterAll
	if a := r.URL.Query().Get("active"); a == "true" {
		activeFilter = service.SCIMFilterActive
	} else if a == "false" {
		activeFilter = service.SCIMFilterInactive
	}
	limit := int32(100)
	if l := r.URL.Query().Get("count"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = int32(n)
		}
	}
	users, total, err := s.svc.ListSCIMUsers(r.Context(), tc, activeFilter, limit)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	resources := make([]map[string]any, 0, len(users))
	for i := range users {
		resources = append(resources, scimUserResponse(&users[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schemas":      []string{scimListSchema},
		"totalResults": total,
		"Resources":    resources,
		"itemsPerPage": limit,
		"startIndex":   1,
	})
}

func (s *Server) scimUpdateUser(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	userID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "user id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req scimCreateUserRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := s.svc.UpdateSCIMUser(r.Context(), tc, userID, service.SCIMUserInput{
		ExternalID: req.ExternalID, DisplayName: req.DisplayName,
		Email: req.Email, Active: req.Active,
	})
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, scimUserResponse(user))
}

func (s *Server) scimDeleteUser(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	userID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "user id must be a uuid", reqIDFromRequest(r))
		return
	}
	if err := s.svc.DeleteSCIMUser(r.Context(), tc, userID); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// scimPatchUser — PATCH /scim/v2/Users/{id}
func (s *Server) scimPatchUser(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	userID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "user id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req service.SCIMPatchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := s.svc.PatchSCIMUser(r.Context(), tc, userID, req)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, scimUserResponse(user))
}

// --- SCIM Groups ---

const scimGroupSchema = "urn:ietf:params:scim:schemas:core:2.0:Group"

func scimGroupResponse(g any) map[string]any {
	m := structToMap(g)
	return map[string]any{
		"schemas":     []string{scimGroupSchema},
		"id":          m["id"],
		"externalId":  m["external_id"],
		"displayName": m["display_name"],
		"meta": map[string]any{
			"resourceType": "Group",
			"created":      m["created_at"],
			"lastModified": m["updated_at"],
		},
	}
}

type scimCreateGroupRequest struct {
	ExternalID  string `json:"externalId"`
	DisplayName string `json:"displayName"`
}

func (s *Server) scimCreateGroup(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	var req scimCreateGroupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	group, err := s.svc.CreateSCIMGroup(r.Context(), tc, req.ExternalID, req.DisplayName)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, scimGroupResponse(group))
}

func (s *Server) scimGetGroup(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	groupID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "group id must be a uuid", reqIDFromRequest(r))
		return
	}
	group, err := s.svc.GetSCIMGroup(r.Context(), tc, groupID)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, scimGroupResponse(group))
}

func (s *Server) scimListGroups(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	limit := int32(100)
	if l := r.URL.Query().Get("count"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = int32(n)
		}
	}
	groups, total, err := s.svc.ListSCIMGroups(r.Context(), tc, limit)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	resources := make([]map[string]any, 0, len(groups))
	for i := range groups {
		resources = append(resources, scimGroupResponse(&groups[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schemas":      []string{scimListSchema},
		"totalResults": total,
		"Resources":    resources,
		"itemsPerPage": limit,
		"startIndex":   1,
	})
}

func (s *Server) scimDeleteGroup(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	groupID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "group id must be a uuid", reqIDFromRequest(r))
		return
	}
	if err := s.svc.DeleteSCIMGroup(r.Context(), tc, groupID); err != nil {
		s.serviceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// scimPatchGroup — PATCH /scim/v2/Groups/{id}
func (s *Server) scimPatchGroup(w http.ResponseWriter, r *http.Request) {
	tc, _ := tenant.FromContext(r.Context())
	groupID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "group id must be a uuid", reqIDFromRequest(r))
		return
	}
	var req service.SCIMPatchRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	group, err := s.svc.PatchSCIMGroup(r.Context(), tc, groupID, req)
	if err != nil {
		s.serviceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, scimGroupResponse(group))
}
