-- name: InsertAuditEvent :one
INSERT INTO audit_events (provider_id, environment_id, actor_type, actor_id, action, target_type, target_id, metadata, request_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListAuditEventsByProvider :many
SELECT * FROM audit_events
WHERE provider_id = $1
ORDER BY created_at DESC
LIMIT $2;
