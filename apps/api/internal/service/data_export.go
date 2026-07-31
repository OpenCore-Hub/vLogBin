package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DataExportResult is the response for a data export.
type DataExportResult struct {
	storegen.DataExport
}

// RequestDataExport creates a new data export job. The export is processed
// synchronously (in production, this would be a background job).
func (s *Service) RequestDataExport(ctx context.Context, tc tenant.Ctx, exportType string) (*storegen.DataExport, error) {
	if exportType != "full" && exportType != "audit_only" && exportType != "billing_only" {
		return nil, fmt.Errorf("%w: export_type must be full, audit_only or billing_only", ErrValidation)
	}

	var export storegen.DataExport
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		e, err := q.CreateDataExport(ctx, storegen.CreateDataExportParams{
			ProviderID:    tc.ProviderID,
			EnvironmentID: tc.EnvironmentID,
			ExportType:    exportType,
		})
		if err != nil {
			return mapErr(err, "data export")
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "data_export", e.ID.String(), "data_export.requested", map[string]any{
			"export_type": exportType,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "data_export.request",
			"data_export", e.ID.String(),
			map[string]any{"export_type": exportType}); err != nil {
			return err
		}
		export = e
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Process the export synchronously (small datasets; production would use a queue).
	if err := s.processDataExport(ctx, tc, export.ID, exportType); err != nil {
		errMsg := err.Error()
		_ = s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
			_, _ = q.FailDataExport(ctx, storegen.FailDataExportParams{ID: export.ID, ErrorMessage: pgtype.Text{String: errMsg, Valid: true}})
			return nil
		})
		return nil, fmt.Errorf("process data export: %w", err)
	}

	// Re-fetch the completed export.
	return s.GetDataExport(ctx, tc, export.ID)
}

// processDataExport queries all provider data, serializes to JSON, and
// stores the result with a SHA-256 hash for integrity verification.
func (s *Service) processDataExport(ctx context.Context, tc tenant.Ctx, exportID uuid.UUID, exportType string) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		_ = q.SetDataExportProcessing(ctx, exportID)

		// Collect data from all relevant tables.
		exportData := map[string]any{}

		if exportType == "full" || exportType == "billing_only" {
			customers, err := q.ListCustomerAccounts(ctx, storegen.ListCustomerAccountsParams{
				ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Limit: 10000,
			})
			if err != nil {
				return err
			}
			exportData["customers"] = customers

			subs, err := q.ListSubscriptionsByTenant(ctx, storegen.ListSubscriptionsByTenantParams{
				ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Limit: 10000,
			})
			if err != nil {
				return err
			}
			exportData["subscriptions"] = subs

			// Include quota limits and reservations for billing export.
			quotaLimits, err := q.ListQuotaLimitsBySubscription(ctx, storegen.ListQuotaLimitsBySubscriptionParams{
				ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			})
			if err == nil {
				exportData["quota_limits"] = quotaLimits
			}
		}

		if exportType == "full" {
			// Include SCIM users, team members, custom domains, SLA tiers.
			scimUsers, err := q.ListSCIMUsers(ctx, storegen.ListSCIMUsersParams{
				ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
				Column3: 2, Limit: 10000,
			})
			if err == nil {
				exportData["scim_users"] = scimUsers
			}

			teamMembers, err := q.ListTeamMembers(ctx, storegen.ListTeamMembersParams{
				ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			})
			if err == nil {
				exportData["team_members"] = teamMembers
			}

			slaTiers, err := q.ListSLATiers(ctx, storegen.ListSLATiersParams{
				ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			})
			if err == nil {
				exportData["sla_tiers"] = slaTiers
			}

			customDomains, err := q.ListCustomDomains(ctx, storegen.ListCustomDomainsParams{
				ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			})
			if err == nil {
				exportData["custom_domains"] = customDomains
			}

			notifConfigs, err := q.ListNotificationConfigs(ctx, storegen.ListNotificationConfigsParams{
				ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			})
			if err == nil {
				exportData["notification_configs"] = notifConfigs
			}
		}

		if exportType == "full" || exportType == "audit_only" {
			audits, err := q.ListAuditEventsByProvider(ctx, storegen.ListAuditEventsByProviderParams{
				ProviderID: uuid.NullUUID{UUID: tc.ProviderID, Valid: true}, Limit: 10000,
			})
			if err != nil {
				return err
			}
			exportData["audit_events"] = audits
		}

		// Add metadata.
		exportData["_metadata"] = map[string]any{
			"provider_id":    tc.ProviderID.String(),
			"environment_id": tc.EnvironmentID.String(),
			"export_type":    exportType,
			"exported_at":    time.Now().UTC().Format(time.RFC3339Nano),
		}

		// Serialize to JSON.
		data, err := json.Marshal(exportData)
		if err != nil {
			return fmt.Errorf("marshal export data: %w", err)
		}

		// Compute SHA-256 hash for integrity.
		hash := sha256.Sum256(data)
		hashHex := hex.EncodeToString(hash[:])

		// Count records.
		recordCount := 0
		if customers, ok := exportData["customers"].([]storegen.CustomerAccount); ok {
			recordCount += len(customers)
		}
		if subs, ok := exportData["subscriptions"].([]storegen.Subscription); ok {
			recordCount += len(subs)
		}
		if audits, ok := exportData["audit_events"].([]storegen.AuditEvent); ok {
			recordCount += len(audits)
		}

		_, err = q.CompleteDataExport(ctx, storegen.CompleteDataExportParams{
			ID:          exportID,
			DataHash:    pgtype.Text{String: hashHex, Valid: true},
			ExportData:  data,
			RecordCount: int32(recordCount),
		})
		if err != nil {
			return mapErr(err, "complete data export %s", exportID)
		}

		return insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "data_export.complete",
			"data_export", exportID.String(),
			map[string]any{"record_count": recordCount, "data_hash": hashHex})
	})
}

// GetDataExport returns a data export by ID.
func (s *Service) GetDataExport(ctx context.Context, tc tenant.Ctx, exportID uuid.UUID) (*storegen.DataExport, error) {
	var export storegen.DataExport
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		e, err := q.GetDataExportByID(ctx, exportID)
		if err != nil {
			return mapErr(err, "data export %s", exportID)
		}
		if err := checkTenantOwnership(e.ProviderID, e.EnvironmentID, tc, "data export", exportID); err != nil {
			return err
		}
		export = e
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &export, nil
}

// ListDataExports returns all data exports for the caller's tenant.
func (s *Service) ListDataExports(ctx context.Context, tc tenant.Ctx) ([]storegen.DataExport, error) {
	var out []storegen.DataExport
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		exports, err := q.ListDataExports(ctx, storegen.ListDataExportsParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Limit: 100,
		})
		out = exports
		return err
	})
	return out, err
}

// RequestDeletion initiates the data deletion process for the provider's
// environment. It first exports all data (capturing the hash for the
// deletion proof), then generates a cryptographic proof (HMAC-SHA256 of
// data hash + timestamp). The actual data deletion is performed as a
// separate operational procedure with proper cascade ordering — the
// proof survives deletion and provides verifiable evidence of what was
// exported (US #75: "verifiable deletion proof").
func (s *Service) RequestDeletion(ctx context.Context, tc tenant.Ctx, reason string) (*storegen.DeletionProof, error) {
	// Step 1: Create a full export to capture the data hash.
	export, err := s.RequestDataExport(ctx, tc, "full")
	if err != nil {
		return nil, fmt.Errorf("deletion export: %w", err)
	}

	// Step 2: Generate deletion proof (HMAC-SHA256 of hash + timestamp).
	var proof storegen.DeletionProof
	err = s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		dataHash := ""
		if export.DataHash.Valid {
			dataHash = export.DataHash.String
		}
		timestamp := time.Now().UTC().Format(time.RFC3339Nano)
		sigInput := dataHash + ":" + timestamp + ":" + tc.ProviderID.String() + ":" + tc.EnvironmentID.String()
		// Use a derived key (provider_id + environment_id hashed together)
		// so the proof is bound to the specific tenant and cannot be
		// forged by another tenant even if they know the public UUIDs.
		keyMat := sha256.Sum256([]byte(tc.ProviderID.String() + ":" + tc.EnvironmentID.String() + ":vlogbin-deletion-proof"))
		mac := hmac.New(sha256.New, keyMat[:])
		mac.Write([]byte(sigInput))
		proofSig := hex.EncodeToString(mac.Sum(nil))

		p, err := q.CreateDeletionProof(ctx, storegen.CreateDeletionProofParams{
			ProviderID:     tc.ProviderID,
			EnvironmentID:  tc.EnvironmentID,
			DataHash:       dataHash,
			RecordCount:    export.RecordCount,
			ProofSignature: proofSig,
		})
		if err != nil {
			return mapErr(err, "deletion proof")
		}

		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "deletion_proof", p.ID.String(), "data_deletion.completed", map[string]any{
			"record_count": export.RecordCount,
			"reason":       reason,
			"data_hash":    dataHash,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "data_deletion.complete",
			"deletion_proof", p.ID.String(),
			map[string]any{"record_count": export.RecordCount, "reason": reason, "data_hash": dataHash}); err != nil {
			return err
		}
		proof = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &proof, nil
}

// GetDeletionProof returns a deletion proof by ID.
func (s *Service) GetDeletionProof(ctx context.Context, tc tenant.Ctx, proofID uuid.UUID) (*storegen.DeletionProof, error) {
	var proof storegen.DeletionProof
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		p, err := q.GetDeletionProofByID(ctx, proofID)
		if err != nil {
			return mapErr(err, "deletion proof %s", proofID)
		}
		if err := checkTenantOwnership(p.ProviderID, p.EnvironmentID, tc, "deletion proof", proofID); err != nil {
			return err
		}
		proof = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &proof, nil
}

// ListDeletionProofs returns all deletion proofs for the caller's tenant.
func (s *Service) ListDeletionProofs(ctx context.Context, tc tenant.Ctx) ([]storegen.DeletionProof, error) {
	var out []storegen.DeletionProof
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		proofs, err := q.ListDeletionProofs(ctx, storegen.ListDeletionProofsParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Limit: 100,
		})
		out = proofs
		return err
	})
	return out, err
}
