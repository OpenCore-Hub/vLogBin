-- name: ListAllNotificationConfigCiphertexts :many
-- Operator-only view used by the re-encryption worker.
SELECT id, config_enc FROM notification_configs
ORDER BY id
LIMIT $1;

-- name: UpdateNotificationConfigCiphertext :exec
UPDATE notification_configs SET config_enc = $2, updated_at = now()
WHERE id = $1;

-- name: UpsertNotificationConfig :one
INSERT INTO notification_configs (provider_id, environment_id, channel, provider_type, config_enc, from_address, enabled)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (provider_id, environment_id, channel) DO UPDATE
SET provider_type = EXCLUDED.provider_type, config_enc = EXCLUDED.config_enc,
    from_address = EXCLUDED.from_address, enabled = EXCLUDED.enabled, updated_at = now()
RETURNING *;

-- name: GetNotificationConfig :one
SELECT * FROM notification_configs
WHERE provider_id = $1 AND environment_id = $2 AND channel = $3;

-- name: ListNotificationConfigs :many
SELECT * FROM notification_configs
WHERE provider_id = $1 AND environment_id = $2
ORDER BY channel;

-- name: DeleteNotificationConfig :execrows
DELETE FROM notification_configs
WHERE provider_id = $1 AND environment_id = $2 AND channel = $3;
