package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestSCIMUserCRUD(t *testing.T) {
	_, apiKey := createProviderAPI(t, "scim-crud-"+uuid.NewString()[:8])

	// Create SCIM user.
	status, body := apiReq(t, "POST", "/scim/v2/Users", apiKey, map[string]any{
		"externalId":  "user-001",
		"displayName": "Alice Smith",
		"email":       "alice@example.com",
		"active":      true,
	})
	if status != http.StatusCreated {
		t.Fatalf("create: status %d, body %v", status, body)
	}
	userID := body["id"].(string)
	schemas := body["schemas"].([]any)
	if schemas[0] != "urn:ietf:params:scim:schemas:core:2.0:User" {
		t.Fatalf("schema = %v", schemas[0])
	}
	if body["userName"] != "alice@example.com" {
		t.Fatalf("userName = %v", body["userName"])
	}

	// Get by ID.
	status, body = apiReq(t, "GET", "/scim/v2/Users/"+userID, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get: status %d", status)
	}
	if body["externalId"] != "user-001" {
		t.Fatalf("externalId = %v", body["externalId"])
	}

	// List users.
	status, body = apiReq(t, "GET", "/scim/v2/Users", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d", status)
	}
	if body["totalResults"].(float64) != 1 {
		t.Fatalf("totalResults = %v, want 1", body["totalResults"])
	}
	resources := body["Resources"].([]any)
	if len(resources) != 1 {
		t.Fatalf("Resources count = %d, want 1", len(resources))
	}

	// Update user.
	status, body = apiReq(t, "PUT", "/scim/v2/Users/"+userID, apiKey, map[string]any{
		"externalId":  "user-001",
		"displayName": "Alice Smith-Jones",
		"email":       "alice.smith@example.com",
		"active":      false,
	})
	if status != http.StatusOK {
		t.Fatalf("update: status %d, body %v", status, body)
	}
	if body["active"] != false {
		t.Fatalf("active = %v, want false", body["active"])
	}

	// Delete user.
	status, _ = apiReq(t, "DELETE", "/scim/v2/Users/"+userID, apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete: status %d, want 204", status)
	}

	// Get after delete — 404.
	status, _ = apiReq(t, "GET", "/scim/v2/Users/"+userID, apiKey, nil)
	if status != http.StatusNotFound {
		t.Fatalf("get deleted: status %d, want 404", status)
	}
}

func TestSCIMUserIdempotency(t *testing.T) {
	_, apiKey := createProviderAPI(t, "scim-idm-"+uuid.NewString()[:8])

	// Create user.
	status, body1 := apiReq(t, "POST", "/scim/v2/Users", apiKey, map[string]any{
		"externalId":  "user-idem",
		"displayName": "Bob",
		"email":       "bob@example.com",
		"active":      true,
	})
	if status != http.StatusCreated {
		t.Fatalf("create: status %d", status)
	}
	id1 := body1["id"].(string)

	// Create same user again — should return existing (idempotent).
	status, body2 := apiReq(t, "POST", "/scim/v2/Users", apiKey, map[string]any{
		"externalId":  "user-idem",
		"displayName": "Bob",
		"email":       "bob@example.com",
		"active":      true,
	})
	if status != http.StatusCreated {
		t.Fatalf("idempotent create: status %d", status)
	}
	if body2["id"] != id1 {
		t.Fatalf("idempotent create returned different id: %v vs %v", body2["id"], id1)
	}
}

func TestSCIMUserValidation(t *testing.T) {
	_, apiKey := createProviderAPI(t, "scim-val-"+uuid.NewString()[:8])

	// Missing externalId.
	status, _ := apiReq(t, "POST", "/scim/v2/Users", apiKey, map[string]any{
		"displayName": "No ID",
		"email":       "noid@example.com",
		"active":      true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("missing externalId: status %d, want 400", status)
	}

	// Missing email.
	status, _ = apiReq(t, "POST", "/scim/v2/Users", apiKey, map[string]any{
		"externalId":  "no-email",
		"displayName": "No Email",
		"active":      true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("missing email: status %d, want 400", status)
	}
}

func TestSCIMUserCrossTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "scim-iso-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "scim-iso-b-"+uuid.NewString()[:8])

	// Provider A creates a user.
	status, body := apiReq(t, "POST", "/scim/v2/Users", keyA, map[string]any{
		"externalId":  "iso-user",
		"displayName": "Iso User",
		"email":       "iso@example.com",
		"active":      true,
	})
	if status != http.StatusCreated {
		t.Fatalf("A create: status %d", status)
	}
	userAID := body["id"].(string)

	// Provider B cannot get A's user.
	status, _ = apiReq(t, "GET", "/scim/v2/Users/"+userAID, keyB, nil)
	if status != http.StatusNotFound {
		t.Fatalf("B get A's user: status %d, want 404", status)
	}

	// Provider B cannot list A's users.
	status, body = apiReq(t, "GET", "/scim/v2/Users", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("B list: status %d", status)
	}
	if body["totalResults"].(float64) != 0 {
		t.Fatalf("B: totalResults = %v, want 0 (RLS leak)", body["totalResults"])
	}
}
