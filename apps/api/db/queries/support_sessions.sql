-- name: CreateSupportSession :one
INSERT INTO support_sessions (
    provider_id, environment_id, access_type, requested_by, reason, requested_scopes, expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetSupportSessionByID :one
SELECT * FROM support_sessions WHERE id = $1;

-- name: ApproveSupportSession :one
-- Provider approves a standard support request. Transitions status to active.
UPDATE support_sessions
SET status = 'active', approved_by = $2, granted_at = now(), updated_at = now()
WHERE id = $1 AND status = 'requested' AND access_type = 'standard'
RETURNING *;

-- name: SetEmergencyFirstApprover :one
-- First operator approves an emergency request (sets approved_by, status stays requested).
UPDATE support_sessions
SET approved_by = $2, updated_at = now()
WHERE id = $1 AND status = 'requested' AND access_type = 'emergency' AND approved_by IS NULL
RETURNING *;

-- name: ApproveEmergencySupportSession :one
-- Second operator approves an emergency request. Transitions status to active.
UPDATE support_sessions
SET status = 'active', second_approver = $2, granted_at = now(), updated_at = now()
WHERE id = $1 AND status = 'requested' AND access_type = 'emergency'
    AND approved_by IS NOT NULL AND approved_by != $2
RETURNING *;

-- name: DenySupportSession :one
-- Provider denies a pending standard request. The deny reason is stored
-- in revoke_reason (reused for both deny and revoke contexts).
UPDATE support_sessions
SET status = 'denied', revoke_reason = $2, updated_at = now()
WHERE id = $1 AND status = 'requested'
RETURNING *;

-- name: RevokeSupportSession :one
-- Early termination of an active session by operator or provider.
UPDATE support_sessions
SET status = 'revoked', revoked_at = now(), revoked_by = $2, revoke_reason = $3, updated_at = now()
WHERE id = $1 AND status = 'active'
RETURNING *;

-- name: ExpireSupportSessions :execrows
-- Batch-expire all sessions past their expiry. Called by the background sweeper.
UPDATE support_sessions
SET status = 'expired', updated_at = now()
WHERE status = 'active' AND expires_at <= now();

-- name: ListSupportSessionsByTenant :many
SELECT * FROM support_sessions
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: ListSupportSessionsByProvider :many
SELECT * FROM support_sessions
WHERE provider_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ListAllSupportSessions :many
-- Operator console: cross-provider JIT access queue.
SELECT * FROM support_sessions
ORDER BY created_at DESC, id DESC
LIMIT $1;

-- name: ListActiveSupportSessions :many
SELECT * FROM support_sessions
WHERE provider_id = $1 AND environment_id = $2 AND status = 'active'
ORDER BY granted_at DESC;
