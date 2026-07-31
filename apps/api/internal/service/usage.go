package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Usage ingestion results (spec §10.2 idempotency).
const (
	UsageStatusAccepted  = "accepted"
	UsageStatusDuplicate = "duplicate"
	UsageStatusReversed  = "reversed"
)

// UsageIngestInput is one metering record submitted by a provider backend.
type UsageIngestInput struct {
	TransactionID      string
	MetricCode         string
	CustomerExternalID string
	Timestamp          time.Time
	Properties         json.RawMessage
}

type UsageIngestResult struct {
	Status string
	Event  storegen.UsageEvent
}

// UsagePayloadHash computes the idempotency hash over the canonical JSON of
// {transaction_id, metric_code, customer_external_id, timestamp, properties}.
// Canonicalization: keys sorted (encoding/json), timestamp normalized to UTC
// RFC3339Nano, absent properties normalized to {}.
func UsagePayloadHash(transactionID, metricCode, customerExternalID string, ts time.Time, properties json.RawMessage) (string, error) {
	var props any
	if len(properties) > 0 {
		if err := json.Unmarshal(properties, &props); err != nil {
			return "", fmt.Errorf("%w: properties must be valid JSON", ErrValidation)
		}
	}
	if props == nil {
		props = map[string]any{}
	}
	raw, err := json.Marshal(map[string]any{
		"transaction_id":       transactionID,
		"metric_code":          metricCode,
		"customer_external_id": customerExternalID,
		"timestamp":            ts.UTC().Format(time.RFC3339Nano),
		"properties":           props,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// IngestUsage accepts one usage record. Idempotency (Testing Decision #4/#5):
// same transaction_id + same payload hash → duplicate (no new row); same
// transaction_id + different hash → conflict + audit. The accepted row and
// its outbox event share the transaction_id and commit in one transaction,
// so accepted usage survives a downstream billing outage (decision #37).
func (s *Service) IngestUsage(ctx context.Context, tc tenant.Ctx, in UsageIngestInput) (*UsageIngestResult, error) {
	if in.TransactionID == "" || in.MetricCode == "" || in.CustomerExternalID == "" {
		return nil, fmt.Errorf("%w: transaction_id, metric_code and customer_external_id are required", ErrValidation)
	}
	if in.Timestamp.IsZero() {
		return nil, fmt.Errorf("%w: timestamp is required", ErrValidation)
	}
	now := time.Now()
	if in.Timestamp.After(now.Add(s.usageFutureSkew)) {
		return nil, fmt.Errorf("%w: timestamp %s is in the future", ErrValidation, in.Timestamp.Format(time.RFC3339))
	}
	if in.Timestamp.Before(now.Add(-s.usageLateWindow)) {
		return nil, fmt.Errorf("%w: timestamp %s is older than the late window %s", ErrValidation, in.Timestamp.Format(time.RFC3339), s.usageLateWindow)
	}
	hash, err := UsagePayloadHash(in.TransactionID, in.MetricCode, in.CustomerExternalID, in.Timestamp, in.Properties)
	if err != nil {
		return nil, err
	}
	props := in.Properties
	if len(props) == 0 {
		props = []byte(`{}`)
	}
	var out UsageIngestResult
	var conflictTxID string
	err = s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		existing, err := q.GetUsageEventByTransactionID(ctx, storegen.GetUsageEventByTransactionIDParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, TransactionID: in.TransactionID,
		})
		if err == nil {
			if existing.PayloadHash == hash {
				out = UsageIngestResult{Status: UsageStatusDuplicate, Event: existing}
				return nil
			}
			// Same transaction ID, different payload: reject. The audit
			// event is persisted in a separate transaction below so it
			// survives the conflict error (Testing Decision #5).
			conflictTxID = in.TransactionID
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		customer, err := q.GetCustomerByExternalID(ctx, storegen.GetCustomerByExternalIDParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ExternalID: in.CustomerExternalID,
		})
		if err != nil {
			return fmt.Errorf("%w: unknown customer_external_id %q", ErrValidation, in.CustomerExternalID)
		}
		sub, err := q.GetActiveSubscriptionByCustomer(ctx, customer.ID)
		if err != nil {
			return fmt.Errorf("%w: customer %q has no active subscription", ErrValidation, in.CustomerExternalID)
		}
		// The metric must exist in the subscription's pinned catalog version.
		if _, err := q.GetMetricByVersionAndCode(ctx, storegen.GetMetricByVersionAndCodeParams{
			CatalogVersionID: sub.CatalogVersionID, Code: in.MetricCode,
		}); err != nil {
			return fmt.Errorf("%w: unknown metric_code %q in catalog version %s", ErrValidation, in.MetricCode, sub.CatalogVersionID)
		}
		event, err := q.InsertUsageEvent(ctx, storegen.InsertUsageEventParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			TransactionID: in.TransactionID, Kind: domain.UsageKindIngestion, MetricCode: in.MetricCode,
			CustomerAccountID: customer.ID, SubscriptionID: sub.ID,
			EventTimestamp: in.Timestamp, Properties: props, PayloadHash: hash,
			ReversesID: uuid.NullUUID{}, Reason: pgtype.Text{},
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				// Racing retry with the same transaction_id: idempotent.
				out = UsageIngestResult{Status: UsageStatusDuplicate}
				return nil
			}
			return err
		}
		if err := emitOutboxWithTxIDTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "usage_event", event.ID.String(), "usage.accepted", in.TransactionID, map[string]any{
			"transaction_id":       in.TransactionID,
			"code":                 in.MetricCode,
			"external_customer_id": in.CustomerExternalID,
			"timestamp":            in.Timestamp.UTC().Format(time.RFC3339Nano),
			"properties":           json.RawMessage(props),
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "usage.accepted", "usage_event", event.ID.String(),
			map[string]any{"transaction_id": in.TransactionID, "metric_code": in.MetricCode}); err != nil {
			return err
		}
		out = UsageIngestResult{Status: UsageStatusAccepted, Event: event}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if conflictTxID != "" {
		// Persist the conflict audit in a separate transaction so it
		// survives the conflict error (Testing Decision #5: rejected
		// and audited).
		if auditErr := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
			return insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
				"credential", tc.CredentialID.String(), "usage.conflict", "usage_event", conflictTxID,
				map[string]any{"transaction_id": conflictTxID})
		}); auditErr != nil {
			s.log.Error("conflict audit persistence failed", "error", auditErr, "transaction_id", conflictTxID)
		}
		return nil, fmt.Errorf("%w: transaction_id %q was already used with a different payload", ErrUsageConflict, conflictTxID)
	}
	return &out, nil
}

// ReverseUsage corrects an accepted ingestion event with an immutable
// reversal row (originals are never overwritten; spec §10.3). Exactly one
// reversal per original (partial unique index).
func (s *Service) ReverseUsage(ctx context.Context, tc tenant.Ctx, originalTxID, reversalTxID, reason string) (*storegen.UsageEvent, error) {
	if reversalTxID == "" {
		reversalTxID = uuid.NewString()
	}
	var out storegen.UsageEvent
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		original, err := q.GetUsageEventByTransactionID(ctx, storegen.GetUsageEventByTransactionIDParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, TransactionID: originalTxID,
		})
		if err != nil {
			return mapErr(err, "usage event %q", originalTxID)
		}
		if original.Kind != domain.UsageKindIngestion {
			return fmt.Errorf("%w: usage event %q is a reversal and cannot be reversed", ErrConflict, originalTxID)
		}
		if _, err := q.GetReversalForUsageEvent(ctx, uuid.NullUUID{UUID: original.ID, Valid: true}); err == nil {
			return fmt.Errorf("%w: usage event %q is already reversed", ErrConflict, originalTxID)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		// Post-invoice reversal check (Testing #6): if the usage has been
		// invoiced, direct reversal is not allowed — a credit note must
		// be issued instead of overwriting the finalized invoice.
		invoiced, err := q.CheckUsageInvoiced(ctx, storegen.CheckUsageInvoicedParams{
			EventTransactionID: pgtype.Text{String: originalTxID, Valid: originalTxID != ""},
			ProviderID:         tc.ProviderID,
			EnvironmentID:      tc.EnvironmentID,
		})
		if err != nil {
			return err
		}
		if invoiced {
			return fmt.Errorf("%w: transaction_id %q has been invoiced; issue a credit note instead of reversing", ErrUsageAlreadyInvoiced, originalTxID)
		}
		customer, err := q.GetCustomerByID(ctx, original.CustomerAccountID)
		if err != nil {
			return err
		}
		now := time.Now()
		hash, err := UsagePayloadHash(reversalTxID, original.MetricCode, customer.ExternalID, now, original.Properties)
		if err != nil {
			return err
		}
		reversal, err := q.InsertUsageEvent(ctx, storegen.InsertUsageEventParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			TransactionID: reversalTxID, Kind: domain.UsageKindReversal, MetricCode: original.MetricCode,
			CustomerAccountID: original.CustomerAccountID, SubscriptionID: original.SubscriptionID,
			EventTimestamp: now, Properties: original.Properties, PayloadHash: hash,
			ReversesID: uuid.NullUUID{UUID: original.ID, Valid: true},
			Reason:     pgtype.Text{String: reason, Valid: reason != ""},
		})
		if err != nil {
			return mapErr(err, "reversal of %q", originalTxID)
		}
		if err := emitOutboxWithTxIDTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "usage_event", reversal.ID.String(), "usage.reversed", reversalTxID, map[string]any{
			"transaction_id":       reversalTxID,
			"code":                 original.MetricCode,
			"external_customer_id": customer.ExternalID,
			"timestamp":            now.UTC().Format(time.RFC3339Nano),
			"reverses":             originalTxID,
			"reason":               reason,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "usage.reversed", "usage_event", reversal.ID.String(),
			map[string]any{"transaction_id": reversalTxID, "reverses": originalTxID, "reason": reason}); err != nil {
			return err
		}
		out = reversal
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) ListUsageEvents(ctx context.Context, tc tenant.Ctx, limit int32) ([]storegen.UsageEvent, error) {
	var out []storegen.UsageEvent
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		evs, err := q.ListUsageEventsByTenant(ctx, storegen.ListUsageEventsByTenantParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Limit: limit,
		})
		out = evs
		return err
	})
	return out, err
}
