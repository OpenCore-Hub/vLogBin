-- name: CreatePSPCredential :one
INSERT INTO psp_credentials (
    provider_id, environment_id, psp_type, label,
    encrypted_api_key, encrypted_webhook_secret, key_version
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListPSPCredentials :many
SELECT * FROM psp_credentials
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC;

-- name: GetPSPCredentialByID :one
SELECT * FROM psp_credentials
WHERE id = $1 AND provider_id = $2 AND environment_id = $3;

-- name: GetActivePSPByType :one
SELECT * FROM psp_credentials
WHERE provider_id = $1 AND environment_id = $2 AND psp_type = $3 AND active = true;

-- name: RevokePSPCredential :exec
UPDATE psp_credentials SET active = false, revoked_at = now()
WHERE id = $1 AND provider_id = $2 AND environment_id = $3;

-- name: CountPSPCredentialsByType :one
SELECT count(*)::bigint FROM psp_credentials
WHERE provider_id = $1 AND environment_id = $2 AND psp_type = $3 AND active = true;

-- name: ListAllPSPCredentialCiphertexts :many
-- Operator-only view used by the re-encryption worker: every row's encrypted
-- fields across all tenants, so ciphertext sealed under a rotated-out master
-- key can be re-sealed with the active key.
SELECT id, encrypted_api_key, encrypted_webhook_secret FROM psp_credentials
ORDER BY id
LIMIT $1;

-- name: UpdatePSPCredentialCiphertexts :exec
-- NULLIF keeps NULL webhook secrets NULL: sqlc reads a NULL as "" so a
-- re-write must map "" back to NULL to stay byte-identical otherwise.
UPDATE psp_credentials
SET encrypted_api_key = $2, encrypted_webhook_secret = NULLIF($3, '')
WHERE id = $1;
