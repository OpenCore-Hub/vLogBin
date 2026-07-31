package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// EntitlementSnapshotResult is the computed entitlement snapshot returned
// to callers: the pinned catalog version, the evaluated entitlements map,
// and the computation timestamp.
type EntitlementSnapshotResult struct {
	Snapshot         storegen.EntitlementSnapshot       `json:"snapshot"`
	CatalogVersionID uuid.UUID                          `json:"catalog_version_id"`
	Entitlements     map[string]domain.EvaluatedEntitlement `json:"entitlements"`
	ComputedAt       time.Time                          `json:"computed_at"`
}

// UpsertEntitlementOverrideInput is the parameter bundle for setting or
// replacing a subscription-level entitlement override.
type UpsertEntitlementOverrideInput struct {
	SubscriptionID uuid.UUID
	Key            string
	ValueType      string
	Value          json.RawMessage
	ExpiresAt      *time.Time
	Reason         string
}

// UpsertEntitlementOverride sets or replaces a subscription-level
// entitlement override. A non-expired override wins over the plan grant
// for the same key during evaluation.
func (s *Service) UpsertEntitlementOverride(ctx context.Context, tc tenant.Ctx, in UpsertEntitlementOverrideInput) (*storegen.EntitlementOverride, error) {
	if in.Key == "" {
		return nil, fmt.Errorf("%w: key is required", ErrValidation)
	}
	if !domain.ValidValueType(in.ValueType) {
		return nil, fmt.Errorf("%w: unknown value_type %q", ErrValidation, in.ValueType)
	}
	if len(in.Value) == 0 || string(in.Value) == "null" {
		return nil, fmt.Errorf("%w: value is required", ErrValidation)
	}
	var out storegen.EntitlementOverride
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Confirm the subscription belongs to this tenant (RLS-enforced).
		if _, err := q.GetSubscriptionByID(ctx, in.SubscriptionID); err != nil {
			return mapErr(err, "subscription %s", in.SubscriptionID)
		}
		override, err := q.UpsertEntitlementOverride(ctx, storegen.UpsertEntitlementOverrideParams{
			ProviderID:     tc.ProviderID,
			EnvironmentID:  tc.EnvironmentID,
			SubscriptionID: in.SubscriptionID,
			Key:            in.Key,
			ValueType:      in.ValueType,
			Value:          in.Value,
			ExpiresAt:      in.ExpiresAt,
			Reason:         in.Reason,
		})
		if err != nil {
			return mapErr(err, "entitlement override %q", in.Key)
		}
		payload := map[string]any{
			"subscription_id": in.SubscriptionID.String(),
			"key":             in.Key,
			"value_type":      in.ValueType,
			"value":           json.RawMessage(in.Value),
		}
		if in.ExpiresAt != nil {
			payload["expires_at"] = in.ExpiresAt.UTC().Format(time.RFC3339Nano)
		}
		if in.Reason != "" {
			payload["reason"] = in.Reason
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "entitlement_override", override.ID.String(), "subscription.override_set", payload); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q,
			tc.ProviderNullUUID(),
			tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "subscription.override_set",
			"entitlement_override", override.ID.String(),
			map[string]any{"subscription_id": in.SubscriptionID.String(), "key": in.Key, "value_type": in.ValueType}); err != nil {
			return err
		}
		out = override
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListEntitlementOverrides returns all overrides of a subscription.
func (s *Service) ListEntitlementOverrides(ctx context.Context, tc tenant.Ctx, subscriptionID uuid.UUID) ([]storegen.EntitlementOverride, error) {
	var out []storegen.EntitlementOverride
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Confirm ownership (RLS-enforced).
		if _, err := q.GetSubscriptionByID(ctx, subscriptionID); err != nil {
			return mapErr(err, "subscription %s", subscriptionID)
		}
		overrides, err := q.ListOverridesBySubscription(ctx, subscriptionID)
		out = overrides
		return err
	})
	return out, err
}

// DeleteEntitlementOverride removes a single override key. Returns
// ErrNotFound when no row matched.
func (s *Service) DeleteEntitlementOverride(ctx context.Context, tc tenant.Ctx, subscriptionID uuid.UUID, key string) error {
	if key == "" {
		return fmt.Errorf("%w: key is required", ErrValidation)
	}
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Confirm ownership (RLS-enforced).
		if _, err := q.GetSubscriptionByID(ctx, subscriptionID); err != nil {
			return mapErr(err, "subscription %s", subscriptionID)
		}
		rows, err := q.DeleteEntitlementOverride(ctx, storegen.DeleteEntitlementOverrideParams{
			SubscriptionID: subscriptionID, Key: key,
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return fmt.Errorf("%w: entitlement override %q on subscription %s", ErrNotFound, key, subscriptionID)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "entitlement_override", subscriptionID.String(), "subscription.override_deleted", map[string]any{
			"subscription_id": subscriptionID.String(), "key": key,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q,
			tc.ProviderNullUUID(),
			tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "subscription.override_deleted",
			"entitlement_override", subscriptionID.String(),
			map[string]any{"subscription_id": subscriptionID.String(), "key": key}); err != nil {
			return err
		}
		return nil
	})
}

// GetEntitlementSnapshot computes the current entitlements for a customer
// (plan grants merged with non-expired overrides), persists a snapshot
// row, and emits an outbox event. The customer is resolved by external_id
// within the tenant scope.
func (s *Service) GetEntitlementSnapshot(ctx context.Context, tc tenant.Ctx, customerExternalID string) (*EntitlementSnapshotResult, error) {
	if customerExternalID == "" {
		return nil, fmt.Errorf("%w: customer_external_id is required", ErrValidation)
	}
	var out EntitlementSnapshotResult
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		customer, err := q.GetCustomerByExternalID(ctx, storegen.GetCustomerByExternalIDParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ExternalID: customerExternalID,
		})
		if err != nil {
			return mapErr(err, "customer %q", customerExternalID)
		}
		sub, err := q.GetActiveSubscriptionByCustomer(ctx, customer.ID)
		if err != nil {
			return mapErr(err, "active subscription for customer %q", customerExternalID)
		}
		// The subscription pins a catalog version; the plan grants come
		// from that version's entitlement_grants for the subscription's plan.
		grants, err := q.ListGrantsByPlan(ctx, sub.PlanID)
		if err != nil {
			return err
		}
		overrides, err := q.ListOverridesBySubscription(ctx, sub.ID)
		if err != nil {
			return err
		}
		now := time.Now()
		evaluated := domain.EvaluateEntitlements(toGrantKVs(grants), toOverrideKVs(overrides), now)

		// Build the snapshot payload: catalog version + entitlements map.
		entitlementsJSON := make(map[string]any, len(evaluated))
		for k, v := range evaluated {
			entitlementsJSON[k] = v
		}
		payload, err := json.Marshal(map[string]any{
			"catalog_version_id": sub.CatalogVersionID.String(),
			"entitlements":       entitlementsJSON,
			"computed_at":        now.UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			return err
		}
		snapshot, err := q.InsertEntitlementSnapshot(ctx, storegen.InsertEntitlementSnapshotParams{
			ProviderID:        tc.ProviderID,
			EnvironmentID:     tc.EnvironmentID,
			CustomerAccountID: customer.ID,
			SubscriptionID:    sub.ID,
			CatalogVersionID:  sub.CatalogVersionID,
			Payload:           payload,
		})
		if err != nil {
			return mapErr(err, "entitlement snapshot")
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "entitlement_snapshot", snapshot.ID.String(), "entitlement.snapshot_computed", map[string]any{
			"snapshot_id":       snapshot.ID.String(),
			"customer_account_id": customer.ID.String(),
			"subscription_id":   sub.ID.String(),
			"catalog_version_id": sub.CatalogVersionID.String(),
		}); err != nil {
			return err
		}
		out = EntitlementSnapshotResult{
			Snapshot:         snapshot,
			CatalogVersionID: sub.CatalogVersionID,
			Entitlements:     evaluated,
			ComputedAt:       snapshot.ComputedAt,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func toGrantKVs(grants []storegen.EntitlementGrant) []domain.GrantKV {
	out := make([]domain.GrantKV, 0, len(grants))
	for _, g := range grants {
		out = append(out, domain.GrantKV{
			Key: g.Key, ValueType: g.ValueType, Value: g.Value,
		})
	}
	return out
}

func toOverrideKVs(overrides []storegen.EntitlementOverride) []domain.OverrideKV {
	out := make([]domain.OverrideKV, 0, len(overrides))
	for _, o := range overrides {
		out = append(out, domain.OverrideKV{
			Key: o.Key, ValueType: o.ValueType, Value: o.Value, ExpiresAt: o.ExpiresAt,
		})
	}
	return out
}
