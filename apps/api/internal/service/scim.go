package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SCIMUserInput is the SCIM 2.0 user representation for create/update.
type SCIMUserInput struct {
	ExternalID  string `json:"external_id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Active      bool   `json:"active"`
}

// SCIMUserResult is the SCIM 2.0 user response with a linked customer ID.
type SCIMUserResult struct {
	storegen.ScimUser
}

// CreateSCIMUser creates a new SCIM user. If the external_id already exists,
// returns the existing user (idempotent per SCIM spec).
func (s *Service) CreateSCIMUser(ctx context.Context, tc tenant.Ctx, in SCIMUserInput) (*storegen.ScimUser, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.ExternalID == "" {
		return nil, fmt.Errorf("%w: external_id is required", ErrValidation)
	}
	if in.DisplayName == "" {
		return nil, fmt.Errorf("%w: display_name is required", ErrValidation)
	}
	if in.Email == "" {
		return nil, fmt.Errorf("%w: email is required", ErrValidation)
	}

	var user storegen.ScimUser
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Idempotency: check if user already exists.
		existing, err := q.GetSCIMUserByExternalID(ctx, storegen.GetSCIMUserByExternalIDParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ExternalID: in.ExternalID,
		})
		if err == nil {
			user = existing
			return nil
		}

		u, err := q.CreateSCIMUser(ctx, storegen.CreateSCIMUserParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
			ExternalID:    in.ExternalID,
			DisplayName:   in.DisplayName,
			Email:         in.Email,
			Active:        in.Active,
		})
		if err != nil {
			return mapErr(err, "scim user %q", in.ExternalID)
		}

		// Link to existing customer if one exists with the same external_id.
		customer, err := q.GetCustomerByExternalID(ctx, storegen.GetCustomerByExternalIDParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ExternalID: in.ExternalID,
		})
		if err == nil && customer.ID != uuid.Nil {
			_ = q.SetSCIMUserCustomer(ctx, storegen.SetSCIMUserCustomerParams{
				ID: u.ID, CustomerID: uuid.NullUUID{UUID: customer.ID, Valid: true},
			})
			u.CustomerID = uuid.NullUUID{UUID: customer.ID, Valid: true}
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "scim_user", u.ID.String(), "scim.user_created", map[string]any{
			"external_id": in.ExternalID, "email": in.Email,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "scim.user_create",
			"scim_user", u.ID.String(),
			map[string]any{"external_id": in.ExternalID, "email": in.Email}); err != nil {
			return err
		}
		user = u
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetSCIMUser returns a SCIM user by ID.
func (s *Service) GetSCIMUser(ctx context.Context, tc tenant.Ctx, userID uuid.UUID) (*storegen.ScimUser, error) {
	var user storegen.ScimUser
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		u, err := q.GetSCIMUserByID(ctx, userID)
		if err != nil {
			return mapErr(err, "scim user %s", userID)
		}
		if err := checkTenantOwnership(u.ProviderID, u.EnvironmentID, tc, "scim user", userID); err != nil {
			return err
		}
		user = u
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// SCIMActiveFilter controls SCIM user listing by active status.
// Replaces raw int32 primitive with a typed enum (eliminates primitive obsession).
type SCIMActiveFilter int32

const (
	SCIMFilterInactive SCIMActiveFilter = 0 // inactive users only
	SCIMFilterActive   SCIMActiveFilter = 1 // active users only
	SCIMFilterAll      SCIMActiveFilter = 2 // all users (default)
)

// ListSCIMUsers returns a paginated list of SCIM users. Pass SCIMFilterAll
// for all users, SCIMFilterActive for active only, SCIMFilterInactive for
// inactive only.
func (s *Service) ListSCIMUsers(ctx context.Context, tc tenant.Ctx, activeFilter SCIMActiveFilter, limit int32) ([]storegen.ScimUser, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var users []storegen.ScimUser
	var total int64
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		u, err := q.ListSCIMUsers(ctx, storegen.ListSCIMUsersParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			Column3: int32(activeFilter), Limit: limit,
		})
		if err != nil {
			return err
		}
		c, err := q.CountSCIMUsers(ctx, storegen.CountSCIMUsersParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return err
		}
		users = u
		total = c
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// UpdateSCIMUser updates a SCIM user's attributes.
func (s *Service) UpdateSCIMUser(ctx context.Context, tc tenant.Ctx, userID uuid.UUID, in SCIMUserInput) (*storegen.ScimUser, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.DisplayName == "" {
		return nil, fmt.Errorf("%w: display_name is required", ErrValidation)
	}
	if in.Email == "" {
		return nil, fmt.Errorf("%w: email is required", ErrValidation)
	}

	var user storegen.ScimUser
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		existing, err := q.GetSCIMUserByID(ctx, userID)
		if err != nil {
			return mapErr(err, "scim user %s", userID)
		}
		if err := checkTenantOwnership(existing.ProviderID, existing.EnvironmentID, tc, "scim user", userID); err != nil {
			return err
		}
		u, err := q.UpdateSCIMUser(ctx, storegen.UpdateSCIMUserParams{
			ID:          userID,
			DisplayName: in.DisplayName,
			Email:       in.Email,
			Active:      in.Active,
		})
		if err != nil {
			return mapErr(err, "update scim user %s", userID)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "scim_user", userID.String(), "scim.user_updated", map[string]any{
			"external_id": existing.ExternalID, "active": in.Active,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "scim.user_update",
			"scim_user", userID.String(),
			map[string]any{"active": in.Active}); err != nil {
			return err
		}
		user = u
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// DeleteSCIMUser removes a SCIM user (SCIM DELETE operation).
func (s *Service) DeleteSCIMUser(ctx context.Context, tc tenant.Ctx, userID uuid.UUID) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := q.DeleteSCIMUser(ctx, storegen.DeleteSCIMUserParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ID: userID,
		})
		if err != nil {
			return mapErr(err, "delete scim user %s", userID)
		}
		if rows == 0 {
			return fmt.Errorf("%w: scim user %s", ErrNotFound, userID)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "scim_user", userID.String(), "scim.user_deleted", nil); err != nil {
			return err
		}
		return insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "scim.user_delete",
			"scim_user", userID.String(), nil)
	})
}

// SCIMPatchOperation represents a single SCIM PATCH operation.
type SCIMPatchOperation struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// SCIMPatchRequest is the SCIM 2.0 PATCH request body.
type SCIMPatchRequest struct {
	Operations []SCIMPatchOperation `json:"Operations"`
}

// PatchSCIMUser applies SCIM 2.0 PATCH operations to a user. Supports
// "replace" operations on displayName, email, and active fields.
func (s *Service) PatchSCIMUser(ctx context.Context, tc tenant.Ctx, userID uuid.UUID, patch SCIMPatchRequest) (*storegen.ScimUser, error) {
	var user storegen.ScimUser
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		existing, err := q.GetSCIMUserByID(ctx, userID)
		if err != nil {
			return mapErr(err, "scim user %s", userID)
		}
		if err := checkTenantOwnership(existing.ProviderID, existing.EnvironmentID, tc, "scim user", userID); err != nil {
			return err
		}

		// Apply each PATCH operation.
		for _, op := range patch.Operations {
			if op.Op != "replace" {
				continue // Only "replace" is supported.
			}
			switch op.Path {
			case "displayName":
				if v, ok := op.Value.(string); ok {
					if err := q.PatchSCIMUserDisplayName(ctx, storegen.PatchSCIMUserDisplayNameParams{ID: userID, DisplayName: v}); err != nil {
						return mapErr(err, "patch scim user displayName")
					}
				}
			case "email", "emails":
				if v, ok := op.Value.(string); ok {
					if err := q.PatchSCIMUserEmail(ctx, storegen.PatchSCIMUserEmailParams{ID: userID, Email: strings.ToLower(v)}); err != nil {
						return mapErr(err, "patch scim user email")
					}
				}
			case "active":
				if v, ok := op.Value.(bool); ok {
					if err := q.PatchSCIMUserActive(ctx, storegen.PatchSCIMUserActiveParams{ID: userID, Active: v}); err != nil {
						return mapErr(err, "patch scim user active")
					}
				}
			}
		}

		updated, err := q.GetSCIMUserByID(ctx, userID)
		if err != nil {
			return mapErr(err, "get patched scim user %s", userID)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "scim_user", userID.String(), "scim.user_patched", nil); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "scim.user_patch",
			"scim_user", userID.String(),
			map[string]any{"operations": len(patch.Operations)}); err != nil {
			return err
		}
		user = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// --- SCIM Groups ---

// CreateSCIMGroup creates a new SCIM group.
func (s *Service) CreateSCIMGroup(ctx context.Context, tc tenant.Ctx, externalID, displayName string) (*storegen.ScimGroup, error) {
	if externalID == "" {
		return nil, fmt.Errorf("%w: external_id is required", ErrValidation)
	}
	if displayName == "" {
		return nil, fmt.Errorf("%w: display_name is required", ErrValidation)
	}

	var group storegen.ScimGroup
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Idempotency: check if group already exists.
		existing, err := q.GetSCIMGroupByExternalID(ctx, storegen.GetSCIMGroupByExternalIDParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ExternalID: externalID,
		})
		if err == nil {
			group = existing
			return nil
		}

		g, err := q.CreateSCIMGroup(ctx, storegen.CreateSCIMGroupParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
			ExternalID:    externalID,
			DisplayName:   displayName,
		})
		if err != nil {
			return mapErr(err, "scim group %q", externalID)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "scim_group", g.ID.String(), "scim.group_created", map[string]any{
			"external_id": externalID,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "scim.group_create",
			"scim_group", g.ID.String(),
			map[string]any{"external_id": externalID}); err != nil {
			return err
		}
		group = g
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// GetSCIMGroup returns a SCIM group by ID.
func (s *Service) GetSCIMGroup(ctx context.Context, tc tenant.Ctx, groupID uuid.UUID) (*storegen.ScimGroup, error) {
	var group storegen.ScimGroup
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		g, err := q.GetSCIMGroupByID(ctx, groupID)
		if err != nil {
			return mapErr(err, "scim group %s", groupID)
		}
		if err := checkTenantOwnership(g.ProviderID, g.EnvironmentID, tc, "scim group", groupID); err != nil {
			return err
		}
		group = g
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// ListSCIMGroups returns a paginated list of SCIM groups.
func (s *Service) ListSCIMGroups(ctx context.Context, tc tenant.Ctx, limit int32) ([]storegen.ScimGroup, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var groups []storegen.ScimGroup
	var total int64
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		gs, err := q.ListSCIMGroups(ctx, storegen.ListSCIMGroupsParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Limit: limit,
		})
		if err != nil {
			return err
		}
		c, err := q.CountSCIMGroups(ctx, storegen.CountSCIMGroupsParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return err
		}
		groups = gs
		total = c
		return nil
	})
	return groups, total, err
}

// DeleteSCIMGroup removes a SCIM group (cascade deletes memberships).
func (s *Service) DeleteSCIMGroup(ctx context.Context, tc tenant.Ctx, groupID uuid.UUID) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := q.DeleteSCIMGroup(ctx, storegen.DeleteSCIMGroupParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ID: groupID,
		})
		if err != nil {
			return mapErr(err, "delete scim group %s", groupID)
		}
		if rows == 0 {
			return fmt.Errorf("%w: scim group %s", ErrNotFound, groupID)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "scim_group", groupID.String(), "scim.group_deleted", nil); err != nil {
			return err
		}
		return insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "scim.group_delete",
			"scim_group", groupID.String(), nil)
	})
}

// PatchSCIMGroup applies SCIM 2.0 PATCH operations to a group. Supports:
// - "replace" displayName
// - "add" members (value = [{value: "user-uuid"}, ...])
// - "remove" members (path = "members[value eq \"user-uuid\"]")
func (s *Service) PatchSCIMGroup(ctx context.Context, tc tenant.Ctx, groupID uuid.UUID, patch SCIMPatchRequest) (*storegen.ScimGroup, error) {
	var group storegen.ScimGroup
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		existing, err := q.GetSCIMGroupByID(ctx, groupID)
		if err != nil {
			return mapErr(err, "scim group %s", groupID)
		}
		if err := checkTenantOwnership(existing.ProviderID, existing.EnvironmentID, tc, "scim group", groupID); err != nil {
			return err
		}

		for _, op := range patch.Operations {
			switch op.Op {
			case "replace":
				if op.Path == "displayName" {
					if v, ok := op.Value.(string); ok {
						if err := q.PatchSCIMGroupDisplayName(ctx, storegen.PatchSCIMGroupDisplayNameParams{
							ID: groupID, DisplayName: v,
						}); err != nil {
							return mapErr(err, "patch scim group displayName")
						}
					}
				}
			case "add":
				if op.Path == "members" {
					members, ok := op.Value.([]any)
					if !ok {
						continue
					}
					for _, m := range members {
						memberMap, ok := m.(map[string]any)
						if !ok {
							continue
						}
						userIDStr, ok := memberMap["value"].(string)
						if !ok {
							continue
						}
						userID, err := uuid.Parse(userIDStr)
						if err != nil {
							continue
						}
						_, _ = q.AddSCIMGroupMember(ctx, storegen.AddSCIMGroupMemberParams{
							GroupID: groupID, UserID: userID,
						})
					}
				}
			case "remove":
				// Parse "members[value eq \"user-uuid\"]" or just "members"
				pathStr := op.Path
				if strings.Contains(pathStr, "value eq") {
					// Extract user ID from filter expression.
					start := strings.Index(pathStr, "\"")
					end := strings.LastIndex(pathStr, "\"")
					if start >= 0 && end > start {
						userIDStr := pathStr[start+1 : end]
						userID, err := uuid.Parse(userIDStr)
						if err == nil {
							_, _ = q.RemoveSCIMGroupMember(ctx, storegen.RemoveSCIMGroupMemberParams{
								GroupID: groupID, UserID: userID,
							})
						}
					}
				}
			}
		}

		updated, err := q.GetSCIMGroupByID(ctx, groupID)
		if err != nil {
			return mapErr(err, "get patched scim group %s", groupID)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "scim_group", groupID.String(), "scim.group_patched", nil); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "scim.group_patch",
			"scim_group", groupID.String(),
			map[string]any{"operations": len(patch.Operations)}); err != nil {
			return err
		}
		group = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &group, nil
}
