package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestOperatorCatalogPoliciesLifecycle verifies the Console-facing Policies
// control plane (plan-level entitlement grants): list → set → immutable key →
// delete → 404, all through the operator-session auth context.
func TestOperatorCatalogPoliciesLifecycle(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "op-policies-"+uuid.NewString()[:8])
	base := operatorPlanBase(t, providerID)
	entitlementsPath := base + "/starter/entitlements"

	createPublishedCatalog(t, apiKey)

	// The seeded published catalog already carries a cloned max_users grant.
	status, body := apiReq(t, http.MethodGet, entitlementsPath+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list empty: status %d, body %v", status, body)
	}
	if grants, _ := body["entitlements"].([]any); len(grants) != 1 {
		t.Fatalf("entitlements = %v, want 1 seeded max_users grant", body["entitlements"])
	}

	// Upsert one grant.
	status, body = apiReq(t, http.MethodPut, entitlementsPath+"/feature_export?env=test", operatorToken, map[string]any{
		"key": "feature_export", "value_type": "boolean", "value": true,
	})
	if status != http.StatusOK {
		t.Fatalf("set grant: status %d, body %v", status, body)
	}
	grant := body["entitlement"].(map[string]any)
	if grant["key"] != "feature_export" || grant["value_type"] != "boolean" {
		t.Fatalf("grant = %v", grant)
	}

	status, body = apiReq(t, http.MethodGet, entitlementsPath+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d, body %v", status, body)
	}
	grants, _ := body["entitlements"].([]any)
	if len(grants) != 2 {
		t.Fatalf("entitlements = %v, want 2 (max_users + feature_export)", body["entitlements"])
	}

	// Grant key is immutable: body key must match the path key.
	status, body = apiReq(t, http.MethodPut, entitlementsPath+"/feature_export?env=test", operatorToken, map[string]any{
		"key": "other", "value_type": "boolean", "value": true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("immutable key: status %d, want 400, body %v", status, body)
	}

	// Delete → 204; delete again → 404.
	status, _ = apiReq(t, http.MethodDelete, entitlementsPath+"/feature_export?env=test", operatorToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete grant: status %d, want 204", status)
	}
	status, _ = apiReq(t, http.MethodDelete, entitlementsPath+"/feature_export?env=test", operatorToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("delete missing grant: status %d, want 404", status)
	}
	status, body = apiReq(t, http.MethodGet, entitlementsPath+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list after delete: status %d, body %v", status, body)
	}
	grants, _ = body["entitlements"].([]any)
	if len(grants) != 1 {
		t.Fatalf("entitlements = %v, want 1 (max_users only)", body["entitlements"])
	}

	// Unknown plan / invalid environment share the catalog validation contract.
	if status, body := apiReq(t, http.MethodGet, base+"/nope/entitlements?env=test", operatorToken, nil); status != http.StatusNotFound {
		t.Fatalf("unknown plan: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, entitlementsPath+"?env=staging", operatorToken, nil); status != http.StatusBadRequest {
		t.Fatalf("invalid env: status %d, body %v", status, body)
	}
}
