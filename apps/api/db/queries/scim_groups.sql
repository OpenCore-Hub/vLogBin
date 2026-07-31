-- name: CreateSCIMGroup :one
INSERT INTO scim_groups (provider_id, environment_id, external_id, display_name)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetSCIMGroupByID :one
SELECT * FROM scim_groups WHERE id = $1;

-- name: GetSCIMGroupByExternalID :one
SELECT * FROM scim_groups
WHERE provider_id = $1 AND environment_id = $2 AND external_id = $3;

-- name: ListSCIMGroups :many
SELECT * FROM scim_groups
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at ASC
LIMIT $3;

-- name: CountSCIMGroups :one
SELECT COUNT(*) FROM scim_groups
WHERE provider_id = $1 AND environment_id = $2;

-- name: UpdateSCIMGroup :one
UPDATE scim_groups
SET display_name = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteSCIMGroup :execrows
DELETE FROM scim_groups
WHERE provider_id = $1 AND environment_id = $2 AND id = $3;

-- name: AddSCIMGroupMember :one
INSERT INTO scim_group_members (group_id, user_id)
VALUES ($1, $2)
ON CONFLICT (group_id, user_id) DO NOTHING
RETURNING *;

-- name: RemoveSCIMGroupMember :execrows
DELETE FROM scim_group_members WHERE group_id = $1 AND user_id = $2;

-- name: ListSCIMGroupMembers :many
SELECT m.*, u.external_id AS user_external_id, u.display_name AS user_display_name, u.email AS user_email, u.active AS user_active
FROM scim_group_members m
JOIN scim_users u ON u.id = m.user_id
WHERE m.group_id = $1;

-- name: PatchSCIMUserActive :exec
UPDATE scim_users SET active = $2, updated_at = now() WHERE id = $1;

-- name: PatchSCIMUserDisplayName :exec
UPDATE scim_users SET display_name = $2, updated_at = now() WHERE id = $1;

-- name: PatchSCIMUserEmail :exec
UPDATE scim_users SET email = $2, updated_at = now() WHERE id = $1;

-- name: PatchSCIMGroupDisplayName :exec
UPDATE scim_groups SET display_name = $2, updated_at = now() WHERE id = $1;

-- name: ClearSCIMGroupMembers :execrows
DELETE FROM scim_group_members WHERE group_id = $1;
