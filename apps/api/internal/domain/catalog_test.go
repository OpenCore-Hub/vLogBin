package domain

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// ---- Catalog lifecycle ----

func TestTransitionCatalog(t *testing.T) {
	tests := []struct {
		name string
		from CatalogState
		to   CatalogState
		ok   bool
	}{
		{"draft to validated", CatalogDraft, CatalogValidated, true},
		{"validated to published", CatalogValidated, CatalogPublished, true},
		{"published to retired", CatalogPublished, CatalogRetired, true},
		{"draft to published (skip)", CatalogDraft, CatalogPublished, false},
		{"retired to draft", CatalogRetired, CatalogDraft, false},
		{"published to draft", CatalogPublished, CatalogDraft, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TransitionCatalog(tt.from, tt.to)
			if tt.ok {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tt.to {
					t.Fatalf("got %v, want %v", got, tt.to)
				}
			} else {
				if !errors.Is(err, ErrInvalidTransition) {
					t.Fatalf("expected ErrInvalidTransition, got %v", err)
				}
			}
		})
	}
}

// ---- Catalog validation ----

func TestValidateCatalogValid(t *testing.T) {
	amt := int64(9900)
	unitAmt := int64(10)
	c := CatalogContent{
		Metrics: []MetricInput{
			{Code: "api_calls", Name: "API Calls", AggregationType: AggregationCount},
		},
		Plans: []PlanInput{
			{
				Code: "pro", Name: "Pro", Interval: IntervalMonthly, Currency: "USD",
				Prices: []PriceInput{
					{ChargeModel: ChargeModelFixed, Properties: mustJSON(t, fixedPriceProperties{AmountCents: &amt, Currency: "USD"})},
					{MetricCode: "api_calls", ChargeModel: ChargeModelPerUnit, Properties: mustJSON(t, perUnitPriceProperties{UnitAmountCents: &unitAmt})},
				},
			},
		},
	}
	if err := ValidateCatalog(c); err != nil {
		t.Fatalf("ValidateCatalog: %v", err)
	}
}

func TestValidateCatalogStructureDraft(t *testing.T) {
	// Draft mode: no plans required
	c := CatalogContent{
		Metrics: []MetricInput{
			{Code: "m1", Name: "M1", AggregationType: AggregationSum},
		},
	}
	if err := ValidateCatalogStructure(c); err != nil {
		t.Fatalf("ValidateCatalogStructure: %v", err)
	}
}

func TestValidateCatalogErrors(t *testing.T) {
	tests := []struct {
		name string
		c    CatalogContent
	}{
		{"empty metric code", CatalogContent{Metrics: []MetricInput{{Code: "", AggregationType: AggregationCount}}}},
		{"duplicate metric code", CatalogContent{Metrics: []MetricInput{
			{Code: "m1", AggregationType: AggregationCount},
			{Code: "m1", AggregationType: AggregationCount},
		}}},
		{"invalid aggregation", CatalogContent{Metrics: []MetricInput{{Code: "m1", AggregationType: "invalid"}}}},
		{"no plans (publish)", CatalogContent{Metrics: []MetricInput{{Code: "m1", AggregationType: AggregationCount}}}},
		{"empty plan code", CatalogContent{Plans: []PlanInput{{Code: "", Interval: IntervalMonthly, Currency: "USD"}}}},
		{"duplicate plan code", CatalogContent{Plans: []PlanInput{
			{Code: "p1", Interval: IntervalMonthly, Currency: "USD"},
			{Code: "p1", Interval: IntervalMonthly, Currency: "USD"},
		}}},
		{"invalid interval", CatalogContent{Plans: []PlanInput{{Code: "p1", Interval: "hourly", Currency: "USD"}}}},
		{"currency too short", CatalogContent{Plans: []PlanInput{{Code: "p1", Interval: IntervalMonthly, Currency: "US"}}}},
		{"no prices (publish)", CatalogContent{Plans: []PlanInput{{Code: "p1", Interval: IntervalMonthly, Currency: "USD"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCatalog(tt.c)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestValidatePriceErrors(t *testing.T) {
	amt := int64(-1)
	unitAmt := int64(-1)

	tests := []struct {
		name  string
		plan  PlanInput
		price PriceInput
	}{
		{"invalid charge model", PlanInput{Code: "p1", Currency: "USD"}, PriceInput{ChargeModel: "invalid"}},
		{"fixed negative amount", PlanInput{Code: "p1", Currency: "USD"}, PriceInput{ChargeModel: ChargeModelFixed, Properties: mustJSON(t, fixedPriceProperties{AmountCents: &amt, Currency: "USD"})}},
		{"fixed currency mismatch", PlanInput{Code: "p1", Currency: "EUR"}, PriceInput{ChargeModel: ChargeModelFixed, Properties: mustJSON(t, fixedPriceProperties{AmountCents: ptrInt64(100), Currency: "USD"})}},
		{"per_unit unknown metric", PlanInput{Code: "p1", Currency: "USD"}, PriceInput{ChargeModel: ChargeModelPerUnit, MetricCode: "unknown", Properties: mustJSON(t, perUnitPriceProperties{UnitAmountCents: ptrInt64(10)})}},
		{"per_unit negative", PlanInput{Code: "p1", Currency: "USD"}, PriceInput{ChargeModel: ChargeModelPerUnit, MetricCode: "m1", Properties: mustJSON(t, perUnitPriceProperties{UnitAmountCents: &unitAmt})}},
		{"tiered unknown metric", PlanInput{Code: "p1", Currency: "USD"}, PriceInput{ChargeModel: ChargeModelTiered, MetricCode: "unknown"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := map[string]bool{"m1": true}
			err := validatePrice(tt.plan, tt.price, metrics)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestValidateTiers(t *testing.T) {
	zero := int64(0)
	hundred := int64(100)

	tests := []struct {
		name   string
		tiers  []priceTier
		hasErr bool
	}{
		{"empty tiers", []priceTier{}, true},
		{"valid single tier", []priceTier{{FromValue: &zero, ToValue: nil}}, false},
		{"valid two tiers", []priceTier{{FromValue: &zero, ToValue: &hundred}, {FromValue: &hundred, ToValue: nil}}, false},
		{"not starting at 0", []priceTier{{FromValue: &hundred, ToValue: nil}}, true},
		{"non-contiguous", []priceTier{{FromValue: &zero, ToValue: &hundred}, {FromValue: ptrInt64(200), ToValue: nil}}, true},
		{"to <= from", []priceTier{{FromValue: &zero, ToValue: &zero}}, true},
		{"last tier not open-ended", []priceTier{{FromValue: &zero, ToValue: &hundred}}, true},
		{"missing from_value", []priceTier{{FromValue: nil, ToValue: nil}}, true},
		{"mid tier open-ended", []priceTier{{FromValue: &zero, ToValue: nil}, {FromValue: &hundred, ToValue: nil}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTiers("plan1", tt.tiers)
			if tt.hasErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.hasErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidatePriceInvalidJSON(t *testing.T) {
	plan := PlanInput{Code: "p1", Currency: "USD"}
	metrics := map[string]bool{"m1": true}

	// Invalid JSON for fixed price
	err := validatePrice(plan, PriceInput{ChargeModel: ChargeModelFixed, Properties: json.RawMessage(`invalid`)}, metrics)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}

	// Invalid JSON for per_unit price
	err = validatePrice(plan, PriceInput{ChargeModel: ChargeModelPerUnit, MetricCode: "m1", Properties: json.RawMessage(`invalid`)}, metrics)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}

	// Invalid JSON for tiered price
	err = validatePrice(plan, PriceInput{ChargeModel: ChargeModelTiered, MetricCode: "m1", Properties: json.RawMessage(`invalid`)}, metrics)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestValidateCatalogEntitlementErrors(t *testing.T) {
	tests := []struct {
		name string
		ent  EntitlementInput
	}{
		{"empty key", EntitlementInput{Key: "", ValueType: ValueTypeBoolean, Value: json.RawMessage(`true`)}},
		{"invalid value type", EntitlementInput{Key: "k1", ValueType: "invalid", Value: json.RawMessage(`true`)}},
		{"null value", EntitlementInput{Key: "k1", ValueType: ValueTypeBoolean, Value: json.RawMessage(`null`)}},
		{"empty value", EntitlementInput{Key: "k1", ValueType: ValueTypeBoolean, Value: json.RawMessage(``)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := CatalogContent{
				Metrics: []MetricInput{{Code: "m1", AggregationType: AggregationCount}},
				Plans: []PlanInput{{
					Code: "p1", Interval: IntervalMonthly, Currency: "USD",
					Prices: []PriceInput{{ChargeModel: ChargeModelFixed, Properties: mustJSON(t, fixedPriceProperties{AmountCents: ptrInt64(100), Currency: "USD"})}},
					Entitlements: []EntitlementInput{tt.ent, tt.ent}, // duplicate to trigger duplicate key check too
				}},
			}
			err := ValidateCatalog(c)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// ---- Entitlement evaluation ----

func TestEvaluateEntitlements(t *testing.T) {
	grants := []GrantKV{
		{Key: "seats", ValueType: ValueTypeNumeric, Value: json.RawMessage(`10`)},
		{Key: "feature_x", ValueType: ValueTypeBoolean, Value: json.RawMessage(`true`)},
	}

	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)

	overrides := []OverrideKV{
		{Key: "seats", ValueType: ValueTypeNumeric, Value: json.RawMessage(`20`), ExpiresAt: &future},  // active override
		{Key: "feature_y", ValueType: ValueTypeBoolean, Value: json.RawMessage(`true`), ExpiresAt: &future}, // new key
		{Key: "feature_x", ValueType: ValueTypeBoolean, Value: json.RawMessage(`false`), ExpiresAt: &past},  // expired override
	}

	result := EvaluateEntitlements(grants, overrides, time.Now())

	// seats: override wins (20)
	if string(result["seats"].Value) != "20" {
		t.Fatalf("seats = %s, want 20 (override)", result["seats"].Value)
	}
	if result["seats"].Source != EntitlementSourceOverride {
		t.Fatalf("seats source = %s, want override", result["seats"].Source)
	}

	// feature_x: expired override → plan grant (true)
	if string(result["feature_x"].Value) != "true" {
		t.Fatalf("feature_x = %s, want true (plan grant, override expired)", result["feature_x"].Value)
	}
	if result["feature_x"].Source != EntitlementSourcePlan {
		t.Fatalf("feature_x source = %s, want plan", result["feature_x"].Source)
	}

	// feature_y: only in override
	if string(result["feature_y"].Value) != "true" {
		t.Fatalf("feature_y = %s, want true", result["feature_y"].Value)
	}
	if result["feature_y"].Source != EntitlementSourceOverride {
		t.Fatalf("feature_y source = %s, want override", result["feature_y"].Source)
	}
}

func TestEvaluateEntitlementsEmpty(t *testing.T) {
	result := EvaluateEntitlements(nil, nil, time.Now())
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(result))
	}
}

func TestEvaluateEntitlementsOverrideNoExpiry(t *testing.T) {
	grants := []GrantKV{
		{Key: "k1", ValueType: ValueTypeBoolean, Value: json.RawMessage(`false`)},
	}
	overrides := []OverrideKV{
		{Key: "k1", ValueType: ValueTypeBoolean, Value: json.RawMessage(`true`), ExpiresAt: nil},
	}
	result := EvaluateEntitlements(grants, overrides, time.Now())
	if string(result["k1"].Value) != "true" {
		t.Fatalf("k1 = %s, want true (override with no expiry)", result["k1"].Value)
	}
}

// ---- Simple validation functions ----

func TestValidValueType(t *testing.T) {
	for _, v := range []string{ValueTypeBoolean, ValueTypeNumeric, ValueTypePeriod} {
		if !ValidValueType(v) {
			t.Errorf("ValidValueType(%q) = false", v)
		}
	}
	if ValidValueType("string") {
		t.Error("string should be invalid")
	}
}

func TestValidAccountType(t *testing.T) {
	if !ValidAccountType(AccountTypeIndividual) {
		t.Error("individual should be valid")
	}
	if !ValidAccountType(AccountTypeBusiness) {
		t.Error("business should be valid")
	}
	if ValidAccountType("government") {
		t.Error("government should be invalid")
	}
}

// ---- Helpers ----

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func ptrInt64(v int64) *int64 {
	return &v
}
