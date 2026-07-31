package service

import (
	"context"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CreateSubscription subscribes a customer to a plan of a published catalog
// version. The subscription is pinned to that exact version forever (DB
// trigger); publishing a newer version never changes existing subscriptions.
func (s *Service) CreateSubscription(ctx context.Context, tc tenant.Ctx, externalID, customerExternalID string, catalogVersionID uuid.UUID, planCode string) (*storegen.Subscription, error) {
	if externalID == "" || customerExternalID == "" || planCode == "" {
		return nil, fmt.Errorf("%w: external_id, customer_external_id and plan_code are required", ErrValidation)
	}
	if catalogVersionID == uuid.Nil {
		return nil, fmt.Errorf("%w: catalog_version_id is required", ErrValidation)
	}
	// Prevent new subscriptions during migration cutover (spec Section 17.1).
	if err := s.CheckCutoverLock(ctx, tc); err != nil {
		return nil, err
	}
	var out storegen.Subscription
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		customer, err := q.GetCustomerByExternalID(ctx, storegen.GetCustomerByExternalIDParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ExternalID: customerExternalID,
		})
		if err != nil {
			return fmt.Errorf("%w: unknown customer_external_id %q", ErrValidation, customerExternalID)
		}
		version, err := q.GetCatalogVersionByID(ctx, catalogVersionID)
		if err != nil {
			return fmt.Errorf("%w: unknown catalog_version_id %s", ErrValidation, catalogVersionID)
		}
		if version.State != string(domain.CatalogPublished) {
			return fmt.Errorf("%w: catalog version %s is %s: subscriptions require a published version", ErrConflict, catalogVersionID, version.State)
		}
		plan, err := q.GetPlanByVersionAndCode(ctx, storegen.GetPlanByVersionAndCodeParams{
			CatalogVersionID: catalogVersionID, Code: planCode,
		})
		if err != nil {
			return fmt.Errorf("%w: unknown plan_code %q in catalog version %s", ErrValidation, planCode, catalogVersionID)
		}
		sub, err := q.InsertSubscription(ctx, storegen.InsertSubscriptionParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			ExternalID: externalID, CustomerAccountID: customer.ID,
			CatalogVersionID: catalogVersionID, PlanID: plan.ID,
		})
		if err != nil {
			return mapErr(err, "subscription %q", externalID)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "subscription", sub.ID.String(), "subscription.created", map[string]any{
			"subscription_id": sub.ID.String(), "external_id": externalID,
			"customer_external_id": customerExternalID, "catalog_version_id": catalogVersionID.String(), "plan_code": planCode,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "subscription.create", "subscription", sub.ID.String(),
			map[string]any{"external_id": externalID, "plan_code": planCode, "catalog_version_id": catalogVersionID.String()}); err != nil {
			return err
		}
		out = sub
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) ListSubscriptions(ctx context.Context, tc tenant.Ctx, limit int32) ([]storegen.Subscription, error) {
	var out []storegen.Subscription
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		subs, err := q.ListSubscriptionsByTenant(ctx, storegen.ListSubscriptionsByTenantParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Limit: limit,
		})
		out = subs
		return err
	})
	return out, err
}

// TerminateSubscription terminates an active subscription (the only mutable
// transition; pins stay untouched).
func (s *Service) TerminateSubscription(ctx context.Context, tc tenant.Ctx, id uuid.UUID) (*storegen.Subscription, error) {
	var out storegen.Subscription
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		sub, err := q.GetSubscriptionByID(ctx, id)
		if err != nil {
			return mapErr(err, "subscription %s", id)
		}
		if sub.Status != domain.SubscriptionActive {
			return fmt.Errorf("%w: subscription %s is already terminated", ErrConflict, id)
		}
		sub, err = q.TerminateSubscription(ctx, id)
		if err != nil {
			return err
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "subscription", sub.ID.String(), "subscription.terminated", map[string]any{
			"subscription_id": sub.ID.String(), "external_id": sub.ExternalID,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "subscription.terminate", "subscription", sub.ID.String(),
			map[string]any{"external_id": sub.ExternalID}); err != nil {
			return err
		}
		out = sub
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListSubscriptionsByProvider lists subscriptions across all environments
// of a provider (operator path, for the console).
func (s *Service) ListSubscriptionsByProvider(ctx context.Context, providerID uuid.UUID) ([]storegen.ListSubscriptionsByProviderRow, error) {
	var out []storegen.ListSubscriptionsByProviderRow
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		subs, err := q.ListSubscriptionsByProvider(ctx, providerID)
		out = subs
		return err
	})
	return out, err
}
