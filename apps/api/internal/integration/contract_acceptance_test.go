package integration

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func publishCatalogContent(t *testing.T, apiKey string, content map[string]any) string {
	t.Helper()
	status, body := apiReq(t, "POST", "/v1/catalog/versions", apiKey, map[string]any{})
	if status != http.StatusCreated {
		t.Fatalf("create catalog version: status %d, body %v", status, body)
	}
	versionID := body["version"].(map[string]any)["id"].(string)
	status, body = apiReq(t, "PUT", "/v1/catalog/versions/"+versionID+"/content", apiKey, content)
	if status != http.StatusOK {
		t.Fatalf("replace content: status %d, body %v", status, body)
	}
	status, body = apiReq(t, "POST", "/v1/catalog/versions/"+versionID+"/validate", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("validate: status %d, body %v", status, body)
	}
	status, body = apiReq(t, "POST", "/v1/catalog/versions/"+versionID+"/publish", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("publish: status %d, body %v", status, body)
	}
	return versionID
}

// TestCommercialContractAcceptance is the black-box public-contract smoke
// harness for the commercial launch gate. It drives only provider-facing
// public API routes and asserts the highest-risk contract seams.
func TestCommercialContractAcceptance(t *testing.T) {
	_, keyA := createProviderAPI(t, "cta-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "cta-b-"+uuid.NewString()[:8])

	versionA := createPublishedCatalog(t, keyA)
	versionB := createPublishedCatalog(t, keyB)
	custA, subA := createCustomerAndSubscription(t, keyA, versionA)
	_, subB := createCustomerAndSubscription(t, keyB, versionB)

	// SPEC #1: Provider A can never read Provider B data.
	status, body := apiReq(t, "GET", "/v1/customers", keyA, nil)
	if status != http.StatusOK || len(body["customers"].([]any)) != 1 {
		t.Fatalf("provider A customers: status %d, body %v", status, body)
	}
	status, body = apiReq(t, "GET", "/v1/customers", keyB, nil)
	if status != http.StatusOK || len(body["customers"].([]any)) != 1 {
		t.Fatalf("provider B customers: status %d, body %v", status, body)
	}

	status, body = apiReq(t, "GET", "/v1/subscriptions", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("provider B subscriptions: status %d, body %v", status, body)
	}
	for _, item := range body["subscriptions"].([]any) {
		if item.(map[string]any)["id"] == subA {
			t.Fatal("provider B can see provider A subscription")
		}
	}
	if len(body["subscriptions"].([]any)) != 1 {
		t.Fatalf("provider B subscriptions = %d, want 1", len(body["subscriptions"].([]any)))
	}

	// SPEC #3: request headers cannot override credential environment.
	status, _ = apiReqEnv(t, "GET", "/v1/whoami", keyA, "live")
	if status != http.StatusBadRequest {
		t.Fatalf("X-Environment mismatch: status %d, want 400", status)
	}

	// SPEC #4: identical usage retries create one billable effect.
	txID := "cta-tx-" + uuid.NewString()[:8]
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	status, body = ingestUsage(t, keyA, txID, custA, "api_calls", ts, map[string]any{"count": 1})
	if status != http.StatusCreated {
		t.Fatalf("first ingest: status %d, body %v", status, body)
	}
	status, body = ingestUsage(t, keyA, txID, custA, "api_calls", ts, map[string]any{"count": 1})
	if status != http.StatusOK || body["status"] != "duplicate" {
		t.Fatalf("duplicate ingest: status %d, body %v", status, body)
	}
	status, body = apiReq(t, "GET", "/v1/usage/events", keyA, nil)
	if status != http.StatusOK || len(body["events"].([]any)) != 1 {
		t.Fatalf("provider A usage events: status %d, body %v", status, body)
	}

	// SPEC #7: published catalog versions cannot be mutated.
	status, _ = apiReq(t, "PUT", "/v1/catalog/versions/"+versionA+"/content", keyA, catalogContent())
	if status != http.StatusConflict {
		t.Fatalf("mutate published catalog: status %d, want 409", status)
	}

	// SPEC #8: existing subscriptions remain pinned when a newer version publishes.
	proContent := catalogContent()
	proPlans := proContent["plans"].([]map[string]any)
	proPlans[0]["code"] = "pro"
	publishCatalogContent(t, keyA, proContent)
	status, body = apiReq(t, "GET", "/v1/subscriptions", keyA, nil)
	if status != http.StatusOK {
		t.Fatalf("provider A subscriptions after publish: status %d, body %v", status, body)
	}
	found := false
	for _, item := range body["subscriptions"].([]any) {
		sub := item.(map[string]any)
		if sub["id"] == subA {
			found = true
			if sub["catalog_version_id"] != versionA {
				t.Fatalf("pinned subscription catalog_version_id = %v, want %s", sub["catalog_version_id"], versionA)
			}
		}
		if sub["id"] == subB {
			t.Fatal("provider A can see provider B subscription after publish")
		}
	}
	if !found {
		t.Fatalf("provider A subscription %s missing after publish: %v", subA, body)
	}
}
