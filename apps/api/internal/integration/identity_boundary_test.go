package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestSameEmailDifferentSubjectsAreIsolated proves the platform never
// correlates users across providers by email: the same email provisioned
// under two subjects yields unrelated workspaces, memberships and providers.
func TestSameEmailDifferentSubjectsAreIsolated(t *testing.T) {
	email := "same-email-" + uuid.NewString()[:8] + "@example.com"
	a, err := svc.ProvisionWorkspace(testCtx, "sub-a-"+uuid.NewString()[:8], email, "Alice")
	if err != nil {
		t.Fatalf("provision A: %v", err)
	}
	b, err := svc.ProvisionWorkspace(testCtx, "sub-b-"+uuid.NewString()[:8], email, "Bob")
	if err != nil {
		t.Fatalf("provision B: %v", err)
	}
	if a.Workspace.ID == b.Workspace.ID {
		t.Fatal("same email must not collapse into one workspace")
	}
	if a.Provider.ID == b.Provider.ID {
		t.Fatal("same email must not collapse into one provider")
	}
	if a.Membership.UserSub == b.Membership.UserSub {
		t.Fatal("memberships must carry distinct subjects")
	}
}

// TestTenantOverrideAttemptsRejected verifies that request query/body
// parameters cannot override the tenant derived from the API credential.
func TestTenantOverrideAttemptsRejected(t *testing.T) {
	_, keyA := createProviderAPI(t, "id-bnd-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "id-bnd-b-"+uuid.NewString()[:8])

	// Provider B owns one customer.
	status, body := apiReq(t, "POST", "/v1/customers", keyB, map[string]any{
		"external_id":  "b-only",
		"account_type": "business",
		"display_name": "B Customer",
	})
	if status != http.StatusCreated {
		t.Fatalf("create B customer: status %d, body %v", status, body)
	}

	// A attempts to read B by injecting provider_id into the query.
	status, body = apiReq(t, "GET", "/v1/customers?provider_id="+providerIDForTest(t, keyB), keyA, nil)
	if status != http.StatusForbidden {
		t.Fatalf("A query override: status %d, want 403", status)
	}
	if code := errorCode(body); code != "tenant_context_override" {
		t.Fatalf("A query override code = %q, want tenant_context_override", code)
	}

	// A attempts to create into B by injecting provider/environment into body.
	status, body = apiReq(t, "POST", "/v1/customers", keyA, map[string]any{
		"provider_id":    providerIDForTest(t, keyB),
		"environment_id": environmentIDForTest(t, keyB),
		"external_id":    "sneaky",
		"account_type":   "individual",
		"display_name":   "Sneaky",
	})
	if status != http.StatusForbidden {
		t.Fatalf("A body override create: status %d, want 403", status)
	}
	if code := errorCode(body); code != "tenant_context_override" {
		t.Fatalf("A body override code = %q, want tenant_context_override", code)
	}
	status, body = apiReq(t, "GET", "/v1/customers", keyB, nil)
	for _, item := range body["customers"].([]any) {
		if item.(map[string]any)["external_id"] == "sneaky" {
			t.Fatal("A's override attempt leaked a customer into B")
		}
	}
}

// TestB2BAndB2CCustomerIsolation verifies the same external_id can be used
// independently as a business or individual customer in different providers.
func TestB2BAndB2CCustomerIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "id-acct-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "id-acct-b-"+uuid.NewString()[:8])
	externalID := "shared-" + uuid.NewString()[:8]

	apiReq(t, "POST", "/v1/customers", keyA, map[string]any{
		"external_id": externalID, "account_type": "business", "display_name": "B2B",
	})
	apiReq(t, "POST", "/v1/customers", keyB, map[string]any{
		"external_id": externalID, "account_type": "individual", "display_name": "B2C",
	})

	status, body := apiReq(t, "GET", "/v1/customers", keyA, nil)
	if status != http.StatusOK {
		t.Fatalf("A customers: status %d", status)
	}
	customersA := body["customers"].([]any)
	if len(customersA) != 1 || customersA[0].(map[string]any)["account_type"] != "business" {
		t.Fatalf("A customers = %v, want one business customer", customersA)
	}
	status, body = apiReq(t, "GET", "/v1/customers", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("B customers: status %d", status)
	}
	customersB := body["customers"].([]any)
	if len(customersB) != 1 || customersB[0].(map[string]any)["account_type"] != "individual" {
		t.Fatalf("B customers = %v, want one individual customer", customersB)
	}
}

func providerIDForTest(t *testing.T, apiKey string) string {
	t.Helper()
	status, body := apiReq(t, "GET", "/v1/whoami", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("whoami: status %d", status)
	}
	return body["provider_id"].(string)
}

func environmentIDForTest(t *testing.T, apiKey string) string {
	t.Helper()
	status, body := apiReq(t, "GET", "/v1/whoami", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("whoami: status %d", status)
	}
	return body["environment_id"].(string)
}
