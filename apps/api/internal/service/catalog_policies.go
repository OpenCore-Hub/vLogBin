package service

import (
	"context"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/jackc/pgx/v5"
)

// ListPlanEntitlements returns the entitlement grants (policies) of a plan in
// the current draft version, falling back to the latest published version when
// no draft exists (read-only, never creates a version).
func (s *Service) ListPlanEntitlements(ctx context.Context, tc tenant.Ctx, planCode string) ([]storegen.EntitlementGrant, error) {
	var out []storegen.EntitlementGrant
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		versionID, err := currentCatalogVersionIDTx(ctx, q, tc)
		if err != nil {
			return err
		}
		plan, err := q.GetPlanByVersionAndCode(ctx, storegen.GetPlanByVersionAndCodeParams{
			CatalogVersionID: versionID, Code: planCode,
		})
		if err != nil {
			return mapErr(err, "plan %q", planCode)
		}
		grants, err := q.ListGrantsByPlan(ctx, plan.ID)
		if err != nil {
			return err
		}
		out = emptyIfNil(grants)
		return nil
	})
	return out, err
}

// SetPlanEntitlement upserts a single entitlement grant (policy) on a plan in
// the current draft version, auto-creating a draft by cloning the latest
// published content when absent. The grant key is immutable — it comes from
// the path; a body key that mismatches is rejected. An advisory lock
// serializes the "fetch draft + write" critical section with other catalog
// mutations.
func (s *Service) SetPlanEntitlement(ctx context.Context, tc tenant.Ctx, planCode, key string, input domain.EntitlementInput) (*storegen.EntitlementGrant, error) {
	if input.Key != "" && input.Key != key {
		return nil, fmt.Errorf("%w: entitlement key is immutable: path key %q does not match body key %q", ErrValidation, key, input.Key)
	}
	input.Key = key
	if err := domain.ValidateEntitlementInput(input); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	var out storegen.EntitlementGrant
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		if err := acquireCatalogPlanLock(ctx, tx, tc); err != nil {
			return err
		}
		version, err := ensureDraftVersionTx(ctx, q, tc)
		if err != nil {
			return err
		}
		plan, err := q.GetPlanByVersionAndCode(ctx, storegen.GetPlanByVersionAndCodeParams{
			CatalogVersionID: version.ID, Code: planCode,
		})
		if err != nil {
			return mapErr(err, "plan %q", planCode)
		}
		grant, err := q.UpsertEntitlementGrant(ctx, storegen.UpsertEntitlementGrantParams{
			PlanID: plan.ID, CatalogVersionID: version.ID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			Key: key, ValueType: input.ValueType, Value: input.Value,
		})
		if err != nil {
			return mapErr(err, "entitlement %q for plan %q", key, planCode)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "plan", plan.ID.String(),
			"catalog.entitlement_set", map[string]any{
				"catalog_version_id": version.ID.String(), "version": version.Version,
				"plan_code": plan.Code, "entitlement_key": key, "value_type": input.ValueType,
			}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(), "credential", tc.CredentialID.String(),
			"catalog.entitlement_set", "plan", plan.ID.String(),
			map[string]any{"catalog_version_id": version.ID.String(), "version": version.Version,
				"plan_code": plan.Code, "entitlement_key": key, "value_type": input.ValueType}); err != nil {
			return err
		}
		out = grant
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeletePlanEntitlement removes a single entitlement grant (policy) from a
// plan in the current draft version. Deleting from a draft never mutates an
// immutable published version; when the plan only exists in the published
// catalog, a draft is created first (cloned), then the grant is removed.
// Returns ErrNotFound when the plan or the grant does not exist.
func (s *Service) DeletePlanEntitlement(ctx context.Context, tc tenant.Ctx, planCode, key string) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		if err := acquireCatalogPlanLock(ctx, tx, tc); err != nil {
			return err
		}
		version, err := ensureDraftVersionTx(ctx, q, tc)
		if err != nil {
			return err
		}
		plan, err := q.GetPlanByVersionAndCode(ctx, storegen.GetPlanByVersionAndCodeParams{
			CatalogVersionID: version.ID, Code: planCode,
		})
		if err != nil {
			return mapErr(err, "plan %q", planCode)
		}
		n, err := q.DeleteEntitlementGrantByKey(ctx, storegen.DeleteEntitlementGrantByKeyParams{
			PlanID: plan.ID, Key: key,
		})
		if err != nil {
			return mapErr(err, "entitlement %q for plan %q", key, planCode)
		}
		if n == 0 {
			return fmt.Errorf("%w: entitlement %q not found for plan %q", ErrNotFound, key, planCode)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "plan", plan.ID.String(),
			"catalog.entitlement_deleted", map[string]any{
				"catalog_version_id": version.ID.String(), "version": version.Version,
				"plan_code": plan.Code, "entitlement_key": key,
			}); err != nil {
			return err
		}
		return insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(), "credential", tc.CredentialID.String(),
			"catalog.entitlement_delete", "plan", plan.ID.String(),
			map[string]any{"catalog_version_id": version.ID.String(), "version": version.Version,
				"plan_code": plan.Code, "entitlement_key": key})
	})
}
