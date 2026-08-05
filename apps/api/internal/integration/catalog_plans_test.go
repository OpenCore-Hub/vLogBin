package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func planBody(code string) map[string]any {
	return map[string]any{
		"code":     code,
		"name":     "Plan " + code,
		"interval": "monthly",
		"currency": "USD",
		"prices": []map[string]any{
			{"charge_model": "fixed", "properties": map[string]any{"amount_cents": 2000, "currency": "USD"}},
		},
		"entitlements": []map[string]any{
			{"key": "max_users", "value_type": "numeric", "value": 5},
		},
	}
}

// TestCatalogPlansCRUD exercises the plan-level endpoints: create (with
// auto-created draft), list, duplicate conflict, update (ID preserved),
// immutable code, 404s, and delete.
func TestCatalogPlansCRUD(t *testing.T) {
	_, apiKey := createProviderAPI(t, "plans-crud-"+uuid.NewString()[:6])

	// Create — no draft exists yet, one is auto-created.
	status, body := apiReq(t, "POST", "/v1/catalog/plans", apiKey, planBody("pro"))
	if status != http.StatusCreated {
		t.Fatalf("create plan: status %d, body %v", status, body)
	}
	plan := body["plan"].(map[string]any)
	planID := plan["id"].(string)
	if plan["code"] != "pro" {
		t.Fatalf("code = %v, want pro", plan["code"])
	}
	if body["prices"] == nil || len(body["prices"].([]any)) != 1 {
		t.Fatalf("detail prices = %v, want 1 price", body["prices"])
	}
	if body["entitlement_grants"] == nil || len(body["entitlement_grants"].([]any)) != 1 {
		t.Fatalf("detail grants = %v, want 1 grant", body["entitlement_grants"])
	}

	// List.
	status, body = apiReq(t, "GET", "/v1/catalog/plans", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list plans: status %d, body %v", status, body)
	}
	if plans := body["plans"].([]any); len(plans) != 1 {
		t.Fatalf("len(plans) = %d, want 1", len(plans))
	}

	// Duplicate code → 409.
	status, body = apiReq(t, "POST", "/v1/catalog/plans", apiKey, planBody("pro"))
	if status != http.StatusConflict {
		t.Fatalf("duplicate plan: status %d, want 409, body %v", status, body)
	}

	// Update — ID preserved, children rebuilt.
	updated := planBody("pro")
	updated["name"] = "Pro Plus"
	updated["prices"] = []map[string]any{
		{"charge_model": "fixed", "properties": map[string]any{"amount_cents": 3000, "currency": "USD"}},
	}
	status, body = apiReq(t, "PUT", "/v1/catalog/plans/pro", apiKey, updated)
	if status != http.StatusOK {
		t.Fatalf("update plan: status %d, body %v", status, body)
	}
	plan = body["plan"].(map[string]any)
	if plan["id"] != planID {
		t.Fatalf("plan id changed: %v != %v", plan["id"], planID)
	}
	if plan["name"] != "Pro Plus" {
		t.Fatalf("name = %v, want Pro Plus", plan["name"])
	}

	// Code is immutable — body code must match the path code.
	status, body = apiReq(t, "PUT", "/v1/catalog/plans/pro", apiKey, planBody("renamed"))
	if status != http.StatusBadRequest {
		t.Fatalf("rename plan: status %d, want 400, body %v", status, body)
	}

	// Update missing plan → 404.
	status, body = apiReq(t, "PUT", "/v1/catalog/plans/nope", apiKey, planBody("nope"))
	if status != http.StatusNotFound {
		t.Fatalf("update missing plan: status %d, want 404, body %v", status, body)
	}

	// Delete → 204, then list is empty; delete again → 404.
	status, _ = apiReq(t, "DELETE", "/v1/catalog/plans/pro", apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete plan: status %d, want 204", status)
	}
	status, body = apiReq(t, "GET", "/v1/catalog/plans", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list after delete: status %d, body %v", status, body)
	}
	if plans := body["plans"].([]any); len(plans) != 0 {
		t.Fatalf("plans after delete = %v, want empty", body["plans"])
	}
	status, _ = apiReq(t, "DELETE", "/v1/catalog/plans/pro", apiKey, nil)
	if status != http.StatusNotFound {
		t.Fatalf("delete missing plan: status %d, want 404", status)
	}
}

// TestCatalogPlansPublishedImmutable verifies that mutating a plan after
// publish stages a new draft and never mutates the immutable published
// version.
func TestCatalogPlansPublishedImmutable(t *testing.T) {
	_, apiKey := createProviderAPI(t, "plans-immut-"+uuid.NewString()[:6])
	publishedID := createPublishedCatalog(t, apiKey)

	// Update the published plan's name — must stage a new draft.
	status, body := apiReq(t, "PUT", "/v1/catalog/plans/starter", apiKey, map[string]any{
		"code": "starter", "name": "Starter Renamed", "interval": "monthly", "currency": "USD",
		"prices": []map[string]any{
			{"charge_model": "fixed", "properties": map[string]any{"amount_cents": 1500, "currency": "USD"}},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("update published plan: status %d, body %v", status, body)
	}

	// The published version content must be unchanged.
	status, body = apiReq(t, "GET", "/v1/catalog/versions/"+publishedID, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get published version: status %d, body %v", status, body)
	}
	plans := body["plans"].([]any)
	if len(plans) != 1 {
		t.Fatalf("published plans = %d, want 1", len(plans))
	}
	if name := plans[0].(map[string]any)["name"]; name != "Starter" {
		t.Fatalf("published plan name = %v, want Starter (immutable)", name)
	}

	// List plans now reflects the draft (renamed).
	status, body = apiReq(t, "GET", "/v1/catalog/plans", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list plans: status %d, body %v", status, body)
	}
	plansList := body["plans"].([]any)
	if len(plansList) != 1 || plansList[0].(map[string]any)["name"] != "Starter Renamed" {
		t.Fatalf("draft plans = %v, want renamed draft", body["plans"])
	}
}

// TestCatalogPlansClonePublishedContent verifies that creating a plan without
// a draft clones the latest published content (metrics + plans), so prices
// can reference the published metric, and unknown metric references are
// rejected.
func TestCatalogPlansClonePublishedContent(t *testing.T) {
	_, apiKey := createProviderAPI(t, "plans-clone-"+uuid.NewString()[:6])
	createPublishedCatalog(t, apiKey)

	// Create a second plan referencing the cloned metric.
	status, body := apiReq(t, "POST", "/v1/catalog/plans", apiKey, map[string]any{
		"code": "pro", "name": "Pro", "interval": "monthly", "currency": "USD",
		"prices": []map[string]any{
			{"charge_model": "per_unit", "metric_code": "api_calls",
				"properties": map[string]any{"unit_amount_cents": 5, "currency": "USD"}},
		},
	})
	if status != http.StatusCreated {
		t.Fatalf("create plan with metric: status %d, body %v", status, body)
	}

	// Draft contains the cloned starter plan + the new pro plan.
	status, body = apiReq(t, "GET", "/v1/catalog/plans", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list plans: status %d, body %v", status, body)
	}
	if plans := body["plans"].([]any); len(plans) != 2 {
		t.Fatalf("plans = %d, want 2 (cloned starter + new pro)", len(plans))
	}

	// Referencing an unknown metric is rejected.
	status, body = apiReq(t, "POST", "/v1/catalog/plans", apiKey, map[string]any{
		"code": "bad", "name": "Bad", "interval": "monthly", "currency": "USD",
		"prices": []map[string]any{
			{"charge_model": "per_unit", "metric_code": "does_not_exist",
				"properties": map[string]any{"unit_amount_cents": 5, "currency": "USD"}},
		},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("unknown metric: status %d, want 400, body %v", status, body)
	}
}
