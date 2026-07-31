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
	"github.com/jackc/pgx/v5/pgtype"
)

// CatalogVersionDetail is a catalog version with its full content.
type CatalogVersionDetail struct {
	Version storegen.CatalogVersion     `json:"version"`
	Metrics []storegen.Metric           `json:"metrics"`
	Plans   []storegen.Plan             `json:"plans"`
	Prices  []storegen.Price            `json:"prices"`
	Grants  []storegen.EntitlementGrant `json:"entitlement_grants"`
}

// CreateCatalogVersion creates the next draft version for the tenant
// (version = max+1). When fromVersionID is set, the new draft is a deep
// copy (clone) of that version's metrics/plans/prices/grants.
func (s *Service) CreateCatalogVersion(ctx context.Context, tc tenant.Ctx, fromVersionID *uuid.UUID) (*storegen.CatalogVersion, error) {
	var out storegen.CatalogVersion
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		next, err := q.NextCatalogVersionNumber(ctx, storegen.NextCatalogVersionNumberParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return err
		}
		version, err := q.InsertCatalogVersion(ctx, storegen.InsertCatalogVersionParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Version: next,
		})
		if err != nil {
			return mapErr(err, "catalog version %d", next)
		}
		if fromVersionID != nil {
			if err := cloneCatalogContentTx(ctx, q, tc, *fromVersionID, version.ID); err != nil {
				return err
			}
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "catalog_version", version.ID.String(), "catalog.version_created", map[string]any{
			"catalog_version_id": version.ID.String(), "version": version.Version,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(), "credential", tc.CredentialID.String(),
			"catalog.version_create", "catalog_version", version.ID.String(),
			map[string]any{"version": version.Version}); err != nil {
			return err
		}
		out = version
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// cloneCatalogContentTx deep-copies the content of the source version into
// the target (draft) version inside the current transaction.
func cloneCatalogContentTx(ctx context.Context, q *store.Queries, tc tenant.Ctx, fromID, toID uuid.UUID) error {
	if _, err := q.GetCatalogVersionByID(ctx, fromID); err != nil {
		return mapErr(err, "catalog version %s", fromID)
	}
	metrics, err := q.ListMetricsByVersion(ctx, fromID)
	if err != nil {
		return err
	}
	plans, err := q.ListPlansByVersion(ctx, fromID)
	if err != nil {
		return err
	}
	prices, err := q.ListPricesByVersion(ctx, fromID)
	if err != nil {
		return err
	}
	grants, err := q.ListGrantsByVersion(ctx, fromID)
	if err != nil {
		return err
	}
	metricIDs := map[uuid.UUID]uuid.UUID{}
	for _, m := range metrics {
		nm, err := q.InsertMetric(ctx, storegen.InsertMetricParams{
			CatalogVersionID: toID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			Code: m.Code, Name: m.Name, AggregationType: m.AggregationType, FieldName: m.FieldName, Billable: m.Billable,
		})
		if err != nil {
			return err
		}
		metricIDs[m.ID] = nm.ID
	}
	planIDs := map[uuid.UUID]uuid.UUID{}
	for _, p := range plans {
		np, err := q.InsertPlan(ctx, storegen.InsertPlanParams{
			CatalogVersionID: toID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			Code: p.Code, Name: p.Name, Interval: p.Interval, Currency: p.Currency,
		})
		if err != nil {
			return err
		}
		planIDs[p.ID] = np.ID
	}
	for _, pr := range prices {
		var metricID uuid.NullUUID
		if pr.MetricID.Valid {
			metricID = uuid.NullUUID{UUID: metricIDs[pr.MetricID.UUID], Valid: true}
		}
		if _, err := q.InsertPrice(ctx, storegen.InsertPriceParams{
			PlanID: planIDs[pr.PlanID], CatalogVersionID: toID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			MetricID: metricID, ChargeModel: pr.ChargeModel, Properties: pr.Properties,
		}); err != nil {
			return err
		}
	}
	for _, g := range grants {
		if _, err := q.InsertEntitlementGrant(ctx, storegen.InsertEntitlementGrantParams{
			PlanID: planIDs[g.PlanID], CatalogVersionID: toID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			Key: g.Key, ValueType: g.ValueType, Value: g.Value,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ListCatalogVersions(ctx context.Context, tc tenant.Ctx) ([]storegen.CatalogVersion, error) {
	var out []storegen.CatalogVersion
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		vs, err := q.ListCatalogVersionsByTenant(ctx, storegen.ListCatalogVersionsByTenantParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		out = vs
		return err
	})
	return out, err
}

// GetCatalogVersion returns a version with its full content.
func (s *Service) GetCatalogVersion(ctx context.Context, tc tenant.Ctx, id uuid.UUID) (*CatalogVersionDetail, error) {
	var out CatalogVersionDetail
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		detail, err := loadCatalogDetailTx(ctx, q, id)
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

func loadCatalogDetailTx(ctx context.Context, q *store.Queries, id uuid.UUID) (*CatalogVersionDetail, error) {
	version, err := q.GetCatalogVersionByID(ctx, id)
	if err != nil {
		return nil, mapErr(err, "catalog version %s", id)
	}
	metrics, err := q.ListMetricsByVersion(ctx, id)
	if err != nil {
		return nil, err
	}
	plans, err := q.ListPlansByVersion(ctx, id)
	if err != nil {
		return nil, err
	}
	prices, err := q.ListPricesByVersion(ctx, id)
	if err != nil {
		return nil, err
	}
	grants, err := q.ListGrantsByVersion(ctx, id)
	if err != nil {
		return nil, err
	}
	return &CatalogVersionDetail{
		Version: version,
		Metrics: emptyIfNil(metrics),
		Plans:   emptyIfNil(plans),
		Prices:  emptyIfNil(prices),
		Grants:  emptyIfNil(grants),
	}, nil
}

func emptyIfNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// ReplaceCatalogContent atomically replaces the content of a draft version
// (metrics + plans + prices + grants) in one transaction.
func (s *Service) ReplaceCatalogContent(ctx context.Context, tc tenant.Ctx, id uuid.UUID, content domain.CatalogContent) (*CatalogVersionDetail, error) {
	if err := domain.ValidateCatalogStructure(content); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	var out CatalogVersionDetail
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		version, err := q.GetCatalogVersionByID(ctx, id)
		if err != nil {
			return mapErr(err, "catalog version %s", id)
		}
		if version.State != string(domain.CatalogDraft) {
			return fmt.Errorf("%w: catalog version %s is %s: content replace requires a draft", ErrConflict, id, version.State)
		}
		if err := replaceCatalogContentTx(ctx, q, tc, id, content); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(), "credential", tc.CredentialID.String(),
			"catalog.content_replace", "catalog_version", id.String(),
			map[string]any{"version": version.Version}); err != nil {
			return err
		}
		detail, err := loadCatalogDetailTx(ctx, q, id)
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

func replaceCatalogContentTx(ctx context.Context, q *store.Queries, tc tenant.Ctx, versionID uuid.UUID, content domain.CatalogContent) error {
	// Children first: prices and grants reference plans.
	if err := q.DeleteGrantsByVersion(ctx, versionID); err != nil {
		return err
	}
	if err := q.DeletePricesByVersion(ctx, versionID); err != nil {
		return err
	}
	if err := q.DeletePlansByVersion(ctx, versionID); err != nil {
		return err
	}
	if err := q.DeleteMetricsByVersion(ctx, versionID); err != nil {
		return err
	}
	metricIDs := map[string]uuid.UUID{}
	for _, m := range content.Metrics {
		billable := true
		if m.Billable != nil {
			billable = *m.Billable
		}
		nm, err := q.InsertMetric(ctx, storegen.InsertMetricParams{
			CatalogVersionID: versionID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			Code: m.Code, Name: m.Name, AggregationType: m.AggregationType,
			FieldName: pgtype.Text{String: m.FieldName, Valid: m.FieldName != ""},
			Billable:  billable,
		})
		if err != nil {
			return mapErr(err, "metric %q", m.Code)
		}
		metricIDs[m.Code] = nm.ID
	}
	for _, p := range content.Plans {
		np, err := q.InsertPlan(ctx, storegen.InsertPlanParams{
			CatalogVersionID: versionID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			Code: p.Code, Name: p.Name, Interval: p.Interval, Currency: p.Currency,
		})
		if err != nil {
			return mapErr(err, "plan %q", p.Code)
		}
		for _, pr := range p.Prices {
			var metricID uuid.NullUUID
			if pr.MetricCode != "" {
				metricID = uuid.NullUUID{UUID: metricIDs[pr.MetricCode], Valid: true}
			}
			props := pr.Properties
			if len(props) == 0 {
				props = []byte(`{}`)
			}
			if _, err := q.InsertPrice(ctx, storegen.InsertPriceParams{
				PlanID: np.ID, CatalogVersionID: versionID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
				MetricID: metricID, ChargeModel: pr.ChargeModel, Properties: props,
			}); err != nil {
				return err
			}
		}
		for _, e := range p.Entitlements {
			if _, err := q.InsertEntitlementGrant(ctx, storegen.InsertEntitlementGrantParams{
				PlanID: np.ID, CatalogVersionID: versionID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
				Key: e.Key, ValueType: e.ValueType, Value: e.Value,
			}); err != nil {
				return mapErr(err, "entitlement %q", e.Key)
			}
		}
	}
	return nil
}

// storedContentTx rebuilds the domain content model of a version for
// validation (metric references are restored to codes).
func storedContentTx(ctx context.Context, q *store.Queries, versionID uuid.UUID) (domain.CatalogContent, error) {
	metrics, err := q.ListMetricsByVersion(ctx, versionID)
	if err != nil {
		return domain.CatalogContent{}, err
	}
	plans, err := q.ListPlansByVersion(ctx, versionID)
	if err != nil {
		return domain.CatalogContent{}, err
	}
	prices, err := q.ListPricesByVersion(ctx, versionID)
	if err != nil {
		return domain.CatalogContent{}, err
	}
	grants, err := q.ListGrantsByVersion(ctx, versionID)
	if err != nil {
		return domain.CatalogContent{}, err
	}
	metricCodes := map[uuid.UUID]string{}
	content := domain.CatalogContent{}
	for _, m := range metrics {
		metricCodes[m.ID] = m.Code
		content.Metrics = append(content.Metrics, domain.MetricInput{
			Code: m.Code, Name: m.Name, AggregationType: m.AggregationType,
			FieldName: m.FieldName.String, Billable: &m.Billable,
		})
	}
	pricesByPlan := map[uuid.UUID][]domain.PriceInput{}
	for _, pr := range prices {
		pi := domain.PriceInput{ChargeModel: pr.ChargeModel, Properties: pr.Properties}
		if pr.MetricID.Valid {
			pi.MetricCode = metricCodes[pr.MetricID.UUID]
		}
		pricesByPlan[pr.PlanID] = append(pricesByPlan[pr.PlanID], pi)
	}
	grantsByPlan := map[uuid.UUID][]domain.EntitlementInput{}
	for _, g := range grants {
		grantsByPlan[g.PlanID] = append(grantsByPlan[g.PlanID], domain.EntitlementInput{
			Key: g.Key, ValueType: g.ValueType, Value: g.Value,
		})
	}
	for _, p := range plans {
		content.Plans = append(content.Plans, domain.PlanInput{
			Code: p.Code, Name: p.Name, Interval: p.Interval, Currency: p.Currency,
			Prices: pricesByPlan[p.ID], Entitlements: grantsByPlan[p.ID],
		})
	}
	return content, nil
}

// ValidateCatalogVersion runs the publish-gate content rules and moves the
// version draft → validated.
func (s *Service) ValidateCatalogVersion(ctx context.Context, tc tenant.Ctx, id uuid.UUID) (*storegen.CatalogVersion, error) {
	return s.transitionCatalogVersion(ctx, tc, id, domain.CatalogValidated, func(ctx context.Context, q *store.Queries) error {
		content, err := storedContentTx(ctx, q, id)
		if err != nil {
			return err
		}
		if err := domain.ValidateCatalog(content); err != nil {
			return fmt.Errorf("%w: %v", ErrValidation, err)
		}
		return nil
	})
}

// PublishCatalogVersion moves a validated version to published. From this
// point its content is immutable (DB triggers enforce it).
func (s *Service) PublishCatalogVersion(ctx context.Context, tc tenant.Ctx, id uuid.UUID) (*storegen.CatalogVersion, error) {
	return s.transitionCatalogVersion(ctx, tc, id, domain.CatalogPublished, nil)
}

// RetireCatalogVersion retires a published version.
func (s *Service) RetireCatalogVersion(ctx context.Context, tc tenant.Ctx, id uuid.UUID) (*storegen.CatalogVersion, error) {
	return s.transitionCatalogVersion(ctx, tc, id, domain.CatalogRetired, nil)
}

func (s *Service) transitionCatalogVersion(ctx context.Context, tc tenant.Ctx, id uuid.UUID, to domain.CatalogState, check func(context.Context, *store.Queries) error) (*storegen.CatalogVersion, error) {
	var out storegen.CatalogVersion
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		version, err := q.GetCatalogVersionByID(ctx, id)
		if err != nil {
			return mapErr(err, "catalog version %s", id)
		}
		next, err := domain.TransitionCatalog(domain.CatalogState(version.State), to)
		if err != nil {
			return err
		}
		if check != nil {
			if err := check(ctx, q); err != nil {
				return err
			}
		}
		version, err = q.UpdateCatalogVersionState(ctx, storegen.UpdateCatalogVersionStateParams{
			ID: id, State: string(next),
		})
		if err != nil {
			return err
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "catalog_version", id.String(),
			"catalog.version_"+string(next), map[string]any{
				"catalog_version_id": id.String(), "version": version.Version, "state": string(next),
			}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(), "credential", tc.CredentialID.String(),
			"catalog.version_"+string(next), "catalog_version", id.String(),
			map[string]any{"version": version.Version, "state": string(next)}); err != nil {
			return err
		}
		out = version
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ---- operator views ----

// ListCatalogVersionsByProvider lists versions across all environments of a
// provider (operator path, for the console).
func (s *Service) ListCatalogVersionsByProvider(ctx context.Context, providerID uuid.UUID) ([]storegen.ListCatalogVersionsByProviderRow, error) {
	var out []storegen.ListCatalogVersionsByProviderRow
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		vs, err := q.ListCatalogVersionsByProvider(ctx, providerID)
		out = vs
		return err
	})
	return out, err
}
