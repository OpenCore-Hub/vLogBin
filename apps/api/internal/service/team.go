package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/keys"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// InvitedTeamMember is the result of inviting a team member: the member
// record plus the plaintext API key (shown exactly once).
type InvitedTeamMember struct {
	Member storegen.TeamMember `json:"member"`
	APIKey string              `json:"api_key"`
}

// InviteTeamMember creates a new team member with a role-based API key.
// The API key is returned exactly once. The caller must hold the
// credentials:manage scope (enforced by route middleware).
func (s *Service) InviteTeamMember(ctx context.Context, tc tenant.Ctx, email, displayName, role, invitedBy string) (*InvitedTeamMember, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, fmt.Errorf("%w: email is required", ErrValidation)
	}
	if displayName == "" {
		return nil, fmt.Errorf("%w: display_name is required", ErrValidation)
	}
	if !domain.ValidTeamRole(role) {
		return nil, fmt.Errorf("%w: role must be admin, billing_admin, developer or support_agent", ErrValidation)
	}

	scopes := domain.RoleScopes(role)
	// Scope attenuation: the inviter's scopes must cover the role's scopes.
	for _, sc := range scopes {
		if !tc.HasScope(sc) {
			return nil, fmt.Errorf("%w: role %q requires scope %q which the caller does not hold", ErrValidation, role, sc)
		}
	}

	var out InvitedTeamMember
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		member, err := q.CreateTeamMember(ctx, storegen.CreateTeamMemberParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
			Email:         email,
			DisplayName:   displayName,
			Role:          role,
			Status:        domain.TeamStatusActive,
			InvitedBy:     invitedBy,
		})
		if err != nil {
			return mapErr(err, "team member %q", email)
		}

		// Generate API key with role-based scopes.
		plaintext, err := keys.Generate(tc.EnvironmentKind)
		if err != nil {
			return err
		}
		cred, err := q.CreateCredential(ctx, storegen.CreateCredentialParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
			Name:          displayName + " [" + role + "]",
			KeyPrefix:     keys.Prefix(plaintext),
			KeyHash:       keys.Hash(plaintext),
			Scopes:        scopes,
		})
		if err != nil {
			return mapErr(err, "create credential for team member")
		}

		// Link the credential to the team member.
		if err := q.SetTeamMemberCredential(ctx, storegen.SetTeamMemberCredentialParams{
			ID:           member.ID,
			CredentialID: uuid.NullUUID{UUID: cred.ID, Valid: true},
		}); err != nil {
			return mapErr(err, "link credential to team member")
		}

		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "team_member", member.ID.String(), "team.member_invited", map[string]any{
			"email": email, "role": role, "display_name": displayName,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "team.member_invite",
			"team_member", member.ID.String(),
			map[string]any{"email": email, "role": role}); err != nil {
			return err
		}
		out.Member = member
		out.APIKey = plaintext
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListTeamMembers returns all team members (including suspended/removed)
// for the caller's tenant.
func (s *Service) ListTeamMembers(ctx context.Context, tc tenant.Ctx) ([]storegen.TeamMember, error) {
	var out []storegen.TeamMember
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		members, err := q.ListTeamMembers(ctx, storegen.ListTeamMembersParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		out = members
		return err
	})
	return out, err
}

// ListActiveTeamMembers returns only active team members.
func (s *Service) ListActiveTeamMembers(ctx context.Context, tc tenant.Ctx) ([]storegen.TeamMember, error) {
	var out []storegen.TeamMember
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		members, err := q.ListActiveTeamMembers(ctx, storegen.ListActiveTeamMembersParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		out = members
		return err
	})
	return out, err
}

// GetTeamMember returns a single team member by ID.
func (s *Service) GetTeamMember(ctx context.Context, tc tenant.Ctx, memberID uuid.UUID) (*storegen.TeamMember, error) {
	var out storegen.TeamMember
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		m, err := q.GetTeamMemberByID(ctx, memberID)
		if err != nil {
			return mapErr(err, "team member %s", memberID)
		}
		if err := checkTenantOwnership(m.ProviderID, m.EnvironmentID, tc, "team member", memberID); err != nil {
			return err
		}
		out = m
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateTeamMemberRole changes the role of an active team member and
// updates the linked credential's scopes to match the new role.
func (s *Service) UpdateTeamMemberRole(ctx context.Context, tc tenant.Ctx, memberID uuid.UUID, newRole string) (*storegen.TeamMember, error) {
	if !domain.ValidTeamRole(newRole) {
		return nil, fmt.Errorf("%w: role must be admin, billing_admin, developer or support_agent", ErrValidation)
	}

	newScopes := domain.RoleScopes(newRole)
	for _, sc := range newScopes {
		if !tc.HasScope(sc) {
			return nil, fmt.Errorf("%w: role %q requires scope %q which the caller does not hold", ErrValidation, newRole, sc)
		}
	}

	var out storegen.TeamMember
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		m, err := q.GetTeamMemberByID(ctx, memberID)
		if err != nil {
			return mapErr(err, "team member %s", memberID)
		}
		if err := checkTenantOwnership(m.ProviderID, m.EnvironmentID, tc, "team member", memberID); err != nil {
			return err
		}
		if m.Status != domain.TeamStatusActive {
			return fmt.Errorf("%w: team member %s is not active (status=%s)", ErrConflict, memberID, m.Status)
		}

		updated, err := q.UpdateTeamMemberRole(ctx, storegen.UpdateTeamMemberRoleParams{
			ID:   memberID,
			Role: newRole,
		})
		if err != nil {
			return mapErr(err, "update team member %s", memberID)
		}

		// Update the linked credential's scopes.
		if m.CredentialID.Valid {
			if _, err := q.UpdateCredentialScopes(ctx, storegen.UpdateCredentialScopesParams{
				ID:     m.CredentialID.UUID,
				Scopes: newScopes,
			}); err != nil {
				return mapErr(err, "update credential scopes")
			}
		}

		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "team_member", memberID.String(), "team.member_role_changed", map[string]any{
			"email": m.Email, "old_role": m.Role, "new_role": newRole,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "team.member_role_change",
			"team_member", memberID.String(),
			map[string]any{"email": m.Email, "old_role": m.Role, "new_role": newRole}); err != nil {
			return err
		}
		out = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SuspendTeamMember suspends an active team member and revokes their
// credential. The member can be reactivated later.
func (s *Service) SuspendTeamMember(ctx context.Context, tc tenant.Ctx, memberID uuid.UUID) (*storegen.TeamMember, error) {
	var out storegen.TeamMember
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		m, err := q.GetTeamMemberByID(ctx, memberID)
		if err != nil {
			return mapErr(err, "team member %s", memberID)
		}
		if err := checkTenantOwnership(m.ProviderID, m.EnvironmentID, tc, "team member", memberID); err != nil {
			return err
		}
		if m.Status != domain.TeamStatusActive {
			return fmt.Errorf("%w: team member %s is not active (status=%s)", ErrConflict, memberID, m.Status)
		}

		updated, err := q.SuspendTeamMember(ctx, memberID)
		if err != nil {
			return mapErr(err, "suspend team member %s", memberID)
		}

		// Revoke the linked credential.
		if m.CredentialID.Valid {
			if _, err := q.RevokeCredential(ctx, m.CredentialID.UUID); err != nil {
				return mapErr(err, "revoke credential")
			}
		}

		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "team_member", memberID.String(), "team.member_suspended", map[string]any{
			"email": m.Email,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "team.member_suspend",
			"team_member", memberID.String(),
			map[string]any{"email": m.Email}); err != nil {
			return err
		}
		out = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveTeamMember permanently removes a team member and revokes their
// credential. This action cannot be undone.
func (s *Service) RemoveTeamMember(ctx context.Context, tc tenant.Ctx, memberID uuid.UUID) (*storegen.TeamMember, error) {
	var out storegen.TeamMember
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		m, err := q.GetTeamMemberByID(ctx, memberID)
		if err != nil {
			return mapErr(err, "team member %s", memberID)
		}
		if err := checkTenantOwnership(m.ProviderID, m.EnvironmentID, tc, "team member", memberID); err != nil {
			return err
		}
		if m.Status == domain.TeamStatusRemoved {
			return fmt.Errorf("%w: team member %s already removed", ErrConflict, memberID)
		}

		updated, err := q.RemoveTeamMember(ctx, memberID)
		if err != nil {
			return mapErr(err, "remove team member %s", memberID)
		}

		// Revoke the linked credential.
		if m.CredentialID.Valid {
			if _, err := q.RevokeCredential(ctx, m.CredentialID.UUID); err != nil {
				return mapErr(err, "revoke credential")
			}
		}

		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "team_member", memberID.String(), "team.member_removed", map[string]any{
			"email": m.Email,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "team.member_remove",
			"team_member", memberID.String(),
			map[string]any{"email": m.Email}); err != nil {
			return err
		}
		out = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ReactivatedTeamMember is the result of reactivating a suspended member:
// the member record plus a new plaintext API key (the old one was revoked
// during suspension).
type ReactivatedTeamMember struct {
	Member storegen.TeamMember `json:"member"`
	APIKey string              `json:"api_key"`
}

// ReactivateTeamMember reactivates a suspended team member and issues a
// new API key (the old key was revoked during suspension).
func (s *Service) ReactivateTeamMember(ctx context.Context, tc tenant.Ctx, memberID uuid.UUID) (*ReactivatedTeamMember, error) {
	var out ReactivatedTeamMember
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		m, err := q.GetTeamMemberByID(ctx, memberID)
		if err != nil {
			return mapErr(err, "team member %s", memberID)
		}
		if err := checkTenantOwnership(m.ProviderID, m.EnvironmentID, tc, "team member", memberID); err != nil {
			return err
		}
		if m.Status != domain.TeamStatusSuspended {
			return fmt.Errorf("%w: team member %s is not suspended (status=%s)", ErrConflict, memberID, m.Status)
		}

		updated, err := q.ReactivateTeamMember(ctx, memberID)
		if err != nil {
			return mapErr(err, "reactivate team member %s", memberID)
		}

		// Issue a new credential with the role's scopes.
		scopes := domain.RoleScopes(m.Role)
		plaintext, err := keys.Generate(tc.EnvironmentKind)
		if err != nil {
			return err
		}
		cred, err := q.CreateCredential(ctx, storegen.CreateCredentialParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
			Name:          m.DisplayName + " [" + m.Role + "]",
			KeyPrefix:     keys.Prefix(plaintext),
			KeyHash:       keys.Hash(plaintext),
			Scopes:        scopes,
		})
		if err != nil {
			return mapErr(err, "create credential for reactivated member")
		}

		// Link the new credential.
		if err := q.SetTeamMemberCredential(ctx, storegen.SetTeamMemberCredentialParams{
			ID:           memberID,
			CredentialID: uuid.NullUUID{UUID: cred.ID, Valid: true},
		}); err != nil {
			return mapErr(err, "link credential to reactivated member")
		}

		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "team_member", memberID.String(), "team.member_reactivated", map[string]any{
			"email": m.Email,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "team.member_reactivate",
			"team_member", memberID.String(),
			map[string]any{"email": m.Email}); err != nil {
			return err
		}
		out.Member = updated
		out.APIKey = plaintext
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
