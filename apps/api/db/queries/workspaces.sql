-- Workspace control-plane queries (design baseline §3.1 R11).

-- name: CreateWorkspace :one
INSERT INTO workspaces (slug, name, created_by)
VALUES ($1, $2, $3)
RETURNING *;

-- name: CreateWorkspaceIfFree :one
-- Slug allocation with transactional safety: a conflicting slug leaves the
-- transaction usable so callers can fall back to a suffixed candidate.
INSERT INTO workspaces (slug, name, created_by)
VALUES ($1, $2, $3)
ON CONFLICT (slug) DO NOTHING
RETURNING *;

-- name: GetWorkspaceByID :one
SELECT * FROM workspaces WHERE id = $1;

-- name: GetWorkspaceBySlug :one
SELECT * FROM workspaces WHERE slug = $1;

-- name: CreateWorkspaceMember :one
INSERT INTO workspace_members (workspace_id, user_sub, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetWorkspaceMember :one
SELECT * FROM workspace_members WHERE workspace_id = $1 AND user_sub = $2;

-- name: ListWorkspacesByUser :many
SELECT w.*, m.role AS membership_role, m.status AS membership_status
FROM workspace_members m
JOIN workspaces w ON w.id = m.workspace_id
WHERE m.user_sub = $1 AND m.status = 'active'
ORDER BY w.created_at;

-- name: ListWorkspaceMembers :many
SELECT * FROM workspace_members WHERE workspace_id = $1 ORDER BY created_at;

-- name: UpdateWorkspace :one
UPDATE workspaces SET name = $2, slug = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpsertWorkspaceMember :one
INSERT INTO workspace_members (workspace_id, user_sub, role, status)
VALUES ($1, $2, $3, 'active')
ON CONFLICT (workspace_id, user_sub)
DO UPDATE SET role = EXCLUDED.role, status = 'active', updated_at = now()
RETURNING *;

-- name: UpdateWorkspaceMemberRole :one
UPDATE workspace_members SET role = $3, updated_at = now()
WHERE workspace_id = $1 AND user_sub = $2 AND status = 'active'
RETURNING *;

-- name: RemoveWorkspaceMember :exec
DELETE FROM workspace_members WHERE workspace_id = $1 AND user_sub = $2;

-- name: CountWorkspaceAdmins :one
SELECT count(*)::bigint FROM workspace_members
WHERE workspace_id = $1 AND role = 'provider_admin' AND status = 'active';
