-- name: CreateMigrationJob :one
INSERT INTO migration_jobs (provider_id, environment_id, source_system, dry_run, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetMigrationJobByID :one
SELECT * FROM migration_jobs WHERE id = $1;

-- name: GetMigrationJobByIDForTenant :one
SELECT * FROM migration_jobs
WHERE id = $1 AND provider_id = $2 AND environment_id = $3;

-- name: ListMigrationJobs :many
SELECT * FROM migration_jobs
WHERE provider_id = $1 AND environment_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: UpdateMigrationJobStatus :one
UPDATE migration_jobs
SET status = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetMigrationJobStarted :one
UPDATE migration_jobs
SET status = 'importing', started_at = now(), updated_at = now()
WHERE id = $1 AND status IN ('validated', 'importing')
RETURNING *;

-- name: CompleteMigrationJob :one
UPDATE migration_jobs
SET status = 'completed', completed_at = now(), updated_at = now()
WHERE id = $1 AND status = 'importing'
RETURNING *;

-- name: FailMigrationJob :one
UPDATE migration_jobs
SET status = 'failed', error_summary = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: RollbackMigrationJob :one
UPDATE migration_jobs
SET status = 'rolled_back', updated_at = now()
WHERE id = $1 AND status IN ('importing', 'completed', 'failed')
RETURNING *;

-- name: SetCutoverLock :one
UPDATE migration_jobs
SET cutover_locked = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: HasActiveCutoverLock :one
SELECT EXISTS (
    SELECT 1 FROM migration_jobs
    WHERE provider_id = $1 AND environment_id = $2
        AND cutover_locked = true
        AND status NOT IN ('rolled_back')
) AS locked;

-- name: UpdateMigrationJobProgress :exec
UPDATE migration_jobs
SET processed_records = $2, failed_records = $3, updated_at = now()
WHERE id = $1;

-- name: CreateMigrationRecord :one
INSERT INTO migration_records (migration_job_id, record_type, external_id, source_data)
VALUES ($1, $2, $3, $4)
ON CONFLICT (migration_job_id, record_type, external_id) DO NOTHING
RETURNING *;

-- name: GetMigrationRecordByExternalID :one
SELECT * FROM migration_records
WHERE migration_job_id = $1 AND record_type = $2 AND external_id = $3;

-- name: ListMigrationRecords :many
SELECT * FROM migration_records
WHERE migration_job_id = $1
ORDER BY created_at ASC
LIMIT $2;

-- name: ListPendingMigrationRecords :many
SELECT * FROM migration_records
WHERE migration_job_id = $1 AND status IN ('pending', 'valid')
ORDER BY created_at ASC
LIMIT $2;

-- name: ListInvalidMigrationRecords :many
SELECT * FROM migration_records
WHERE migration_job_id = $1 AND status = 'invalid'
ORDER BY created_at ASC
LIMIT $2;

-- name: SetMigrationRecordStatus :one
UPDATE migration_records
SET status = $2, error_message = $3, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: SetMigrationRecordImported :one
UPDATE migration_records
SET status = 'imported', target_id = $2, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: RollbackMigrationRecords :execrows
UPDATE migration_records
SET status = 'rolled_back', updated_at = now()
WHERE migration_job_id = $1 AND status = 'imported';

-- name: CountMigrationRecordsByStatus :one
SELECT
    COUNT(*) FILTER (WHERE status = 'pending') AS pending,
    COUNT(*) FILTER (WHERE status = 'valid') AS valid,
    COUNT(*) FILTER (WHERE status = 'invalid') AS invalid,
    COUNT(*) FILTER (WHERE status = 'imported') AS imported,
    COUNT(*) FILTER (WHERE status = 'failed') AS failed
FROM migration_records
WHERE migration_job_id = $1;

-- name: SetMigrationJobTotalRecords :exec
UPDATE migration_jobs
SET total_records = total_records + $2, updated_at = now()
WHERE id = $1;
