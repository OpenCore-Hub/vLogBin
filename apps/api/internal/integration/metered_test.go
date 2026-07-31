package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestMeteredPricingRuleCRUD(t *testing.T) {
	_, apiKey := createProviderAPI(t, "mb-crud-"+uuid.NewString()[:8])

	// Set per_unit pricing rule.
	status, body := apiReq(t, "PUT", "/v1/metered-pricing-rules", apiKey, map[string]any{
		"metric_code":         "api_calls",
		"pricing_model":       "per_unit",
		"base_price_cents":    1,
		"tier_config":         []map[string]any{},
		"minimum_spend_cents": 0,
		"enabled":             true,
	})
	if status != http.StatusOK {
		t.Fatalf("set rule: status %d, body %v", status, body)
	}
	if body["pricing_model"] != "per_unit" {
		t.Fatalf("pricing_model = %v", body["pricing_model"])
	}

	// Get by metric code.
	status, body = apiReq(t, "GET", "/v1/metered-pricing-rules/api_calls", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get rule: status %d", status)
	}

	// List rules.
	status, body = apiReq(t, "GET", "/v1/metered-pricing-rules", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list rules: status %d", status)
	}
	rules := body["metered_pricing_rules"].([]any)
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	// Update to tiered.
	status, body = apiReq(t, "PUT", "/v1/metered-pricing-rules", apiKey, map[string]any{
		"metric_code":      "api_calls",
		"pricing_model":    "tiered",
		"base_price_cents": 2,
		"tier_config": []map[string]any{
			{"up_to": 1000, "price_cents": 2},
			{"up_to": 10000, "price_cents": 1},
		},
		"minimum_spend_cents": 100,
		"enabled":             true,
	})
	if status != http.StatusOK {
		t.Fatalf("update to tiered: status %d", status)
	}
	if body["pricing_model"] != "tiered" {
		t.Fatalf("pricing_model = %v", body["pricing_model"])
	}

	// Delete.
	status, _ = apiReq(t, "DELETE", "/v1/metered-pricing-rules/api_calls", apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete: status %d, want 204", status)
	}

	// Get after delete — 404.
	status, _ = apiReq(t, "GET", "/v1/metered-pricing-rules/api_calls", apiKey, nil)
	if status != http.StatusNotFound {
		t.Fatalf("get deleted: status %d, want 404", status)
	}
}

func TestMeteredPricingRuleValidation(t *testing.T) {
	_, apiKey := createProviderAPI(t, "mb-val-"+uuid.NewString()[:8])

	// Invalid pricing_model.
	status, _ := apiReq(t, "PUT", "/v1/metered-pricing-rules", apiKey, map[string]any{
		"metric_code":      "bad",
		"pricing_model":    "invalid",
		"base_price_cents": 1,
		"enabled":          true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid model: status %d, want 400", status)
	}

	// Missing metric_code.
	status, _ = apiReq(t, "PUT", "/v1/metered-pricing-rules", apiKey, map[string]any{
		"pricing_model":    "per_unit",
		"base_price_cents": 1,
		"enabled":          true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("missing metric_code: status %d, want 400", status)
	}

	// Negative base_price_cents.
	status, _ = apiReq(t, "PUT", "/v1/metered-pricing-rules", apiKey, map[string]any{
		"metric_code":      "neg",
		"pricing_model":    "per_unit",
		"base_price_cents": -1,
		"enabled":          true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("negative price: status %d, want 400", status)
	}
}

func TestBudgetAlertCRUD(t *testing.T) {
	_, apiKey := createProviderAPI(t, "ba-crud-"+uuid.NewString()[:8])

	// Create budget alert.
	status, body := apiReq(t, "POST", "/v1/budget-alerts", apiKey, map[string]any{
		"metric_code":   "api_calls",
		"budget_cents":  10000,
		"threshold_pct": 80.0,
	})
	if status != http.StatusCreated {
		t.Fatalf("create alert: status %d, body %v", status, body)
	}
	alertID := body["id"].(string)
	if body["alert_status"] != "ok" {
		t.Fatalf("alert_status = %v, want ok", body["alert_status"])
	}

	// Get by ID.
	status, body = apiReq(t, "GET", "/v1/budget-alerts/"+alertID, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get alert: status %d", status)
	}

	// List alerts.
	status, body = apiReq(t, "GET", "/v1/budget-alerts", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list alerts: status %d", status)
	}
	alerts := body["budget_alerts"].([]any)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	// Delete.
	status, _ = apiReq(t, "DELETE", "/v1/budget-alerts/"+alertID, apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete: status %d, want 204", status)
	}
}

func TestBudgetAlertValidation(t *testing.T) {
	_, apiKey := createProviderAPI(t, "ba-val-"+uuid.NewString()[:8])

	// Zero budget.
	status, _ := apiReq(t, "POST", "/v1/budget-alerts", apiKey, map[string]any{
		"budget_cents":  0,
		"threshold_pct": 80.0,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("zero budget: status %d, want 400", status)
	}

	// Invalid threshold.
	status, _ = apiReq(t, "POST", "/v1/budget-alerts", apiKey, map[string]any{
		"budget_cents":  1000,
		"threshold_pct": 150.0,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid threshold: status %d, want 400", status)
	}
}

func TestMeteredPricingCrossTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "mb-iso-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "mb-iso-b-"+uuid.NewString()[:8])

	// Provider A sets a rule.
	apiReq(t, "PUT", "/v1/metered-pricing-rules", keyA, map[string]any{
		"metric_code":      "api_calls",
		"pricing_model":    "per_unit",
		"base_price_cents": 1,
		"enabled":          true,
	})

	// Provider B cannot see A's rules.
	status, body := apiReq(t, "GET", "/v1/metered-pricing-rules", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("B list: status %d", status)
	}
	rules := body["metered_pricing_rules"].([]any)
	if len(rules) != 0 {
		t.Fatalf("B: expected 0 rules, got %d (RLS leak)", len(rules))
	}
}
