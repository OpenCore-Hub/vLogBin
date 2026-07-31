-- name: CreateDataExport :one
INSERT INTO data_exports (provider_id, environment_id, export_type)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetDataExportByID :one
SELECT * FROM data_exports WHERE id = $1;

-- name: ListDataExports :many
SELECT * FROM data_exports
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: CompleteDataExport :one
UPDATE data_exports
SET status = 'completed', data_hash = $2, export_data = $3, record_count = $4, completed_at = now()
WHERE id = $1
RETURNING *;

-- name: FailDataExport :one
UPDATE data_exports
SET status = 'failed', error_message = $2
WHERE id = $1
RETURNING *;

-- name: SetDataExportProcessing :exec
UPDATE data_exports SET status = 'processing' WHERE id = $1;

-- name: CreateDeletionProof :one
INSERT INTO deletion_proofs (provider_id, environment_id, data_hash, record_count, proof_signature)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetDeletionProofByID :one
SELECT * FROM deletion_proofs WHERE id = $1;

-- name: ListDeletionProofs :many
SELECT * FROM deletion_proofs
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: DeleteProviderData :execrows
-- Deletes customer data for the provider's environment. Other data
-- (subscriptions, usage, invoices) should be handled by cascade or
-- separate cleanup. The deletion proof provides cryptographic evidence.
DELETE FROM customer_accounts
WHERE provider_id = $1 AND environment_id = $2;
