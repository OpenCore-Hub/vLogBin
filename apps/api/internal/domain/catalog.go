package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Catalog lifecycle states: DRAFT → VALIDATED → PUBLISHED → RETIRED.
// Published versions are immutable (enforced at the database level by
// triggers); price and structure changes require a new version.
type CatalogState string

const (
	CatalogDraft     CatalogState = "draft"
	CatalogValidated CatalogState = "validated"
	CatalogPublished CatalogState = "published"
	CatalogRetired   CatalogState = "retired"
)

// Aggregation types for billable metrics.
const (
	AggregationCount       = "count"
	AggregationSum         = "sum"
	AggregationMax         = "max"
	AggregationUniqueCount = "unique_count"
)

// Plan intervals.
const (
	IntervalWeekly  = "weekly"
	IntervalMonthly = "monthly"
	IntervalYearly  = "yearly"
)

// Charge models.
const (
	ChargeModelFixed   = "fixed"
	ChargeModelPerUnit = "per_unit"
	ChargeModelTiered  = "tiered"
)

// Entitlement value types.
const (
	ValueTypeBoolean = "boolean"
	ValueTypeNumeric = "numeric"
	ValueTypePeriod  = "period"
)

// Customer account types. B2C individuals are first-class accounts without
// organizations (spec decision #10).
const (
	AccountTypeIndividual = "individual"
	AccountTypeBusiness   = "business"
)

// Subscription statuses.
const (
	SubscriptionActive     = "active"
	SubscriptionTerminated = "terminated"
)

// Usage event kinds. Originals are immutable; corrections are reversals.
const (
	UsageKindIngestion = "ingestion"
	UsageKindReversal  = "reversal"
)

// Entitlement sources in an evaluation result.
const (
	EntitlementSourcePlan     = "plan"
	EntitlementSourceOverride = "override"
)

var catalogTransitions = map[CatalogState][]CatalogState{
	CatalogDraft:     {CatalogValidated},
	CatalogValidated: {CatalogPublished},
	CatalogPublished: {CatalogRetired},
	CatalogRetired:   {},
}

// TransitionCatalog validates a catalog lifecycle transition, returning the
// target state or ErrInvalidTransition (mapped to 409 by the API layer).
func TransitionCatalog(from, to CatalogState) (CatalogState, error) {
	for _, s := range catalogTransitions[from] {
		if s == to {
			return to, nil
		}
	}
	return "", fmt.Errorf("%w: catalog %s -> %s", ErrInvalidTransition, from, to)
}

func validAggregationType(s string) bool {
	switch s {
	case AggregationCount, AggregationSum, AggregationMax, AggregationUniqueCount:
		return true
	}
	return false
}

func validInterval(s string) bool {
	switch s {
	case IntervalWeekly, IntervalMonthly, IntervalYearly:
		return true
	}
	return false
}

func validChargeModel(s string) bool {
	switch s {
	case ChargeModelFixed, ChargeModelPerUnit, ChargeModelTiered:
		return true
	}
	return false
}

// ValidValueType reports whether s is an entitlement value type.
func ValidValueType(s string) bool {
	switch s {
	case ValueTypeBoolean, ValueTypeNumeric, ValueTypePeriod:
		return true
	}
	return false
}

// ValidAccountType reports whether s is a customer account type.
func ValidAccountType(s string) bool {
	return s == AccountTypeIndividual || s == AccountTypeBusiness
}

// ---- catalog content (input model for PUT + validation) ----

type MetricInput struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	AggregationType string `json:"aggregation_type"`
	FieldName       string `json:"field_name,omitempty"`
	Billable        *bool  `json:"billable,omitempty"`
}

type PriceInput struct {
	MetricCode  string          `json:"metric_code,omitempty"`
	ChargeModel string          `json:"charge_model"`
	Properties  json.RawMessage `json:"properties"`
}

type EntitlementInput struct {
	Key       string          `json:"key"`
	ValueType string          `json:"value_type"`
	Value     json.RawMessage `json:"value"`
}

type PlanInput struct {
	Code         string             `json:"code"`
	Name         string             `json:"name"`
	Interval     string             `json:"interval"`
	Currency     string             `json:"currency"`
	Prices       []PriceInput       `json:"prices"`
	Entitlements []EntitlementInput `json:"entitlements,omitempty"`
}

// CatalogContent is the full replaceable content of a draft version.
type CatalogContent struct {
	Metrics []MetricInput `json:"metrics"`
	Plans   []PlanInput   `json:"plans"`
}

type fixedPriceProperties struct {
	AmountCents *int64 `json:"amount_cents"`
	Currency    string `json:"currency"`
}

type perUnitPriceProperties struct {
	UnitAmountCents *int64 `json:"unit_amount_cents"`
}

type tieredPriceProperties struct {
	Tiers []priceTier `json:"tiers"`
}

type priceTier struct {
	FromValue       *int64 `json:"from_value"`
	ToValue         *int64 `json:"to_value"`
	UnitAmountCents *int64 `json:"unit_amount_cents"`
}

// ValidateCatalog enforces the publish-time content rules (structure plus
// completeness). It returns the first violation found; the service layer
// wraps it as a 400.
func ValidateCatalog(c CatalogContent) error {
	return validateCatalog(c, true)
}

// ValidateCatalogStructure enforces only the structural rules (codes, enums,
// property shapes), so a draft may be saved incomplete and completed later;
// completeness (at least one plan, at least one price per plan) is checked
// at the validate transition.
func ValidateCatalogStructure(c CatalogContent) error {
	return validateCatalog(c, false)
}

func validateCatalog(c CatalogContent, requireComplete bool) error {
	metricCodes := map[string]bool{}
	for _, m := range c.Metrics {
		if m.Code == "" {
			return fmt.Errorf("metric code is required")
		}
		if metricCodes[m.Code] {
			return fmt.Errorf("duplicate metric code %q", m.Code)
		}
		metricCodes[m.Code] = true
		if !validAggregationType(m.AggregationType) {
			return fmt.Errorf("metric %q: unknown aggregation_type %q", m.Code, m.AggregationType)
		}
	}
	if requireComplete && len(c.Plans) == 0 {
		return fmt.Errorf("catalog must contain at least one plan")
	}
	planCodes := map[string]bool{}
	for _, p := range c.Plans {
		if p.Code == "" {
			return fmt.Errorf("plan code is required")
		}
		if planCodes[p.Code] {
			return fmt.Errorf("duplicate plan code %q", p.Code)
		}
		planCodes[p.Code] = true
		if !validInterval(p.Interval) {
			return fmt.Errorf("plan %q: unknown interval %q", p.Code, p.Interval)
		}
		if len(p.Currency) != 3 {
			return fmt.Errorf("plan %q: currency must be a 3-letter code", p.Code)
		}
		if requireComplete && len(p.Prices) == 0 {
			return fmt.Errorf("plan %q: at least one price is required", p.Code)
		}
		for _, pr := range p.Prices {
			if err := validatePrice(p, pr, metricCodes); err != nil {
				return err
			}
		}
		grantKeys := map[string]bool{}
		for _, e := range p.Entitlements {
			if e.Key == "" {
				return fmt.Errorf("plan %q: entitlement key is required", p.Code)
			}
			if grantKeys[e.Key] {
				return fmt.Errorf("plan %q: duplicate entitlement key %q", p.Code, e.Key)
			}
			grantKeys[e.Key] = true
			if !ValidValueType(e.ValueType) {
				return fmt.Errorf("plan %q: entitlement %q: unknown value_type %q", p.Code, e.Key, e.ValueType)
			}
			if len(e.Value) == 0 || string(e.Value) == "null" {
				return fmt.Errorf("plan %q: entitlement %q: value is required", p.Code, e.Key)
			}
		}
	}
	return nil
}

func validatePrice(plan PlanInput, pr PriceInput, metricCodes map[string]bool) error {
	if !validChargeModel(pr.ChargeModel) {
		return fmt.Errorf("plan %q: unknown charge_model %q", plan.Code, pr.ChargeModel)
	}
	switch pr.ChargeModel {
	case ChargeModelFixed:
		var props fixedPriceProperties
		if err := json.Unmarshal(pr.Properties, &props); err != nil {
			return fmt.Errorf("plan %q: fixed price properties: %v", plan.Code, err)
		}
		if props.AmountCents == nil || *props.AmountCents < 0 {
			return fmt.Errorf("plan %q: fixed price requires properties.amount_cents >= 0", plan.Code)
		}
		if props.Currency != plan.Currency {
			return fmt.Errorf("plan %q: fixed price currency %q does not match plan currency %q", plan.Code, props.Currency, plan.Currency)
		}
	case ChargeModelPerUnit:
		if pr.MetricCode == "" || !metricCodes[pr.MetricCode] {
			return fmt.Errorf("plan %q: per_unit price references unknown metric %q", plan.Code, pr.MetricCode)
		}
		var props perUnitPriceProperties
		if err := json.Unmarshal(pr.Properties, &props); err != nil {
			return fmt.Errorf("plan %q: per_unit price properties: %v", plan.Code, err)
		}
		if props.UnitAmountCents == nil || *props.UnitAmountCents < 0 {
			return fmt.Errorf("plan %q: per_unit price requires properties.unit_amount_cents >= 0", plan.Code)
		}
	case ChargeModelTiered:
		if pr.MetricCode == "" || !metricCodes[pr.MetricCode] {
			return fmt.Errorf("plan %q: tiered price references unknown metric %q", plan.Code, pr.MetricCode)
		}
		var props tieredPriceProperties
		if err := json.Unmarshal(pr.Properties, &props); err != nil {
			return fmt.Errorf("plan %q: tiered price properties: %v", plan.Code, err)
		}
		if err := validateTiers(plan.Code, props.Tiers); err != nil {
			return err
		}
	}
	return nil
}

// validateTiers enforces: sorted by from_value, starting at 0, contiguous,
// last tier open-ended (to_value null).
func validateTiers(planCode string, tiers []priceTier) error {
	if len(tiers) == 0 {
		return fmt.Errorf("plan %q: tiered price requires at least one tier", planCode)
	}
	for i, t := range tiers {
		if t.FromValue == nil {
			return fmt.Errorf("plan %q: tier %d: from_value is required", planCode, i)
		}
		if i == 0 && *t.FromValue != 0 {
			return fmt.Errorf("plan %q: tiers must start at from_value 0", planCode)
		}
		if i > 0 {
			prev := tiers[i-1]
			if prev.ToValue == nil {
				return fmt.Errorf("plan %q: tier %d: only the last tier may be open-ended", planCode, i-1)
			}
			if *prev.ToValue != *t.FromValue {
				return fmt.Errorf("plan %q: tiers must be contiguous and sorted (tier %d ends at %d, tier %d starts at %d)",
					planCode, i-1, *prev.ToValue, i, *t.FromValue)
			}
		}
		if t.ToValue != nil && *t.ToValue <= *t.FromValue {
			return fmt.Errorf("plan %q: tier %d: to_value must be greater than from_value", planCode, i)
		}
	}
	if tiers[len(tiers)-1].ToValue != nil {
		return fmt.Errorf("plan %q: last tier must be open-ended (to_value null)", planCode)
	}
	return nil
}

// ---- entitlement evaluation ----

// GrantKV is a plan entitlement grant from a pinned catalog version.
type GrantKV struct {
	Key       string
	ValueType string
	Value     json.RawMessage
}

// OverrideKV is a subscription-level entitlement override.
type OverrideKV struct {
	Key       string
	ValueType string
	Value     json.RawMessage
	ExpiresAt *time.Time
}

// EvaluatedEntitlement is one entry of an entitlement snapshot.
type EvaluatedEntitlement struct {
	ValueType string          `json:"value_type"`
	Value     json.RawMessage `json:"value"`
	Source    string          `json:"source"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
}

// EvaluateEntitlements merges plan grants with subscription overrides: a
// non-expired override wins per key; expired overrides are ignored and the
// plan grant (if any) applies.
func EvaluateEntitlements(grants []GrantKV, overrides []OverrideKV, now time.Time) map[string]EvaluatedEntitlement {
	out := make(map[string]EvaluatedEntitlement, len(grants)+len(overrides))
	for _, g := range grants {
		out[g.Key] = EvaluatedEntitlement{
			ValueType: g.ValueType,
			Value:     g.Value,
			Source:    EntitlementSourcePlan,
		}
	}
	for _, o := range overrides {
		if o.ExpiresAt != nil && !now.Before(*o.ExpiresAt) {
			continue // expired: plan grant (or nothing) applies
		}
		out[o.Key] = EvaluatedEntitlement{
			ValueType: o.ValueType,
			Value:     o.Value,
			Source:    EntitlementSourceOverride,
			ExpiresAt: o.ExpiresAt,
		}
	}
	return out
}
