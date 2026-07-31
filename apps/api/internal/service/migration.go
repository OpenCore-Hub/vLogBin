package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrCutoverLocked is returned when a billing operation is attempted
// while a migration cutover lock is active for the environment.
var ErrCutoverLocked = fmt.Errorf("cutover lock active: billing operations are suspended during migration")

// CreateMigrationJobInput is the parameter bundle for creating a migration job.
type CreateMigrationJobInput struct {
	SourceSystem string
	DryRun       bool
	CreatedBy    string
}

// CreateMigrationJob creates a new migration job for importing data from
// an external billing system.
func (s *Service) CreateMigrationJob(ctx context.Context, tc tenant.Ctx, in CreateMigrationJobInput) (*storegen.MigrationJob, error) {
	if in.SourceSystem == "" {
		return nil, fmt.Errorf("%w: source_system is required", ErrValidation)
	}
	var job storegen.MigrationJob
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		j, err := q.CreateMigrationJob(ctx, storegen.CreateMigrationJobParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
			SourceSystem:  in.SourceSystem,
			DryRun:        in.DryRun,
			CreatedBy:     in.CreatedBy,
		})
		if err != nil {
			return mapErr(err, "migration job")
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "migration_job", j.ID.String(), "migration.job_created", map[string]any{
			"source_system": in.SourceSystem, "dry_run": in.DryRun,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "migration.job_create",
			"migration_job", j.ID.String(),
			map[string]any{"source_system": in.SourceSystem, "dry_run": in.DryRun}); err != nil {
			return err
		}
		job = j
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// MigrationRecordInput is a single record to import.
type MigrationRecordInput struct {
	RecordType string            `json:"record_type"`
	ExternalID string            `json:"external_id"`
	SourceData map[string]any    `json:"source_data"`
}

// AddMigrationRecords bulk-adds records to a migration job. Duplicate
// (record_type, external_id) pairs are silently skipped (ON CONFLICT DO NOTHING).
func (s *Service) AddMigrationRecords(ctx context.Context, tc tenant.Ctx, jobID uuid.UUID, records []MigrationRecordInput) (int, error) {
	added := 0
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Verify the job belongs to this tenant.
		job, err := q.GetMigrationJobByIDForTenant(ctx, storegen.GetMigrationJobByIDForTenantParams{
			ID: jobID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "migration job %s", jobID)
		}
		if job.Status != domain.MigrationStatusDraft {
			return fmt.Errorf("%w: migration job %s is not in draft status (status=%s)", ErrConflict, jobID, job.Status)
		}

		for _, rec := range records {
			if !domain.ValidMigrationRecordType(rec.RecordType) {
				return fmt.Errorf("%w: record_type must be customer or subscription", ErrValidation)
			}
			if rec.ExternalID == "" {
				return fmt.Errorf("%w: external_id is required", ErrValidation)
			}
			data, err := json.Marshal(rec.SourceData)
			if err != nil {
				return fmt.Errorf("%w: invalid source_data: %v", ErrValidation, err)
			}
			created, err := q.CreateMigrationRecord(ctx, storegen.CreateMigrationRecordParams{
				MigrationJobID: jobID,
				RecordType:     rec.RecordType,
				ExternalID:     rec.ExternalID,
				SourceData:     data,
			})
			if err == nil && created.ID != uuid.Nil {
				added++
			}
		}

		// Update total_records (accumulate).
		if err := q.SetMigrationJobTotalRecords(ctx, storegen.SetMigrationJobTotalRecordsParams{
			ID: jobID, TotalRecords: int32(added),
		}); err != nil {
			return err
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "migration_job", jobID.String(), "migration.records_added", map[string]any{
			"added": added,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return added, nil
}

// ValidateMigrationJob performs dry-run validation of all pending records.
// Each record's source_data is checked for required fields. Valid records
// are marked "valid"; invalid records are marked "invalid" with an error message.
func (s *Service) ValidateMigrationJob(ctx context.Context, tc tenant.Ctx, jobID uuid.UUID) (*storegen.MigrationJob, error) {
	var job storegen.MigrationJob
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		j, err := q.GetMigrationJobByIDForTenant(ctx, storegen.GetMigrationJobByIDForTenantParams{
			ID: jobID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "migration job %s", jobID)
		}
		if j.Status != domain.MigrationStatusDraft && j.Status != domain.MigrationStatusValidated {
			return fmt.Errorf("%w: migration job %s must be in draft or validated status (status=%s)", ErrConflict, jobID, j.Status)
		}

		// Set status to validating.
		if _, err := q.UpdateMigrationJobStatus(ctx, storegen.UpdateMigrationJobStatusParams{
			ID: jobID, Status: domain.MigrationStatusValidating,
		}); err != nil {
			return mapErr(err, "update migration job status")
		}

		// Validate each pending record.
		records, err := q.ListMigrationRecords(ctx, storegen.ListMigrationRecordsParams{
			MigrationJobID: jobID, Limit: 10000,
		})
		if err != nil {
			return mapErr(err, "list migration records")
		}

		validCount, invalidCount := 0, 0
		for _, rec := range records {
			errMsg := validateCustomerRecord(rec.SourceData)
			status := domain.MigrationRecordValid
			if errMsg != "" {
				status = domain.MigrationRecordInvalid
				invalidCount++
			} else {
				validCount++
			}
			if _, err := q.SetMigrationRecordStatus(ctx, storegen.SetMigrationRecordStatusParams{
				ID:           rec.ID,
				Status:       status,
				ErrorMessage: textOrNil(errMsg),
			}); err != nil {
				return mapErr(err, "update migration record")
			}
		}

		// Set status to validated.
		updated, err := q.UpdateMigrationJobStatus(ctx, storegen.UpdateMigrationJobStatusParams{
			ID: jobID, Status: domain.MigrationStatusValidated,
		})
		if err != nil {
			return mapErr(err, "update migration job status")
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "migration_job", jobID.String(), "migration.validated", map[string]any{
			"valid": validCount, "invalid": invalidCount,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "migration.job_validate",
			"migration_job", jobID.String(),
			map[string]any{"valid": validCount, "invalid": invalidCount}); err != nil {
			return err
		}
		job = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// StartMigration begins importing valid records. It acquires the cutover
// lock and processes records in batches. If the job was interrupted, call
// ResumeMigration to continue from where it left off.
func (s *Service) StartMigration(ctx context.Context, tc tenant.Ctx, jobID uuid.UUID) (*storegen.MigrationJob, error) {
	var job storegen.MigrationJob
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		j, err := q.GetMigrationJobByIDForTenant(ctx, storegen.GetMigrationJobByIDForTenantParams{
			ID: jobID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "migration job %s", jobID)
		}
		if j.Status != domain.MigrationStatusValidated && j.Status != domain.MigrationStatusImporting {
			return fmt.Errorf("%w: migration job %s must be validated or importing (status=%s)", ErrConflict, jobID, j.Status)
		}

		// Acquire cutover lock.
		if _, err := q.SetCutoverLock(ctx, storegen.SetCutoverLockParams{
			ID: jobID, CutoverLocked: true,
		}); err != nil {
			return mapErr(err, "acquire cutover lock")
		}

		updated, err := q.SetMigrationJobStarted(ctx, jobID)
		if err != nil {
			return mapErr(err, "start migration job")
		}
		job = updated
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Process records outside the initial transaction (resumable).
	return &job, s.processMigrationBatch(ctx, tc, jobID)
}

// ResumeMigration continues importing records from where the job left off.
// This is used after an interruption (crash, timeout, etc.).
func (s *Service) ResumeMigration(ctx context.Context, tc tenant.Ctx, jobID uuid.UUID) error {
	return s.processMigrationBatch(ctx, tc, jobID)
}

// processMigrationBatch processes a batch of pending/valid records. It
// commits each record individually so that progress is preserved across
// interruptions (resumability).
func (s *Service) processMigrationBatch(ctx context.Context, tc tenant.Ctx, jobID uuid.UUID) error {
	const batchSize = 100
	for {
		var batch []storegen.MigrationRecord
		err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
			records, err := q.ListPendingMigrationRecords(ctx, storegen.ListPendingMigrationRecordsParams{
				MigrationJobID: jobID, Limit: batchSize,
			})
			batch = records
			return err
		})
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}

		for _, rec := range batch {
			s.importSingleRecord(ctx, tc, rec)
		}
	}

	// Update final progress counts.
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		counts, err := q.CountMigrationRecordsByStatus(ctx, jobID)
		if err != nil {
			return err
		}
		processed := int32(counts.Imported + counts.Failed)
		return q.UpdateMigrationJobProgress(ctx, storegen.UpdateMigrationJobProgressParams{
			ID: jobID, ProcessedRecords: processed, FailedRecords: int32(counts.Failed),
		})
	})
}

// importSingleRecord imports a single record. Customer records are imported
// via CreateCustomer; subscription records are marked as failed with a
// clear message indicating subscription import is not yet supported.
// Errors are recorded per-record without aborting the batch.
func (s *Service) importSingleRecord(ctx context.Context, tc tenant.Ctx, rec storegen.MigrationRecord) {
	if rec.RecordType == domain.MigrationRecordSubscription {
		s.markRecordFailed(ctx, tc, rec.ID, "subscription import is not yet supported; import customers first, then create subscriptions via the API")
		return
	}

	var source struct {
		Name         string `json:"name"`
		Email        string `json:"email"`
		Type         string `json:"type"`
		ExternalCode string `json:"external_code"`
	}
	if err := json.Unmarshal(rec.SourceData, &source); err != nil {
		s.markRecordFailed(ctx, tc, rec.ID, fmt.Sprintf("invalid source_data: %v", err))
		return
	}

	customer, err := s.CreateCustomer(ctx, tc, source.ExternalCode, source.Type, source.Name)
	if err != nil {
		s.markRecordFailed(ctx, tc, rec.ID, err.Error())
		return
	}

	// Mark record as imported with the target customer ID.
	_ = s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		_, err := q.SetMigrationRecordImported(ctx, storegen.SetMigrationRecordImportedParams{
			ID: rec.ID, TargetID: uuid.NullUUID{UUID: customer.ID, Valid: true},
		})
		return err
	})
}

// markRecordFailed marks a migration record as failed with an error message.
func (s *Service) markRecordFailed(ctx context.Context, tc tenant.Ctx, recordID uuid.UUID, errMsg string) {
	_ = s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		_, err := q.SetMigrationRecordStatus(ctx, storegen.SetMigrationRecordStatusParams{
			ID: recordID, Status: domain.MigrationRecordFailed, ErrorMessage: textOrNil(errMsg),
		})
		return err
	})
}

// CompleteMigration marks a migration job as completed and releases the
// cutover lock.
func (s *Service) CompleteMigration(ctx context.Context, tc tenant.Ctx, jobID uuid.UUID) (*storegen.MigrationJob, error) {
	var job storegen.MigrationJob
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		j, err := q.GetMigrationJobByIDForTenant(ctx, storegen.GetMigrationJobByIDForTenantParams{
			ID: jobID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "migration job %s", jobID)
		}
		if j.Status != domain.MigrationStatusImporting {
			return fmt.Errorf("%w: migration job %s is not importing (status=%s)", ErrConflict, jobID, j.Status)
		}

		// Release cutover lock.
		if _, err := q.SetCutoverLock(ctx, storegen.SetCutoverLockParams{
			ID: jobID, CutoverLocked: false,
		}); err != nil {
			return mapErr(err, "release cutover lock")
		}

		updated, err := q.CompleteMigrationJob(ctx, jobID)
		if err != nil {
			return mapErr(err, "complete migration job")
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "migration.job_complete",
			"migration_job", jobID.String(),
			map[string]any{"processed": j.ProcessedRecords, "failed": j.FailedRecords}); err != nil {
			return err
		}
		job = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// RollbackMigration marks a migration job and all imported records as
// rolled back. The cutover lock is released. Imported customers remain
// in the database (soft rollback) — a hard rollback would violate
// referential integrity if the customers have subscriptions.
func (s *Service) RollbackMigration(ctx context.Context, tc tenant.Ctx, jobID uuid.UUID) (*storegen.MigrationJob, error) {
	var job storegen.MigrationJob
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		j, err := q.GetMigrationJobByIDForTenant(ctx, storegen.GetMigrationJobByIDForTenantParams{
			ID: jobID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "migration job %s", jobID)
		}
		if j.Status != domain.MigrationStatusImporting && j.Status != domain.MigrationStatusCompleted && j.Status != domain.MigrationStatusFailed {
			return fmt.Errorf("%w: migration job %s cannot be rolled back (status=%s)", ErrConflict, jobID, j.Status)
		}

		// Release cutover lock.
		if _, err := q.SetCutoverLock(ctx, storegen.SetCutoverLockParams{
			ID: jobID, CutoverLocked: false,
		}); err != nil {
			return mapErr(err, "release cutover lock")
		}

		// Mark all imported records as rolled back.
		if _, err := q.RollbackMigrationRecords(ctx, jobID); err != nil {
			return mapErr(err, "rollback migration records")
		}

		updated, err := q.RollbackMigrationJob(ctx, jobID)
		if err != nil {
			return mapErr(err, "rollback migration job")
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "migration_job", jobID.String(), "migration.rolled_back", nil); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "migration.job_rollback",
			"migration_job", jobID.String(),
			nil); err != nil {
			return err
		}
		job = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// GetMigrationJob returns a migration job by ID with progress counts.
func (s *Service) GetMigrationJob(ctx context.Context, tc tenant.Ctx, jobID uuid.UUID) (*storegen.MigrationJob, error) {
	var job storegen.MigrationJob
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		j, err := q.GetMigrationJobByIDForTenant(ctx, storegen.GetMigrationJobByIDForTenantParams{
			ID: jobID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "migration job %s", jobID)
		}
		job = j
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// ListMigrationJobs returns all migration jobs for the caller's tenant.
func (s *Service) ListMigrationJobs(ctx context.Context, tc tenant.Ctx) ([]storegen.MigrationJob, error) {
	var out []storegen.MigrationJob
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		jobs, err := q.ListMigrationJobs(ctx, storegen.ListMigrationJobsParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Limit: 100,
		})
		out = jobs
		return err
	})
	return out, err
}

// ListMigrationRecords returns all records for a migration job.
func (s *Service) ListMigrationRecords(ctx context.Context, tc tenant.Ctx, jobID uuid.UUID) ([]storegen.MigrationRecord, error) {
	var out []storegen.MigrationRecord
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Verify job belongs to tenant.
		if _, err := q.GetMigrationJobByIDForTenant(ctx, storegen.GetMigrationJobByIDForTenantParams{
			ID: jobID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		}); err != nil {
			return mapErr(err, "migration job %s", jobID)
		}
		records, err := q.ListMigrationRecords(ctx, storegen.ListMigrationRecordsParams{
			MigrationJobID: jobID, Limit: 1000,
		})
		out = records
		return err
	})
	return out, err
}

// ListInvalidRecords returns all invalid records for a migration job
// (used for the diff/error report).
func (s *Service) ListInvalidRecords(ctx context.Context, tc tenant.Ctx, jobID uuid.UUID) ([]storegen.MigrationRecord, error) {
	var out []storegen.MigrationRecord
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetMigrationJobByIDForTenant(ctx, storegen.GetMigrationJobByIDForTenantParams{
			ID: jobID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		}); err != nil {
			return mapErr(err, "migration job %s", jobID)
		}
		records, err := q.ListInvalidMigrationRecords(ctx, storegen.ListInvalidMigrationRecordsParams{
			MigrationJobID: jobID, Limit: 1000,
		})
		out = records
		return err
	})
	return out, err
}

// IsCutoverLocked checks if the given provider environment has an active
// cutover lock (preventing new billing operations).
func (s *Service) IsCutoverLocked(ctx context.Context, tc tenant.Ctx) (bool, error) {
	var locked bool
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		l, err := q.HasActiveCutoverLock(ctx, storegen.HasActiveCutoverLockParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		locked = l
		return err
	})
	return locked, err
}

// CheckCutoverLock returns ErrCutoverLocked if the environment has an
// active cutover lock. This should be called at the beginning of billing
// operations (CreateSubscription, IngestUsage) to prevent dual-active
// billing during migration.
func (s *Service) CheckCutoverLock(ctx context.Context, tc tenant.Ctx) error {
	locked, err := s.IsCutoverLocked(ctx, tc)
	if err != nil {
		return err
	}
	if locked {
		return ErrCutoverLocked
	}
	return nil
}

// validateCustomerRecord validates the source_data for a customer record.
// Returns an error message string, or "" if valid.
func validateCustomerRecord(sourceData []byte) string {
	var d struct {
		Name         string `json:"name"`
		Email        string `json:"email"`
		Type         string `json:"type"`
		ExternalCode string `json:"external_code"`
	}
	if err := json.Unmarshal(sourceData, &d); err != nil {
		return fmt.Sprintf("invalid JSON: %v", err)
	}
	if strings.TrimSpace(d.Name) == "" {
		return "name is required"
	}
	if strings.TrimSpace(d.Email) == "" {
		return "email is required"
	}
	if d.Type != "individual" && d.Type != "business" {
		return "type must be individual or business"
	}
	if strings.TrimSpace(d.ExternalCode) == "" {
		return "external_code is required"
	}
	return ""
}

// textOrNil returns pgtype.Text for non-empty strings, or invalid pgtype.Text for empty.
func textOrNil(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
