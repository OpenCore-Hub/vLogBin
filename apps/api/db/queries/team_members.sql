-- name: CreateTeamMember :one
INSERT INTO team_members (
    provider_id, environment_id, email, display_name, role, status, credential_id, invited_by
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetTeamMemberByID :one
SELECT * FROM team_members WHERE id = $1;

-- name: GetTeamMemberByEmail :one
SELECT * FROM team_members
WHERE provider_id = $1 AND environment_id = $2 AND email = $3;

-- name: ListTeamMembers :many
SELECT * FROM team_members
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC;

-- name: ListActiveTeamMembers :many
SELECT * FROM team_members
WHERE provider_id = $1 AND environment_id = $2 AND status = 'active'
ORDER BY created_at DESC;

-- name: UpdateTeamMemberRole :one
UPDATE team_members
SET role = $2, updated_at = now()
WHERE id = $1 AND status = 'active'
RETURNING *;

-- name: SetTeamMemberCredential :exec
UPDATE team_members
SET credential_id = $2, updated_at = now()
WHERE id = $1;

-- name: SuspendTeamMember :one
UPDATE team_members
SET status = 'suspended', updated_at = now()
WHERE id = $1 AND status = 'active'
RETURNING *;

-- name: RemoveTeamMember :one
UPDATE team_members
SET status = 'removed', updated_at = now()
WHERE id = $1 AND status IN ('active', 'suspended')
RETURNING *;

-- name: ReactivateTeamMember :one
UPDATE team_members
SET status = 'active', updated_at = now()
WHERE id = $1 AND status = 'suspended'
RETURNING *;
