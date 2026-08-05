package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestPortalSessionAndDashboard covers the customer portal closed loop:
// operator issues a portal token, the public session endpoint validates it,
// and the portal dashboard only returns the token's own customer data.
func TestPortalSessionAndDashboard(t *testing.T) {
	providerID, _ := createProviderAPI(t, "portal-"+uuid.NewString()[:8])
	customersBase := "/v1/operator/providers/" + providerID + "/customers?env=test"

	status, body := apiReq(t, "POST", customersBase, operatorToken, map[string]any{
		"external_id": "portal-cust-a", "account_type": "business", "display_name": "Portal A",
	})
	if status != http.StatusCreated {
		t.Fatalf("create customer A: status %d, body %v", status, body)
	}
	status, body = apiReq(t, "POST", customersBase, operatorToken, map[string]any{
		"external_id": "portal-cust-b", "account_type": "individual", "display_name": "Portal B",
	})
	if status != http.StatusCreated {
		t.Fatalf("create customer B: status %d, body %v", status, body)
	}

	// Issue a portal token for customer A.
	status, body = apiReq(t, "POST",
		"/v1/operator/providers/"+providerID+"/customers/portal-cust-a/portal-token?env=test",
		operatorToken, nil)
	if status != http.StatusCreated {
		t.Fatalf("issue portal token: status %d, body %v", status, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("portal token must be returned")
	}

	// Public session validation.
	status, body = apiReq(t, "POST", "/v1/portal/sessions", "", map[string]any{"token": token})
	if status != http.StatusOK {
		t.Fatalf("validate portal session: status %d, body %v", status, body)
	}
	if body["valid"] != true || body["customer_external_id"] != "portal-cust-a" {
		t.Fatalf("portal session claims = %v", body)
	}

	// Dashboard is scoped to customer A only.
	status, body = apiReq(t, "GET", "/v1/portal/dashboard", token, nil)
	if status != http.StatusOK {
		t.Fatalf("portal dashboard: status %d, body %v", status, body)
	}
	customer := body["customer"].(map[string]any)
	if customer["external_id"] != "portal-cust-a" {
		t.Fatalf("dashboard customer = %v, want portal-cust-a", customer)
	}
	if body["provider_name"] == "" {
		t.Fatal("dashboard must include provider branding")
	}

	// A token for A cannot surface B's customer id through any portal route.
	status, body = apiReq(t, "GET", "/v1/portal/dashboard", token, nil)
	if status != http.StatusOK {
		t.Fatalf("dashboard recheck: status %d, body %v", status, body)
	}
	if body["customer"].(map[string]any)["external_id"] == "portal-cust-b" {
		t.Fatal("portal dashboard leaked customer B data")
	}
}

// TestPortalInvalidToken verifies the portal error contract.
func TestPortalInvalidToken(t *testing.T) {
	status, body := apiReq(t, "GET", "/v1/portal/dashboard", "not-a-token", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("invalid token: status %d, body %v", status, body)
	}
	if code := errorCode(body); code != "invalid_portal_token" {
		t.Fatalf("error code = %q, want invalid_portal_token", code)
	}
	status, body = apiReq(t, "POST", "/v1/portal/sessions", "", map[string]any{"token": "not-a-token"})
	if status != http.StatusUnauthorized {
		t.Fatalf("invalid session token: status %d, body %v", status, body)
	}
}
