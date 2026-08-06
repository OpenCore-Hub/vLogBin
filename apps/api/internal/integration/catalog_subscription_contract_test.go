package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestEntitlementSnapshotSingleSourceOfTruth proves entitlements are
// computed from the pinned catalog version, not from the latest published
// version: publishing a new plan value must not change an existing
// subscription's snapshot.
func TestEntitlementSnapshotSingleSourceOfTruth(t *testing.T) {
	_, apiKey := createProviderAPI(t, "ent-truth-"+uuid.NewString()[:8])
	v1 := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, v1)

	status, body := apiReq(t, "GET", "/v1/entitlements/"+custExt, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("snapshot v1: status %d, body %v", status, body)
	}
	snapshot := body["snapshot"].(map[string]any)
	if snapshot["catalog_version_id"] != v1 {
		t.Fatalf("snapshot catalog_version_id = %v, want %s", snapshot["catalog_version_id"], v1)
	}
	entitlements := snapshot["entitlements"].(map[string]any)
	maxUsers := entitlements["max_users"].(map[string]any)
	if maxUsers["value"] != float64(10) {
		t.Fatalf("max_users = %v, want 10", maxUsers["value"])
	}

	// Publish v2 with a different max_users value.
	content := catalogContent()
	plans := content["plans"].([]map[string]any)
	grants := plans[0]["entitlements"].([]map[string]any)
	grants[0]["value"] = 99
	publishCatalogContent(t, apiKey, content)

	// Existing subscription snapshot must still pin v1 and the old value.
	status, body = apiReq(t, "GET", "/v1/entitlements/"+custExt, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("snapshot after publish v2: status %d, body %v", status, body)
	}
	snapshot = body["snapshot"].(map[string]any)
	if snapshot["catalog_version_id"] != v1 {
		t.Fatalf("snapshot catalog_version_id after v2 = %v, want %s", snapshot["catalog_version_id"], v1)
	}
	entitlements = snapshot["entitlements"].(map[string]any)
	maxUsers = entitlements["max_users"].(map[string]any)
	if maxUsers["value"] != float64(10) {
		t.Fatalf("max_users after v2 = %v, want 10 (pinned)", maxUsers["value"])
	}
}
