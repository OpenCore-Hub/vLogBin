package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func operatorPlanBase(t *testing.T, providerID string) string {
	t.Helper()
	return "/v1/operator/providers/" + providerID + "/catalog/plans"
}

// TestOperatorCatalogPlansLifecycle verifies the Console-facing Plans
// control plane (M2 Plans page): empty state, create → list (with resolved
// metric codes after a published catalog seeds metrics) → get → immutable
// code → update → delete.
func TestOperatorCatalogPlansLifecycle(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "op-plans-"+uuid.NewString()[:8])
	base := operatorPlanBase(t, providerID)

	// A provider with no catalog version yields an empty collection, not an
	// error, so the Plans page can render its empty state.
	status, body := apiReq(t, http.MethodGet, base+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list empty: status %d, body %v", status, body)
	}
	if plans, _ := body["plans"].([]any); len(plans) != 0 {
		t.Fatalf("plans = %v, want empty", body["plans"])
	}
	if metrics, _ := body["metrics"].([]any); len(metrics) != 0 {
		t.Fatalf("metrics = %v, want empty", body["metrics"])
	}

	// Seed a published catalog with metrics + per_unit pricing via the
	// provider API.
	createPublishedCatalog(t, apiKey)

	// Create a plan through the operator control plane. With no draft, the
	// published content is cloned, so the new plan sits next to starter and
	// the cloned metric is available for pricing references.
	status, body = apiReq(t, http.MethodPost, base+"?env=test", operatorToken, map[string]any{
		"code": "pro", "name": "Pro", "interval": "monthly", "currency": "USD",
		"prices": []map[string]any{
			{"charge_model": "fixed", "properties": map[string]any{"amount_cents": 2900, "currency": "USD"}},
			{"charge_model": "per_unit", "metric_code": "api_calls",
				"properties": map[string]any{"unit_amount_cents": 5, "currency": "USD"}},
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("create plan: status %d, body %v", status, body)
	}
	detail := body["plan"].(map[string]any)
	plan := detail["plan"].(map[string]any)
	if plan["code"] != "pro" || plan["name"] != "Pro" {
		t.Fatalf("created plan = %v", plan)
	}
	prices := detail["prices"].([]any)
	createdPerUnit := findPriceByModel(t, prices, "per_unit")
	if len(prices) != 2 || createdPerUnit == nil || createdPerUnit["metric_code"] != "api_calls" {
		t.Fatalf("created plan prices = %v, want resolved metric_code", detail["prices"])
	}

	// The operator list now shows the cloned starter + the new pro plan and
	// resolves the per_unit metric code.
	status, body = apiReq(t, http.MethodGet, base+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d, body %v", status, body)
	}
	metrics, _ := body["metrics"].([]any)
	if len(metrics) != 1 {
		t.Fatalf("metrics = %v, want 1 (api_calls)", body["metrics"])
	}
	if code := metrics[0].(map[string]any)["code"]; code != "api_calls" {
		t.Fatalf("metric code = %v, want api_calls", code)
	}
	plans, _ := body["plans"].([]any)
	if len(plans) != 2 {
		t.Fatalf("plans = %v, want 2 (cloned starter + pro)", body["plans"])
	}
	starter := findPlanByCode(t, plans, "starter")
	if starter == nil {
		t.Fatalf("starter missing from %v", body["plans"])
	}
	prices = starter["prices"].([]any)
	if len(prices) != 2 {
		t.Fatalf("starter prices = %v, want 2", starter["prices"])
	}
	perUnit := findPriceByModel(t, prices, "per_unit")
	if perUnit == nil {
		t.Fatalf("starter per_unit price missing from %v", starter["prices"])
	}
	if perUnit["metric_code"] != "api_calls" {
		t.Fatalf("per_unit metric_code = %v, want api_calls", perUnit["metric_code"])
	}

	// Get one plan by code.
	status, body = apiReq(t, http.MethodGet, base+"/starter?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("get plan: status %d, body %v", status, body)
	}
	gotDetail := body["plan"].(map[string]any)
	got := gotDetail["plan"].(map[string]any)
	if got["code"] != "starter" {
		t.Fatalf("get plan code = %v", got["code"])
	}

	// Update keeps the ID and stages a draft without touching published.
	updated := map[string]any{
		"code": "starter", "name": "Starter Pro", "interval": "monthly", "currency": "USD",
		"prices": []map[string]any{
			{"charge_model": "fixed", "properties": map[string]any{"amount_cents": 1500, "currency": "USD"}},
		},
	}
	status, body = apiReq(t, http.MethodPut, base+"/starter?env=test", operatorToken, updated)
	if status != http.StatusOK {
		t.Fatalf("update plan: status %d, body %v", status, body)
	}
	plan = body["plan"].(map[string]any)["plan"].(map[string]any)
	if plan["name"] != "Starter Pro" {
		t.Fatalf("updated name = %v", plan["name"])
	}

	// Plan code is immutable: body code must match the path code.
	renamed := map[string]any{
		"code": "renamed", "name": "Renamed", "interval": "monthly", "currency": "USD",
		"prices": []map[string]any{
			{"charge_model": "fixed", "properties": map[string]any{"amount_cents": 100, "currency": "USD"}},
		},
	}
	status, body = apiReq(t, http.MethodPut, base+"/starter?env=test", operatorToken, renamed)
	if status != http.StatusBadRequest {
		t.Fatalf("immutable code: status %d, want 400, body %v", status, body)
	}

	// Delete → 204; get and delete again → 404.
	status, _ = apiReq(t, http.MethodDelete, base+"/pro?env=test", operatorToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete plan: status %d, want 204", status)
	}
	status, _ = apiReq(t, http.MethodGet, base+"/pro?env=test", operatorToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("get deleted plan: status %d, want 404", status)
	}
	status, _ = apiReq(t, http.MethodDelete, base+"/pro?env=test", operatorToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("delete deleted plan: status %d, want 404", status)
	}
}

// TestOperatorCatalogPlansValidation verifies the environment/provider error
// contract shared with the OIDC Application control plane.
func TestOperatorCatalogPlansValidation(t *testing.T) {
	providerID, _ := createProviderAPI(t, "op-plansv-"+uuid.NewString()[:8])
	base := operatorPlanBase(t, providerID)

	if status, body := apiReq(t, http.MethodGet, base, operatorToken, nil); status != http.StatusBadRequest {
		t.Fatalf("missing env: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, base+"?env=staging", operatorToken, nil); status != http.StatusBadRequest {
		t.Fatalf("invalid env: status %d, body %v", status, body)
	}
	// createProviderAPI provisions only a test environment; live is absent.
	if status, body := apiReq(t, http.MethodGet, base+"?env=live", operatorToken, nil); status != http.StatusNotFound {
		t.Fatalf("unknown environment: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, "/v1/operator/providers/not-a-uuid/catalog/plans?env=test", operatorToken, nil); status != http.StatusBadRequest {
		t.Fatalf("invalid id: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, "/v1/operator/providers/"+uuid.NewString()+"/catalog/plans?env=test", operatorToken, nil); status != http.StatusNotFound {
		t.Fatalf("unknown provider: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodPost, base+"?env=test", operatorToken, map[string]any{
		"code": "bad", "name": "Bad", "interval": "quarterly", "currency": "USD",
		"prices": []map[string]any{},
	}); status != http.StatusBadRequest {
		t.Fatalf("invalid interval: status %d, body %v", status, body)
	}
}

func findPlanByCode(t *testing.T, plans []any, code string) map[string]any {
	t.Helper()
	for _, raw := range plans {
		detail, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		p, ok := detail["plan"].(map[string]any)
		if !ok {
			continue
		}
		if p["code"] == code {
			return detail
		}
	}
	return nil
}

func findPriceByModel(t *testing.T, prices []any, model string) map[string]any {
	t.Helper()
	for _, raw := range prices {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if p["charge_model"] == model {
			return p
		}
	}
	return nil
}
