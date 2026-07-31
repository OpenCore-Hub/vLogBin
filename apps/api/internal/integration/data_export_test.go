package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestDataExportRequest(t *testing.T) {
	_, apiKey := createProviderAPI(t, "de-req-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	// Request a full export.
	status, body := apiReq(t, "POST", "/v1/data-exports", apiKey, map[string]any{
		"export_type": "full",
	})
	if status != http.StatusCreated {
		t.Fatalf("request export: status %d, body %v", status, body)
	}
	if body["status"] != "completed" {
		t.Fatalf("status = %v, want completed", body["status"])
	}
	if body["data_hash"] == nil {
		t.Fatal("data_hash must be set for completed export")
	}
	if int(body["record_count"].(float64)) == 0 {
		t.Fatal("record_count should be > 0 (provider has customers/subscriptions)")
	}
}

func TestDataExportBillingOnly(t *testing.T) {
	_, apiKey := createProviderAPI(t, "de-bil-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	status, body := apiReq(t, "POST", "/v1/data-exports", apiKey, map[string]any{
		"export_type": "billing_only",
	})
	if status != http.StatusCreated {
		t.Fatalf("billing export: status %d", status)
	}
	if body["export_type"] != "billing_only" {
		t.Fatalf("export_type = %v", body["export_type"])
	}
	if int(body["record_count"].(float64)) < 2 {
		t.Fatalf("record_count = %v, want >= 2 (customer + subscription)", body["record_count"])
	}
}

func TestDataExportListAndGet(t *testing.T) {
	_, apiKey := createProviderAPI(t, "de-lst-"+uuid.NewString()[:8])

	// Initially empty.
	status, body := apiReq(t, "GET", "/v1/data-exports", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d", status)
	}
	exports := body["data_exports"].([]any)
	if len(exports) != 0 {
		t.Fatalf("expected 0 exports, got %d", len(exports))
	}

	// Create an export.
	status, body = apiReq(t, "POST", "/v1/data-exports", apiKey, map[string]any{"export_type": "full"})
	exportID := body["id"].(string)

	// List again.
	status, body = apiReq(t, "GET", "/v1/data-exports", apiKey, nil)
	exports = body["data_exports"].([]any)
	if len(exports) != 1 {
		t.Fatalf("expected 1 export, got %d", len(exports))
	}

	// Get by ID.
	status, body = apiReq(t, "GET", "/v1/data-exports/"+exportID, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get: status %d", status)
	}
	if body["id"] != exportID {
		t.Fatalf("id mismatch: %v vs %v", body["id"], exportID)
	}
}

func TestDataExportValidation(t *testing.T) {
	_, apiKey := createProviderAPI(t, "de-val-"+uuid.NewString()[:8])

	// Invalid export_type.
	status, _ := apiReq(t, "POST", "/v1/data-exports", apiKey, map[string]any{
		"export_type": "invalid",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid type: status %d, want 400", status)
	}
}

func TestDataExportCrossTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "de-iso-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "de-iso-b-"+uuid.NewString()[:8])

	// Provider A creates an export.
	status, body := apiReq(t, "POST", "/v1/data-exports", keyA, map[string]any{"export_type": "full"})
	exportAID := body["id"].(string)

	// Provider B cannot get A's export.
	status, _ = apiReq(t, "GET", "/v1/data-exports/"+exportAID, keyB, nil)
	if status != http.StatusNotFound {
		t.Fatalf("B get A's export: status %d, want 404", status)
	}

	// Provider B lists their own exports (should be empty).
	status, body = apiReq(t, "GET", "/v1/data-exports", keyB, nil)
	exports := body["data_exports"].([]any)
	if len(exports) != 0 {
		t.Fatalf("B: expected 0 exports, got %d (RLS leak)", len(exports))
	}
}

func TestDeletionProof(t *testing.T) {
	_, apiKey := createProviderAPI(t, "de-prf-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	// Request deletion.
	status, body := apiReq(t, "POST", "/v1/data-deletion", apiKey, map[string]any{
		"reason": "provider offboarding",
	})
	if status != http.StatusOK {
		t.Fatalf("request deletion: status %d, body %v", status, body)
	}
	proofID := body["id"].(string)
	if body["data_hash"] == nil {
		t.Fatal("data_hash must be set")
	}
	if body["proof_signature"] == nil {
		t.Fatal("proof_signature must be set")
	}
	if int(body["record_count"].(float64)) == 0 {
		t.Fatal("record_count should be > 0")
	}

	// Get deletion proof by ID.
	status, body = apiReq(t, "GET", "/v1/deletion-proofs/"+proofID, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get proof: status %d", status)
	}
	if body["id"] != proofID {
		t.Fatalf("id mismatch")
	}

	// List deletion proofs.
	status, body = apiReq(t, "GET", "/v1/deletion-proofs", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list proofs: status %d", status)
	}
	proofs := body["deletion_proofs"].([]any)
	if len(proofs) != 1 {
		t.Fatalf("expected 1 proof, got %d", len(proofs))
	}
}

func TestDeletionProofCrossTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "de-piso-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "de-piso-b-"+uuid.NewString()[:8])

	// Provider A requests deletion.
	status, body := apiReq(t, "POST", "/v1/data-deletion", keyA, map[string]any{"reason": "offboarding"})
	proofAID := body["id"].(string)

	// Provider B cannot get A's proof.
	status, _ = apiReq(t, "GET", "/v1/deletion-proofs/"+proofAID, keyB, nil)
	if status != http.StatusNotFound {
		t.Fatalf("B get A's proof: status %d, want 404", status)
	}

	// Provider B lists proofs (should be empty).
	status, body = apiReq(t, "GET", "/v1/deletion-proofs", keyB, nil)
	proofs := body["deletion_proofs"].([]any)
	if len(proofs) != 0 {
		t.Fatalf("B: expected 0 proofs, got %d (RLS leak)", len(proofs))
	}
}
