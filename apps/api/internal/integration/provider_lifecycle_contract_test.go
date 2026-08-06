package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestProviderOffboardingEndToEnd covers the commercial offboarding loop:
// full export, deletion proof, LIVE_ACTIVE promotion, OFFBOARDING transition,
// then write blocking while reads remain available for audit/forensics.
func TestProviderOffboardingEndToEnd(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "offboard-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	// Final billing/export snapshot with checksum.
	status, body := apiReq(t, "POST", "/v1/data-exports", apiKey, map[string]any{
		"export_type": "full",
	})
	if status != http.StatusCreated || body["data_hash"] == nil {
		t.Fatalf("full export: status %d, body %v", status, body)
	}

	// Deletion proof before the provider is removed from writable operation.
	status, body = apiReq(t, "POST", "/v1/data-deletion", apiKey, map[string]any{
		"reason": "provider offboarding",
	})
	if status != http.StatusOK || body["proof_signature"] == nil {
		t.Fatalf("deletion proof: status %d, body %v", status, body)
	}
	proofID := body["id"].(string)

	// Promote to live and then offboard.
	transitionTo(t, providerID, "LIVE_REVIEW")
	submitApprovedRiskReview(t, providerID)
	transitionTo(t, providerID, "LIVE_ACTIVE")
	transitionTo(t, providerID, "OFFBOARDING")

	// Writes are blocked in OFFBOARDING; reads remain available.
	status, body = apiReq(t, "POST", "/v1/subscriptions", apiKey, map[string]any{
		"external_id":          "offboard-blocked",
		"customer_external_id": "offboard-blocked",
		"catalog_version_id":   versionID,
		"plan_code":            "starter",
	})
	if status != http.StatusConflict {
		t.Fatalf("write after offboarding: status %d, want 409", status)
	}
	if code := errorCode(body); code != "provider_not_writable" {
		t.Fatalf("write after offboarding code = %q, want provider_not_writable", code)
	}

	status, body = apiReq(t, "GET", "/v1/deletion-proofs/"+proofID, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("deletion proof after offboarding: status %d, want 200", status)
	}
}
