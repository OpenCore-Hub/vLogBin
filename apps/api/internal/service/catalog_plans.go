package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PlanDetail is a plan with its full pricing and entitlement content.
type PlanDetail struct {
	Plan   storegen.Plan               `json:"plan"`
	Prices []storegen.Price            `json:"prices"`
	Grants []storegen.EntitlementGrant `json:"entitlement_grants"`
}

// ListPlans returns the plans of the current draft version, falling back to
// the latest published version when no draft exists (read-only, never
// creates a version).
func (s *Service) ListPlans(ctx context.Context, tc tenant.Ctx) ([]storegen.Plan, error) {
	var out []storegen.Plan
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		versionID, err := currentCatalogVersionIDTx(ctx, q, tc)
		if err != nil {
			return err
		}
		plans, err := q.ListPlansByVersion(ctx, versionID)
		if err != nil {
			return err
		}
		out = emptyIfNil(plans)
		return nil
	})
	return out, err
}

// CreatePlan adds a plan to the current draft version. When no draft exists,
// the latest published version is cloned into a new draft (or an empty draft
// is created when nothing is published yet), so a plan change is staged
// without mutating an immutable published version. An advisory lock
// serializes the "fetch draft + write" critical section across concurrent
// plan mutations.
func (s *Service) CreatePlan(ctx context.Context, tc tenant.Ctx, input domain.PlanInput) (*PlanDetail, error) {
	var out PlanDetail
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		if err := acquireCatalogPlanLock(ctx, tx, tc); err != nil {
			return err
		}
		version, err := ensureDraftVersionTx(ctx, q, tc)
		if err != nil {
			return err
		}
		plans, err := q.ListPlansByVersion(ctx, version.ID)
		if err != nil {
			return err
		}
		for _, p := range plans {
			if p.Code == input.Code {
				return fmt.Errorf("%w: plan %q already exists in catalog version %d", ErrConflict, input.Code, version.Version)
			}
		}
		if err := validatePlanStructureTx(ctx, q, version.ID, input); err != nil {
			return err
		}
		plan, err := insertPlanContentTx(ctx, q, tc, version.ID, input)
		if err != nil {
			return err
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "plan", plan.ID.String(),
			"catalog.plan_created", map[string]any{
				"catalog_version_id": version.ID.String(), "version": version.Version, "plan_code": plan.Code,
			}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(), "credential", tc.CredentialID.String(),
			"catalog.plan_create", "plan", plan.ID.String(),
			map[string]any{"catalog_version_id": version.ID.String(), "version": version.Version, "plan_code": plan.Code}); err != nil {
			return err
		}
		detail, err := loadPlanDetailTx(ctx, q, *plan)
		if err != nil {
			return err
		}
		out = *detail
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdatePlan replaces the pricing/entitlement content of an existing plan in
// the current draft version. The plan row keeps its ID (subscriptions pin
// plans by id); prices and grants are rebuilt. The plan code is immutable —
// the identifier comes from the path.
func (s *Service) UpdatePlan(ctx context.Context, tc tenant.Ctx, code string, input domain.PlanInput) (*PlanDetail, error) {
	if input.Code != "" && input.Code != code {
		return nil, fmt.Errorf("%w: plan code is immutable, path code %q does not match body code %q", ErrValidation, code, input.Code)
	}
	input.Code = code
	var out PlanDetail
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		if err := acquireCatalogPlanLock(ctx, tx, tc); err != nil {
			return err
		}
		version, err := ensureDraftVersionTx(ctx, q, tc)
		if err != nil {
			return err
		}
		existing, err := q.GetPlanByVersionAndCode(ctx, storegen.GetPlanByVersionAndCodeParams{
			CatalogVersionID: version.ID, Code: code,
		})
		if err != nil {
			return mapErr(err, "plan %q", code)
		}
		if err := validatePlanStructureTx(ctx, q, version.ID, input); err != nil {
			return err
		}
		updated, err := q.UpdatePlan(ctx, storegen.UpdatePlanParams{
			ID: existing.ID, Name: input.Name, Interval: input.Interval, Currency: input.Currency,
		})
		if err != nil {
			return mapErr(err, "plan %q", code)
		}
		if err := replacePlanChildrenTx(ctx, q, tc, version.ID, updated.ID, input); err != nil {
			return err
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "plan", updated.ID.String(),
			"catalog.plan_updated", map[string]any{
				"catalog_version_id": version.ID.String(), "version": version.Version, "plan_code": updated.Code,
			}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(), "credential", tc.CredentialID.String(),
			"catalog.plan_update", "plan", updated.ID.String(),
			map[string]any{"catalog_version_id": version.ID.String(), "version": version.Version, "plan_code": updated.Code}); err != nil {
			return err
		}
		detail, err := loadPlanDetailTx(ctx, q, updated)
		if err != nil {
			return err
		}
		out = *detail
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeletePlan removes a plan from the current draft version. Deleting from a
// draft never affects an immutable published version; when the plan only
// exists in the published catalog, a draft is created first (cloned), then
// the plan is removed from the draft.
func (s *Service) DeletePlan(ctx context.Context, tc tenant.Ctx, code string) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		if err := acquireCatalogPlanLock(ctx, tx, tc); err != nil {
			return err
		}
		version, err := ensureDraftVersionTx(ctx, q, tc)
		if err != nil {
			return err
		}
		existing, err := q.GetPlanByVersionAndCode(ctx, storegen.GetPlanByVersionAndCodeParams{
			CatalogVersionID: version.ID, Code: code,
		})
		if err != nil {
			return mapErr(err, "plan %q", code)
		}
		if err := q.DeleteGrantsByPlan(ctx, existing.ID); err != nil {
			return err
		}
		if err := q.DeletePricesByPlan(ctx, existing.ID); err != nil {
			return err
		}
		if _, err := q.DeletePlanByVersionAndCode(ctx, storegen.DeletePlanByVersionAndCodeParams{
			CatalogVersionID: version.ID, Code: code,
		}); err != nil {
			return mapErr(err, "plan %q", code)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "plan", existing.ID.String(),
			"catalog.plan_deleted", map[string]any{
				"catalog_version_id": version.ID.String(), "version": version.Version, "plan_code": code,
			}); err != nil {
			return err
		}
		return insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(), "credential", tc.CredentialID.String(),
			"catalog.plan_delete", "plan", existing.ID.String(),
			map[string]any{"catalog_version_id": version.ID.String(), "version": version.Version, "plan_code": code})
	})
}

// acquireCatalogPlanLock takes a transaction-scoped advisory lock that
// serializes every "fetch draft + write" plan mutation per tenant. Released
// automatically when the transaction ends.
func acquireCatalogPlanLock(ctx context.Context, tx pgx.Tx, tc tenant.Ctx) error {
	_, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`,
		tc.ProviderID.String()+":catalog-plans", tc.EnvironmentID.String())
	return err
}

// currentCatalogVersionIDTx resolves the version to read plans from: the
// current draft, falling back to the latest published version. No version is
// ever created by this read path. Returns pgx.ErrNoRows when no version
// exists at all.
func currentCatalogVersionIDTx(ctx context.Context, q *store.Queries, tc tenant.Ctx) (uuid.UUID, error) {
	draft, err := q.GetDraftCatalogVersionByTenant(ctx, storegen.GetDraftCatalogVersionByTenantParams{
		ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
	})
	if err == nil {
		return draft.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}
	published, err := q.GetLatestPublishedCatalogVersionByTenant(ctx, storegen.GetLatestPublishedCatalogVersionByTenantParams{
		ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return published.ID, nil
}

// ensureDraftVersionTx returns the current draft version, creating one when
// none exists: the latest published version is deep-cloned into a new draft,
// or an empty draft is created when nothing has been published. Callers must
// hold acquireCatalogPlanLock so concurrent creation is serialized.
func ensureDraftVersionTx(ctx context.Context, q *store.Queries, tc tenant.Ctx) (storegen.CatalogVersion, error) {
	draft, err := q.GetDraftCatalogVersionByTenant(ctx, storegen.GetDraftCatalogVersionByTenantParams{
		ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
	})
	if err == nil {
		return draft, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return storegen.CatalogVersion{}, err
	}
	var fromID *uuid.UUID
	published, err := q.GetLatestPublishedCatalogVersionByTenant(ctx, storegen.GetLatestPublishedCatalogVersionByTenantParams{
		ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
	})
	if err == nil {
		fromID = &published.ID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return storegen.CatalogVersion{}, err
	}
	return createDraftTx(ctx, q, tc, fromID)
}

// createDraftTx creates a new draft version, optionally deep-cloning the
// content of an existing version.
func createDraftTx(ctx context.Context, q *store.Queries, tc tenant.Ctx, fromID *uuid.UUID) (storegen.CatalogVersion, error) {
	next, err := q.NextCatalogVersionNumber(ctx, storegen.NextCatalogVersionNumberParams{
		ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
	})
	if err != nil {
		return storegen.CatalogVersion{}, mapErr(err, "next catalog version number")
	}
	version, err := q.InsertCatalogVersion(ctx, storegen.InsertCatalogVersionParams{
		ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Version: next,
	})
	if err != nil {
		return storegen.CatalogVersion{}, mapErr(err, "catalog version %d", next)
	}
	if fromID != nil {
		if err := cloneCatalogContentTx(ctx, q, tc, *fromID, version.ID); err != nil {
			return storegen.CatalogVersion{}, err
		}
	}
	return version, nil
}

// validatePlanStructureTx validates a single plan against the metrics
// declared in the version (price metric references must resolve).
func validatePlanStructureTx(ctx context.Context, q *store.Queries, versionID uuid.UUID, plan domain.PlanInput) error {
	metrics, err := q.ListMetricsByVersion(ctx, versionID)
	if err != nil {
		return err
	}
	content := domain.CatalogContent{}
	for _, m := range metrics {
		billable := m.Billable
		content.Metrics = append(content.Metrics, domain.MetricInput{
			Code: m.Code, Name: m.Name, AggregationType: m.AggregationType,
			FieldName: m.FieldName.String, Billable: &billable,
		})
	}
	content.Plans = []domain.PlanInput{plan}
	if err := domain.ValidateCatalogStructure(content); err != nil {
		return fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return nil
}

// insertPlanContentTx inserts a plan and its prices/entitlement grants.
func insertPlanContentTx(ctx context.Context, q *store.Queries, tc tenant.Ctx, versionID uuid.UUID, plan domain.PlanInput) (*storegen.Plan, error) {
	created, err := q.InsertPlan(ctx, storegen.InsertPlanParams{
		CatalogVersionID: versionID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		Code: plan.Code, Name: plan.Name, Interval: plan.Interval, Currency: plan.Currency,
	})
	if err != nil {
		return nil, mapErr(err, "plan %q", plan.Code)
	}
	if err := replacePlanChildrenTx(ctx, q, tc, versionID, created.ID, plan); err != nil {
		return nil, err
	}
	return &created, nil
}

// replacePlanChildrenTx rebuilds the prices and entitlement grants of a
// plan. Subscription rows pin plans by id, so the plan row itself is kept.
func replacePlanChildrenTx(ctx context.Context, q *store.Queries, tc tenant.Ctx, versionID, planID uuid.UUID, plan domain.PlanInput) error {
	if err := q.DeleteGrantsByPlan(ctx, planID); err != nil {
		return err
	}
	if err := q.DeletePricesByPlan(ctx, planID); err != nil {
		return err
	}
	metrics, err := q.ListMetricsByVersion(ctx, versionID)
	if err != nil {
		return err
	}
	metricIDs := make(map[string]uuid.UUID, len(metrics))
	for _, m := range metrics {
		metricIDs[m.Code] = m.ID
	}
	for _, pr := range plan.Prices {
		var metricID uuid.NullUUID
		if pr.MetricCode != "" {
			metricID = uuid.NullUUID{UUID: metricIDs[pr.MetricCode], Valid: true}
		}
		props := pr.Properties
		if len(props) == 0 {
			props = []byte(`{}`)
		}
		if _, err := q.InsertPrice(ctx, storegen.InsertPriceParams{
			PlanID: planID, CatalogVersionID: versionID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			MetricID: metricID, ChargeModel: pr.ChargeModel, Properties: props,
		}); err != nil {
			return mapErr(err, "price for plan %q", plan.Code)
		}
	}
	for _, e := range plan.Entitlements {
		if _, err := q.InsertEntitlementGrant(ctx, storegen.InsertEntitlementGrantParams{
			PlanID: planID, CatalogVersionID: versionID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			Key: e.Key, ValueType: e.ValueType, Value: e.Value,
		}); err != nil {
			return mapErr(err, "entitlement %q for plan %q", e.Key, plan.Code)
		}
	}
	return nil
}

// loadPlanDetailTx assembles a plan with its prices and grants.
func loadPlanDetailTx(ctx context.Context, q *store.Queries, plan storegen.Plan) (*PlanDetail, error) {
	prices, err := q.ListPricesByPlan(ctx, plan.ID)
	if err != nil {
		return nil, err
	}
	grants, err := q.ListGrantsByPlan(ctx, plan.ID)
	if err != nil {
		return nil, err
	}
	return &PlanDetail{Plan: plan, Prices: emptyIfNil(prices), Grants: emptyIfNil(grants)}, nil
}

// PlanPriceView is the Console-facing price shape. Unlike the raw storegen
// Price (which references metrics by id), metric references are resolved to
// codes so the Plans page can render and edit per_unit/tiered prices without
// a second lookup.
type PlanPriceView struct {
	ID          string          `json:"id"`
	ChargeModel string          `json:"charge_model"`
	MetricCode  string          `json:"metric_code,omitempty"`
	Properties  json.RawMessage `json:"properties"`
}

// PlanDetailView is the Console-facing plan detail: the plan row, its prices
// with resolved metric codes, and its entitlement grants.
type PlanDetailView struct {
	Plan   storegen.Plan               `json:"plan"`
	Prices []PlanPriceView             `json:"prices"`
	Grants []storegen.EntitlementGrant `json:"entitlement_grants"`
}

// PlanCollectionView is the full Plans page payload: every plan of the
// current draft version (falling back to the latest published version) plus
// the metrics available for pricing references. One request carries all data
// the page needs, avoiding an N+1 detail fan-out.
type PlanCollectionView struct {
	Plans   []PlanDetailView  `json:"plans"`
	Metrics []storegen.Metric `json:"metrics"`
}

// ListPlanDetails returns all plans of the current catalog version with their
// prices (metric codes resolved) and entitlement grants, plus the version's
// metrics. Read-only: a provider with no catalog version yields an empty
// collection instead of an error so the Plans page can show its empty state.
func (s *Service) ListPlanDetails(ctx context.Context, tc tenant.Ctx) (*PlanCollectionView, error) {
	var out PlanCollectionView
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		versionID, err := currentCatalogVersionIDTx(ctx, q, tc)
		if errors.Is(err, pgx.ErrNoRows) {
			out.Plans = []PlanDetailView{}
			out.Metrics = []storegen.Metric{}
			return nil
		}
		if err != nil {
			return err
		}
		plans, err := q.ListPlansByVersion(ctx, versionID)
		if err != nil {
			return err
		}
		metrics, err := q.ListMetricsByVersion(ctx, versionID)
		if err != nil {
			return err
		}
		prices, err := q.ListPricesByVersion(ctx, versionID)
		if err != nil {
			return err
		}
		grants, err := q.ListGrantsByVersion(ctx, versionID)
		if err != nil {
			return err
		}

		metricCodes := make(map[uuid.UUID]string, len(metrics))
		for _, m := range metrics {
			metricCodes[m.ID] = m.Code
		}
		pricesByPlan := make(map[uuid.UUID][]PlanPriceView, len(plans))
		for _, p := range prices {
			view := PlanPriceView{ID: p.ID.String(), ChargeModel: p.ChargeModel, Properties: p.Properties}
			if p.MetricID.Valid {
				view.MetricCode = metricCodes[p.MetricID.UUID]
			}
			pricesByPlan[p.PlanID] = append(pricesByPlan[p.PlanID], view)
		}
		grantsByPlan := make(map[uuid.UUID][]storegen.EntitlementGrant, len(plans))
		for _, g := range grants {
			grantsByPlan[g.PlanID] = append(grantsByPlan[g.PlanID], g)
		}

		out.Metrics = emptyIfNil(metrics)
		out.Plans = make([]PlanDetailView, 0, len(plans))
		for _, plan := range plans {
			out.Plans = append(out.Plans, PlanDetailView{
				Plan:   plan,
				Prices: emptyIfNil(pricesByPlan[plan.ID]),
				Grants: emptyIfNil(grantsByPlan[plan.ID]),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPlanDetail returns a single plan of the current catalog version in the
// Console-facing shape (metric codes resolved). Unknown plans yield
// ErrNotFound.
func (s *Service) GetPlanDetail(ctx context.Context, tc tenant.Ctx, code string) (*PlanDetailView, error) {
	var out PlanDetailView
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		versionID, err := currentCatalogVersionIDTx(ctx, q, tc)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: plan %q", ErrNotFound, code)
			}
			return err
		}
		plan, err := q.GetPlanByVersionAndCode(ctx, storegen.GetPlanByVersionAndCodeParams{
			CatalogVersionID: versionID, Code: code,
		})
		if err != nil {
			return mapErr(err, "plan %q", code)
		}
		prices, err := q.ListPricesByPlan(ctx, plan.ID)
		if err != nil {
			return err
		}
		grants, err := q.ListGrantsByPlan(ctx, plan.ID)
		if err != nil {
			return err
		}
		metrics, err := q.ListMetricsByVersion(ctx, versionID)
		if err != nil {
			return err
		}
		metricCodes := make(map[uuid.UUID]string, len(metrics))
		for _, m := range metrics {
			metricCodes[m.ID] = m.Code
		}
		views := make([]PlanPriceView, 0, len(prices))
		for _, p := range prices {
			view := PlanPriceView{ID: p.ID.String(), ChargeModel: p.ChargeModel, Properties: p.Properties}
			if p.MetricID.Valid {
				view.MetricCode = metricCodes[p.MetricID.UUID]
			}
			views = append(views, view)
		}
		out = PlanDetailView{Plan: plan, Prices: views, Grants: emptyIfNil(grants)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
