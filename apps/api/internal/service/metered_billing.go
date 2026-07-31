package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// MeteredPricingRuleInput is the parameter bundle for creating/updating a metered pricing rule.
type MeteredPricingRuleInput struct {
	MetricCode        string         `json:"metric_code"`
	PricingModel      string         `json:"pricing_model"`
	BasePriceCents    int64          `json:"base_price_cents"`
	TierConfig        []map[string]any `json:"tier_config"`
	MinimumSpendCents int64          `json:"minimum_spend_cents"`
	Enabled           bool           `json:"enabled"`
}

// SetMeteredPricingRule creates or updates a metered pricing rule.
func (s *Service) SetMeteredPricingRule(ctx context.Context, tc tenant.Ctx, in MeteredPricingRuleInput) (*storegen.MeteredPricingRule, error) {
	if in.MetricCode == "" {
		return nil, fmt.Errorf("%w: metric_code is required", ErrValidation)
	}
	if !domain.ValidPricingModel(in.PricingModel) {
		return nil, fmt.Errorf("%w: pricing_model must be per_unit, tiered, volume or stairstep", ErrValidation)
	}
	if in.BasePriceCents < 0 {
		return nil, fmt.Errorf("%w: base_price_cents must be non-negative", ErrValidation)
	}
	if in.MinimumSpendCents < 0 {
		return nil, fmt.Errorf("%w: minimum_spend_cents must be non-negative", ErrValidation)
	}

	tierBytes, err := json.Marshal(in.TierConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid tier_config: %v", ErrValidation, err)
	}

	var rule storegen.MeteredPricingRule
	err = s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		r, err := q.UpsertMeteredPricingRule(ctx, storegen.UpsertMeteredPricingRuleParams{
			ProviderID:        tc.ProviderID,
			EnvironmentID:     tc.EnvironmentID,
			MetricCode:        in.MetricCode,
			PricingModel:      in.PricingModel,
			BasePriceCents:    in.BasePriceCents,
			TierConfig:        tierBytes,
			MinimumSpendCents: in.MinimumSpendCents,
			Enabled:           in.Enabled,
		})
		if err != nil {
			return mapErr(err, "metered pricing rule %q", in.MetricCode)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "metered_pricing_rule", r.ID.String(), "metered_pricing.rule_set", map[string]any{
			"metric_code":   in.MetricCode,
			"pricing_model": in.PricingModel,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "metered_pricing.rule_set",
			"metered_pricing_rule", r.ID.String(),
			map[string]any{"metric_code": in.MetricCode, "pricing_model": in.PricingModel}); err != nil {
			return err
		}
		rule = r
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// GetMeteredPricingRule returns a pricing rule by metric code.
func (s *Service) GetMeteredPricingRule(ctx context.Context, tc tenant.Ctx, metricCode string) (*storegen.MeteredPricingRule, error) {
	var rule storegen.MeteredPricingRule
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		r, err := q.GetMeteredPricingRule(ctx, storegen.GetMeteredPricingRuleParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, MetricCode: metricCode,
		})
		if err != nil {
			return mapErr(err, "metered pricing rule %q", metricCode)
		}
		rule = r
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// ListMeteredPricingRules returns all enabled pricing rules.
func (s *Service) ListMeteredPricingRules(ctx context.Context, tc tenant.Ctx) ([]storegen.MeteredPricingRule, error) {
	var out []storegen.MeteredPricingRule
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rules, err := q.ListMeteredPricingRules(ctx, storegen.ListMeteredPricingRulesParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		out = rules
		return err
	})
	return out, err
}

// DeleteMeteredPricingRule removes a pricing rule.
func (s *Service) DeleteMeteredPricingRule(ctx context.Context, tc tenant.Ctx, metricCode string) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := q.DeleteMeteredPricingRule(ctx, storegen.DeleteMeteredPricingRuleParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, MetricCode: metricCode,
		})
		if err != nil {
			return mapErr(err, "delete metered pricing rule %q", metricCode)
		}
		if rows == 0 {
			return fmt.Errorf("%w: metered pricing rule %q", ErrNotFound, metricCode)
		}
		return insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "metered_pricing.rule_delete",
			"metered_pricing_rule", metricCode, nil)
	})
}

// CalculateMeteredCost calculates the cost for a given quantity using a pricing rule.
// Supports per_unit, tiered, volume, and stairstep pricing models.
func CalculateMeteredCost(rule *storegen.MeteredPricingRule, quantity int64) int64 {
	if rule == nil || !rule.Enabled {
		return 0
	}

	switch rule.PricingModel {
	case domain.PricingModelPerUnit:
		return rule.BasePriceCents * quantity

	case domain.PricingModelVolume:
		// Volume: the unit price for ALL units is the tier that the quantity falls into.
		return calculateVolumeCost(rule, quantity)

	case domain.PricingModelTiered:
		// Tiered: each tier's units are priced at that tier's rate.
		return calculateTieredCost(rule, quantity)

	case domain.PricingModelStairStep:
		// Stairstep: flat fee for the tier the quantity falls into.
		return calculateStairStepCost(rule, quantity)

	default:
		return rule.BasePriceCents * quantity
	}
}

// calculateVolumeCost: all units priced at the tier the quantity falls into.
func calculateVolumeCost(rule *storegen.MeteredPricingRule, quantity int64) int64 {
	var tiers []map[string]any
	if err := json.Unmarshal(rule.TierConfig, &tiers); err != nil || len(tiers) == 0 {
		return rule.BasePriceCents * quantity
	}
	pricePerUnit := rule.BasePriceCents
	for _, tier := range tiers {
		upTo, ok := tier["up_to"].(float64)
		if ok && quantity <= int64(upTo) {
			if p, ok := tier["price_cents"].(float64); ok {
				pricePerUnit = int64(p)
			}
			break
		}
		if p, ok := tier["price_cents"].(float64); ok {
			pricePerUnit = int64(p)
		}
	}
	return pricePerUnit * quantity
}

// calculateTieredCost: each tier's units priced at that tier's rate.
func calculateTieredCost(rule *storegen.MeteredPricingRule, quantity int64) int64 {
	var tiers []map[string]any
	if err := json.Unmarshal(rule.TierConfig, &tiers); err != nil || len(tiers) == 0 {
		return rule.BasePriceCents * quantity
	}
	total := int64(0)
	remaining := quantity
	prevLimit := int64(0)
	for _, tier := range tiers {
		upTo, ok := tier["up_to"].(float64)
		if !ok {
			continue
		}
		limit := int64(upTo)
		tierQty := limit - prevLimit
		tierQty = min(tierQty, remaining)
		if tierQty <= 0 {
			break
		}
		pricePerUnit := rule.BasePriceCents
		if p, ok := tier["price_cents"].(float64); ok {
			pricePerUnit = int64(p)
		}
		total += pricePerUnit * tierQty
		remaining -= tierQty
		prevLimit = limit
		if remaining <= 0 {
			break
		}
	}
	// Apply minimum spend.
	if total < rule.MinimumSpendCents {
		return rule.MinimumSpendCents
	}
	return total
}

// calculateStairStepCost: flat fee for the tier the quantity falls into.
func calculateStairStepCost(rule *storegen.MeteredPricingRule, quantity int64) int64 {
	var tiers []map[string]any
	if err := json.Unmarshal(rule.TierConfig, &tiers); err != nil || len(tiers) == 0 {
		return rule.BasePriceCents
	}
	flatFee := rule.BasePriceCents
	for _, tier := range tiers {
		upTo, ok := tier["up_to"].(float64)
		if ok && quantity <= int64(upTo) {
			if f, ok := tier["flat_fee_cents"].(float64); ok {
				flatFee = int64(f)
			}
			break
		}
		if f, ok := tier["flat_fee_cents"].(float64); ok {
			flatFee = int64(f)
		}
	}
	return flatFee
}

// --- Budget Alerts ---

// BudgetAlertInput is the parameter bundle for creating a budget alert.
type BudgetAlertInput struct {
	SubscriptionID *uuid.UUID
	MetricCode     string
	BudgetCents    int64
	ThresholdPct   float64
}

// CreateBudgetAlert creates a budget alert for a provider/subscription/metric.
func (s *Service) CreateBudgetAlert(ctx context.Context, tc tenant.Ctx, in BudgetAlertInput) (*storegen.BudgetAlert, error) {
	if in.BudgetCents <= 0 {
		return nil, fmt.Errorf("%w: budget_cents must be positive", ErrValidation)
	}
	if in.ThresholdPct <= 0 || in.ThresholdPct > 100 {
		return nil, fmt.Errorf("%w: threshold_pct must be between 0 and 100", ErrValidation)
	}

	var alert storegen.BudgetAlert
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		var subID uuid.NullUUID
		if in.SubscriptionID != nil {
			subID = uuid.NullUUID{UUID: *in.SubscriptionID, Valid: true}
		}
		var metricCode pgtype.Text
		if in.MetricCode != "" {
			metricCode = pgtype.Text{String: in.MetricCode, Valid: true}
		}
		a, err := q.CreateBudgetAlert(ctx, storegen.CreateBudgetAlertParams{
			ProviderID:     tc.ProviderID,
			EnvironmentID:  tc.EnvironmentID,
			SubscriptionID: subID,
			MetricCode:     metricCode,
			BudgetCents:    in.BudgetCents,
			ThresholdPct:   in.ThresholdPct,
		})
		if err != nil {
			return mapErr(err, "budget alert")
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "budget_alert", a.ID.String(), "budget.alert_created", map[string]any{
			"budget_cents":  in.BudgetCents,
			"threshold_pct": in.ThresholdPct,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "budget.alert_create",
			"budget_alert", a.ID.String(),
			map[string]any{"budget_cents": in.BudgetCents}); err != nil {
			return err
		}
		alert = a
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &alert, nil
}

// GetBudgetAlert returns a budget alert by ID.
func (s *Service) GetBudgetAlert(ctx context.Context, tc tenant.Ctx, alertID uuid.UUID) (*storegen.BudgetAlert, error) {
	var alert storegen.BudgetAlert
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		a, err := q.GetBudgetAlertByID(ctx, alertID)
		if err != nil {
			return mapErr(err, "budget alert %s", alertID)
		}
		if err := checkTenantOwnership(a.ProviderID, a.EnvironmentID, tc, "budget alert", alertID); err != nil {
			return err
		}
		alert = a
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &alert, nil
}

// ListBudgetAlerts returns all budget alerts for the caller's tenant.
func (s *Service) ListBudgetAlerts(ctx context.Context, tc tenant.Ctx) ([]storegen.BudgetAlert, error) {
	var out []storegen.BudgetAlert
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		alerts, err := q.ListBudgetAlerts(ctx, storegen.ListBudgetAlertsParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Limit: 100,
		})
		out = alerts
		return err
	})
	return out, err
}

// DeleteBudgetAlert removes a budget alert.
func (s *Service) DeleteBudgetAlert(ctx context.Context, tc tenant.Ctx, alertID uuid.UUID) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := q.DeleteBudgetAlert(ctx, storegen.DeleteBudgetAlertParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ID: alertID,
		})
		if err != nil {
			return mapErr(err, "delete budget alert %s", alertID)
		}
		if rows == 0 {
			return fmt.Errorf("%w: budget alert %s", ErrNotFound, alertID)
		}
		return insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "budget.alert_delete",
			"budget_alert", alertID.String(), nil)
	})
}
