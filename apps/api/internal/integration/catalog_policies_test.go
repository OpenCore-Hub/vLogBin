package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestCatalogPoliciesCRUD exercises the plan-level entitlement (policy)
// endpoints: set (upsert), list, key immutability, validation, and delete
// with proper 404 semantics.
func TestCatalogPoliciesCRUD(t *testing.T) {
	_, apiKey := createProviderAPI(t, "policies-crud-"+uuid.NewString()[:6])
	createPublishedCatalog(t, apiKey)

	// Set a boolean policy on the published starter plan (stages a draft that
	// clones the published content, including the max_users grant).
	status, body := apiReq(t, "PUT", "/v1/catalog/plans/starter/entitlements/feature_export", apiKey, map[string]any{
		"value_type": "boolean", "value": true,
	})
	if status != http.StatusOK {
		t.Fatalf("set policy: status %d, body %v", status, body)
	}
	grant := body["entitlement"].(map[string]any)
	if grant["key"] != "feature_export" || grant["value_type"] != "boolean" {
		t.Fatalf("set policy grant = %v, want key/value_type", grant)
	}
	if grant["value"] != true {
		t.Fatalf("set policy value = %v, want true", grant["value"])
	}

	// Upsert the same key with a numeric value.
	status, body = apiReq(t, "PUT", "/v1/catalog/plans/starter/entitlements/feature_export", apiKey, map[string]any{
		"value_type": "numeric", "value": 5,
	})
	if status != http.StatusOK {
		t.Fatalf("upsert policy: status %d, body %v", status, body)
	}
	if v := body["entitlement"].(map[string]any)["value"]; v != float64(5) {
		t.Fatalf("upsert value = %v, want 5", v)
	}

	// List reflects the upserted grant next to the cloned max_users grant.
	status, body = apiReq(t, "GET", "/v1/catalog/plans/starter/entitlements", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list policies: status %d, body %v", status, body)
	}
	grants := body["entitlements"].([]any)
	if len(grants) != 2 {
		t.Fatalf("list policies = %d, want 2 (cloned max_users + feature_export)", len(grants))
	}
	seen := map[string]map[string]any{}
	for _, g := range grants {
		gm := g.(map[string]any)
		seen[gm["key"].(string)] = gm
	}
	if g := seen["feature_export"]; g == nil || g["value_type"] != "numeric" {
		t.Fatalf("listed feature_export = %v, want upserted numeric", seen["feature_export"])
	}
	if seen["max_users"] == nil {
		t.Fatalf("cloned max_users grant missing from list: %v", body["entitlements"])
	}

	// The entitlement key is immutable: a mismatching body key is rejected.
	status, _ = apiReq(t, "PUT", "/v1/catalog/plans/starter/entitlements/feature_export", apiKey, map[string]any{
		"key": "other", "value_type": "boolean", "value": true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("mismatched key: status %d, want 400", status)
	}

	// Validation: unknown value_type is rejected.
	status, _ = apiReq(t, "PUT", "/v1/catalog/plans/starter/entitlements/bad", apiKey, map[string]any{
		"value_type": "weird", "value": true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("unknown value_type: status %d, want 400", status)
	}

	// Unknown plan is 404.
	status, _ = apiReq(t, "PUT", "/v1/catalog/plans/nope/entitlements/x", apiKey, map[string]any{
		"value_type": "boolean", "value": true,
	})
	if status != http.StatusNotFound {
		t.Fatalf("set on missing plan: status %d, want 404", status)
	}

	// Delete: missing key and missing plan are 404, then success is 204.
	status, _ = apiReq(t, "DELETE", "/v1/catalog/plans/starter/entitlements/nope", apiKey, nil)
	if status != http.StatusNotFound {
		t.Fatalf("delete missing key: status %d, want 404", status)
	}
	status, _ = apiReq(t, "DELETE", "/v1/catalog/plans/nope/entitlements/x", apiKey, nil)
	if status != http.StatusNotFound {
		t.Fatalf("delete missing plan: status %d, want 404", status)
	}
	status, _ = apiReq(t, "DELETE", "/v1/catalog/plans/starter/entitlements/feature_export", apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete policy: status %d, want 204", status)
	}
	status, _ = apiReq(t, "DELETE", "/v1/catalog/plans/starter/entitlements/feature_export", apiKey, nil)
	if status != http.StatusNotFound {
		t.Fatalf("delete deleted policy: status %d, want 404", status)
	}

	// List has only the cloned grant after deletion.
	status, body = apiReq(t, "GET", "/v1/catalog/plans/starter/entitlements", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list after delete: status %d, body %v", status, body)
	}
	if grants := body["entitlements"].([]any); len(grants) != 1 {
		t.Fatalf("grants after delete = %d, want 1 (cloned max_users)", len(grants))
	}
}

// TestCatalogPoliciesPublishedImmutable verifies that setting a policy after
// publish stages a new draft and never mutates the immutable published
// version.
func TestCatalogPoliciesPublishedImmutable(t *testing.T) {
	_, apiKey := createProviderAPI(t, "policies-immut-"+uuid.NewString()[:6])
	publishedID := createPublishedCatalog(t, apiKey)

	// Snapshot the published grants before mutating.
	status, before := apiReq(t, "GET", "/v1/catalog/versions/"+publishedID, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get published version: status %d, body %v", status, before)
	}
	publishedCount := len(before["entitlement_grants"].([]any))

	// Set a policy — stages a new draft, leaves published untouched.
	status, body := apiReq(t, "PUT", "/v1/catalog/plans/starter/entitlements/gpt_boost", apiKey, map[string]any{
		"value_type": "boolean", "value": true,
	})
	if status != http.StatusOK {
		t.Fatalf("set policy: status %d, body %v", status, body)
	}

	// Published content is unchanged and does not contain the new key.
	status, after := apiReq(t, "GET", "/v1/catalog/versions/"+publishedID, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get published version after: status %d, body %v", status, after)
	}
	afterGrants := after["entitlement_grants"].([]any)
	if len(afterGrants) != publishedCount {
		t.Fatalf("published grants = %d, want %d (immutable)", len(afterGrants), publishedCount)
	}
	for _, g := range afterGrants {
		if gm := g.(map[string]any); gm["key"] == "gpt_boost" {
			t.Fatalf("published version contains staged policy %v", gm)
		}
	}

	// The draft exposes the staged policy next to the cloned max_users.
	status, body = apiReq(t, "GET", "/v1/catalog/plans/starter/entitlements", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list policies: status %d, body %v", status, body)
	}
	grants := body["entitlements"].([]any)
	keys := map[string]bool{}
	for _, g := range grants {
		keys[g.(map[string]any)["key"].(string)] = true
	}
	if !keys["gpt_boost"] || !keys["max_users"] {
		t.Fatalf("draft policies = %v, want gpt_boost + cloned max_users", body["entitlements"])
	}
}
