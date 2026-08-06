package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// slugInvalidChars matches characters that are not allowed in a workspace
// slug. Derived slugs are normalized to [a-z0-9-].
var slugInvalidChars = regexp.MustCompile(`[^a-z0-9-]+`)

// slugPattern validates user-supplied workspace slugs (up to 62 chars of
// [a-z0-9] and single hyphens, never starting or ending with a hyphen).
var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// workspace_members.status values (0026_workspaces.sql CHECK constraint).
const membershipActive = "active"

// ProvisionedWorkspace is the result of the signup provisioning (R11): the
// default workspace, the first user's provider_admin membership and the 1:1
// provider record (design baseline §2.1: workspace_id maps to provider_id).
type ProvisionedWorkspace struct {
	Workspace  storegen.Workspace
	Membership storegen.WorkspaceMember
	// Provider is created in REGISTERED state with no home region yet —
	// region and cell are assigned by the operator at activation
	// (REGISTERED → TEST_ACTIVE).
	Provider storegen.Provider
}

// ProvisionWorkspace implements design baseline §3.1 R11: the signup flow
// provisions a default workspace for a platform user, grants the first user
// provider_admin and records the 1:1 provider (REGISTERED). It is idempotent —
// a returning user with an active membership gets their existing workspace
// back instead of a new one, and a missing provider record is self-healed.
// All work happens in a single operator transaction, so a failure anywhere
// rolls back the entire signup.
func (s *Service) ProvisionWorkspace(ctx context.Context, sub, email, displayName string) (*ProvisionedWorkspace, error) {
	if strings.TrimSpace(sub) == "" {
		return nil, fmt.Errorf("%w: user sub is required", ErrValidation)
	}
	var out ProvisionedWorkspace
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		existing, err := q.ListWorkspacesByUser(ctx, sub)
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			ws, err := q.GetWorkspaceByID(ctx, existing[0].ID)
			if err != nil {
				return err
			}
			mem, err := q.GetWorkspaceMember(ctx, storegen.GetWorkspaceMemberParams{
				WorkspaceID: ws.ID,
				UserSub:     sub,
			})
			if err != nil {
				return err
			}
			// Self-heal the 1:1 provider record: workspaces provisioned before
			// the signup→provider link existed may lack it, and idempotent
			// signup must converge to the desired state (workspace + provider).
			provider, err := q.GetProviderByID(ctx, ws.ID)
			if errors.Is(err, pgx.ErrNoRows) {
				provider, err = q.CreateRegisteredProvider(ctx, storegen.CreateRegisteredProviderParams{
					ID:   ws.ID,
					Slug: ws.Slug,
					Name: ws.Name,
				})
			}
			if err != nil {
				return err
			}
			out.Workspace = ws
			out.Membership = mem
			out.Provider = provider
			return nil
		}
		slug := slugForWorkspace(email, displayName, sub)
		ws, err := createWorkspaceWithFreeSlug(ctx, q, slug, workspaceDisplayName(displayName, email), sub)
		if err != nil {
			return err
		}
		mem, err := q.CreateWorkspaceMember(ctx, storegen.CreateWorkspaceMemberParams{
			WorkspaceID: ws.ID,
			UserSub:     sub,
			Role:        domain.PlatformRoleProviderAdmin,
		})
		if err != nil {
			return err
		}
		provider, err := q.CreateRegisteredProvider(ctx, storegen.CreateRegisteredProviderParams{
			ID:   ws.ID,
			Slug: ws.Slug,
			Name: ws.Name,
		})
		if err != nil {
			return err
		}
		out.Workspace = ws
		out.Membership = mem
		out.Provider = provider
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListMyWorkspaces returns the workspaces a platform user is an active
// member of, ordered by creation time.
func (s *Service) ListMyWorkspaces(ctx context.Context, sub string) ([]storegen.ListWorkspacesByUserRow, error) {
	if strings.TrimSpace(sub) == "" {
		return nil, fmt.Errorf("%w: user sub is required", ErrValidation)
	}
	var out []storegen.ListWorkspacesByUserRow
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := q.ListWorkspacesByUser(ctx, sub)
		if err != nil {
			return err
		}
		out = rows
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// slugForWorkspace derives a stable base slug from the signup identity:
// email local part wins, then display name, then the user sub.
func slugForWorkspace(email, displayName, sub string) string {
	base := sub
	switch {
	case email != "":
		base = strings.SplitN(email, "@", 2)[0]
	case displayName != "":
		base = displayName
	}
	base = strings.ToLower(base)
	base = slugInvalidChars.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "workspace"
	}
	return base
}

// workspaceDisplayName builds the human-friendly default workspace name.
func workspaceDisplayName(displayName, email string) string {
	switch {
	case displayName != "":
		return displayName + "'s workspace"
	case email != "":
		return email
	default:
		return "My workspace"
	}
}

// createWorkspaceWithFreeSlug inserts a workspace, retrying with a random
// suffix when the slug is already taken (unique violation 23505).
func createWorkspaceWithFreeSlug(ctx context.Context, q *store.Queries, slug, name, createdBy string) (storegen.Workspace, error) {
	candidates := []string{slug}
	for range 5 {
		candidates = append(candidates, fmt.Sprintf("%s-%s", slug, randomSuffix(4)))
	}
	var lastErr error
	for _, cand := range candidates {
		ws, err := q.CreateWorkspaceIfFree(ctx, storegen.CreateWorkspaceIfFreeParams{Slug: cand, Name: name, CreatedBy: createdBy})
		if err == nil {
			return ws, nil
		}
		if !isNoRows(err) {
			return storegen.Workspace{}, err
		}
		lastErr = err
	}
	return storegen.Workspace{}, lastErr
}

// randomSuffix returns a lowercase hex string of n characters, used to
// disambiguate colliding slugs. On RNG failure it returns "" (the caller
// retries with further candidates).
func randomSuffix(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)[:n]
}

// GetWorkspace returns a workspace the caller is an active member of.
// Workspace existence is hidden from non-members (ErrNotFound, not
// ErrForbidden) to avoid leaking workspace IDs.
func (s *Service) GetWorkspace(ctx context.Context, sub string, workspaceID uuid.UUID) (*storegen.Workspace, error) {
	if strings.TrimSpace(sub) == "" {
		return nil, fmt.Errorf("%w: user sub is required", ErrValidation)
	}
	var out storegen.Workspace
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := requireWorkspaceMember(ctx, q, workspaceID, sub); err != nil {
			return err
		}
		ws, err := q.GetWorkspaceByID(ctx, workspaceID)
		if err != nil {
			return err
		}
		out = ws
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateWorkspaceInput carries the optional fields a workspace admin may change.
type UpdateWorkspaceInput struct {
	Name *string
	Slug *string
}

// UpdateWorkspace lets a provider_admin rename the workspace or change its
// slug. A slug that collides with another workspace fails with ErrConflict.
// At least one field must be provided.
func (s *Service) UpdateWorkspace(ctx context.Context, sub string, workspaceID uuid.UUID, in UpdateWorkspaceInput) (*storegen.Workspace, error) {
	if strings.TrimSpace(sub) == "" {
		return nil, fmt.Errorf("%w: user sub is required", ErrValidation)
	}
	if in.Name == nil && in.Slug == nil {
		return nil, fmt.Errorf("%w: nothing to update", ErrValidation)
	}
	var out storegen.Workspace
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := requireWorkspaceAdmin(ctx, q, workspaceID, sub); err != nil {
			return err
		}
		ws, err := q.GetWorkspaceByID(ctx, workspaceID)
		if err != nil {
			return err
		}
		name, slug := ws.Name, ws.Slug
		if in.Name != nil {
			name = strings.TrimSpace(*in.Name)
			if name == "" {
				return fmt.Errorf("%w: workspace name must not be empty", ErrValidation)
			}
		}
		if in.Slug != nil {
			slug = strings.ToLower(strings.TrimSpace(*in.Slug))
			if !slugPattern.MatchString(slug) {
				return fmt.Errorf("%w: slug must match ^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$", ErrValidation)
			}
		}
		updated, err := q.UpdateWorkspace(ctx, storegen.UpdateWorkspaceParams{ID: workspaceID, Name: name, Slug: slug})
		if err != nil {
			return mapErr(err, "workspace %s", workspaceID)
		}
		out = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListWorkspaceMembers returns all members of a workspace (active, suspended
// and removed) to any active member, ordered by join time.
func (s *Service) ListWorkspaceMembers(ctx context.Context, sub string, workspaceID uuid.UUID) ([]storegen.WorkspaceMember, error) {
	if strings.TrimSpace(sub) == "" {
		return nil, fmt.Errorf("%w: user sub is required", ErrValidation)
	}
	var out []storegen.WorkspaceMember
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := requireWorkspaceMember(ctx, q, workspaceID, sub); err != nil {
			return err
		}
		rows, err := q.ListWorkspaceMembers(ctx, workspaceID)
		if err != nil {
			return err
		}
		out = rows
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// InviteWorkspaceMember adds or re-activates a member. Only provider_admin can
// invite. Inviting an existing member updates their role and re-activates them
// (idempotent upsert), so a removed or suspended member can rejoin.
func (s *Service) InviteWorkspaceMember(ctx context.Context, actorSub string, workspaceID uuid.UUID, userSub, role string) (*storegen.WorkspaceMember, error) {
	if strings.TrimSpace(actorSub) == "" {
		return nil, fmt.Errorf("%w: user sub is required", ErrValidation)
	}
	userSub = strings.TrimSpace(userSub)
	if userSub == "" {
		return nil, fmt.Errorf("%w: member user sub is required", ErrValidation)
	}
	if !domain.ValidPlatformRole(role) {
		return nil, fmt.Errorf("%w: invalid role %q", ErrValidation, role)
	}
	var out storegen.WorkspaceMember
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := requireWorkspaceAdmin(ctx, q, workspaceID, actorSub); err != nil {
			return err
		}
		m, err := q.UpsertWorkspaceMember(ctx, storegen.UpsertWorkspaceMemberParams{
			WorkspaceID: workspaceID,
			UserSub:     userSub,
			Role:        role,
		})
		if err != nil {
			return mapErr(err, "workspace member %s", userSub)
		}
		out = m
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateWorkspaceMemberRole changes a member's role. Only provider_admin can
// do so, and the last active provider_admin can never be demoted.
func (s *Service) UpdateWorkspaceMemberRole(ctx context.Context, actorSub string, workspaceID uuid.UUID, userSub, role string) (*storegen.WorkspaceMember, error) {
	if strings.TrimSpace(actorSub) == "" {
		return nil, fmt.Errorf("%w: user sub is required", ErrValidation)
	}
	userSub = strings.TrimSpace(userSub)
	if userSub == "" {
		return nil, fmt.Errorf("%w: member user sub is required", ErrValidation)
	}
	if !domain.ValidPlatformRole(role) {
		return nil, fmt.Errorf("%w: invalid role %q", ErrValidation, role)
	}
	var out storegen.WorkspaceMember
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := requireWorkspaceAdmin(ctx, q, workspaceID, actorSub); err != nil {
			return err
		}
		target, err := q.GetWorkspaceMember(ctx, storegen.GetWorkspaceMemberParams{
			WorkspaceID: workspaceID,
			UserSub:     userSub,
		})
		if err != nil {
			return mapErr(err, "workspace member %s", userSub)
		}
		if target.Status != membershipActive {
			return fmt.Errorf("%w: member %s is not active", ErrConflict, userSub)
		}
		if target.Role == domain.PlatformRoleProviderAdmin && role != domain.PlatformRoleProviderAdmin {
			if err := guardLastWorkspaceAdmin(ctx, q, workspaceID); err != nil {
				return err
			}
		}
		m, err := q.UpdateWorkspaceMemberRole(ctx, storegen.UpdateWorkspaceMemberRoleParams{
			WorkspaceID: workspaceID,
			UserSub:     userSub,
			Role:        role,
		})
		if err != nil {
			return mapErr(err, "workspace member %s", userSub)
		}
		out = m
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RemoveWorkspaceMember removes a member from the workspace. Only
// provider_admin can do so, and the last active provider_admin can never be
// removed.
func (s *Service) RemoveWorkspaceMember(ctx context.Context, actorSub string, workspaceID uuid.UUID, userSub string) error {
	if strings.TrimSpace(actorSub) == "" {
		return fmt.Errorf("%w: user sub is required", ErrValidation)
	}
	userSub = strings.TrimSpace(userSub)
	if userSub == "" {
		return fmt.Errorf("%w: member user sub is required", ErrValidation)
	}
	return s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := requireWorkspaceAdmin(ctx, q, workspaceID, actorSub); err != nil {
			return err
		}
		target, err := q.GetWorkspaceMember(ctx, storegen.GetWorkspaceMemberParams{
			WorkspaceID: workspaceID,
			UserSub:     userSub,
		})
		if err != nil {
			return mapErr(err, "workspace member %s", userSub)
		}
		if target.Status != membershipActive {
			return fmt.Errorf("%w: member %s is not active", ErrConflict, userSub)
		}
		if target.Role == domain.PlatformRoleProviderAdmin {
			if err := guardLastWorkspaceAdmin(ctx, q, workspaceID); err != nil {
				return err
			}
		}
		return q.RemoveWorkspaceMember(ctx, storegen.RemoveWorkspaceMemberParams{
			WorkspaceID: workspaceID,
			UserSub:     userSub,
		})
	})
}

// requireWorkspaceMember returns the caller's active membership in the
// workspace. Workspace existence is intentionally hidden from non-members:
// a missing workspace and a missing membership both surface as ErrNotFound.
func requireWorkspaceMember(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, sub string) (*storegen.WorkspaceMember, error) {
	m, err := q.GetWorkspaceMember(ctx, storegen.GetWorkspaceMemberParams{
		WorkspaceID: workspaceID,
		UserSub:     sub,
	})
	if err != nil {
		return nil, mapErr(err, "workspace %s", workspaceID)
	}
	if m.Status != membershipActive {
		return nil, ErrNotFound
	}
	return &m, nil
}

// requireWorkspaceAdmin is requireWorkspaceMember plus the provider_admin
// check: membership management requires the highest workspace role.
func requireWorkspaceAdmin(ctx context.Context, q *store.Queries, workspaceID uuid.UUID, sub string) (*storegen.WorkspaceMember, error) {
	m, err := requireWorkspaceMember(ctx, q, workspaceID, sub)
	if err != nil {
		return nil, err
	}
	if m.Role != domain.PlatformRoleProviderAdmin {
		return nil, fmt.Errorf("%w: provider_admin role required", ErrForbidden)
	}
	return m, nil
}

// guardLastWorkspaceAdmin fails with ErrConflict when the workspace has exactly
// one active provider_admin left, protecting the workspace from losing its
// last administrator.
func guardLastWorkspaceAdmin(ctx context.Context, q *store.Queries, workspaceID uuid.UUID) error {
	n, err := q.CountWorkspaceAdmins(ctx, workspaceID)
	if err != nil {
		return err
	}
	if n <= 1 {
		return fmt.Errorf("%w: workspace must keep at least one active provider_admin", ErrConflict)
	}
	return nil
}
