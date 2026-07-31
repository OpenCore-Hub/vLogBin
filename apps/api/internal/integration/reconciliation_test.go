package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestReconciliationDetectsDrift(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "recon-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)
	_ = custExt

	// Run reconciliation. The subscription we just created has no
	// entitlement snapshot, so the subscription_snapshot_freshness check
	// should detect drift.
	results, err := svc.RunReconciliation(testCtx)
	if err != nil {
		t.Fatalf("RunReconciliation: %v", err)
	}
	if len(results) < 5 {
		t.Fatalf("expected >= 5 checks, got %d", len(results))
	}

	// Find the subscription_snapshot_freshness check — it must report drift.
	found := false
	for _, r := range results {
		if r.Name == "subscription_snapshot_freshness" {
			found = true
			if r.Status != "drift" {
				t.Fatalf("snapshot_freshness status = %s, want drift (active sub without snapshot)", r.Status)
			}
			if r.DriftCount < 1 {
				t.Fatalf("snapshot_freshness drift_count = %d, want >= 1", r.DriftCount)
			}
			break
		}
	}
	if !found {
		t.Fatal("subscription_snapshot_freshness check not found in results")
	}

	// Other checks should be ok (no stuck outbox, no orphaned usage, etc.).
	for _, r := range results {
		if r.Name == "usage_outbox_stuck" && r.Status != "ok" {
			t.Fatalf("usage_outbox_stuck status = %s, want ok (no stuck events)", r.Status)
		}
		if r.Name == "outbox_dead_letter" && r.Status != "ok" {
			t.Fatalf("outbox_dead_letter status = %s, want ok (no dead-lettered events)", r.Status)
		}
	}

	// Verify results are accessible via the operator API.
	status, body := apiReq(t, "GET", "/v1/operator/reconciliation-results", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list reconciliation results: status %d, body %v", status, body)
	}
	resultsArr, ok := body["reconciliation_results"].([]any)
	if !ok || len(resultsArr) < 5 {
		t.Fatalf("expected >= 5 reconciliation results via API, got %v", body["reconciliation_results"])
	}

	// Provider API key cannot access reconciliation results (operator-only).
	status, _ = apiReq(t, "GET", "/v1/operator/reconciliation-results", apiKey, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("provider access to reconciliation: status %d, want 401", status)
	}

	_ = providerID
}
