package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestProviderCapabilities(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "cap-"+uuid.NewString()[:8])

	// Grant messaging capability.
	status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/capabilities/messaging/grant", operatorToken, map[string]any{
		"granted_by": "admin@example.com",
		"reason":     "email verified",
	})
	if status != http.StatusOK {
		t.Fatalf("grant messaging: status %d, body %v", status, body)
	}
	cap := body["capability"].(map[string]any)
	if cap["status"] != "granted" {
		t.Fatalf("status = %v, want granted", cap["status"])
	}

	// Grant payments capability (independent grant — spec ID #46).
	status, _ = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/capabilities/payments/grant", operatorToken, map[string]any{
		"granted_by": "admin@example.com",
		"reason":     "PSP connected",
	})
	if status != http.StatusOK {
		t.Fatalf("grant payments: status %d", status)
	}

	// List capabilities (operator view).
	status, body = apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/capabilities", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d", status)
	}
	caps, ok := body["capabilities"].([]any)
	if !ok || len(caps) != 2 {
		t.Fatalf("expected 2 capabilities, got %v", body["capabilities"])
	}
	findCap := func(name, wantStatus string) {
		t.Helper()
		for _, c := range caps {
			cm := c.(map[string]any)
			if cm["capability"] == name {
				if cm["status"] != wantStatus {
					t.Fatalf("%s status = %v, want %s", name, cm["status"], wantStatus)
				}
				return
			}
		}
		t.Fatalf("capability %s not found", name)
	}
	findCap("messaging", "granted")
	findCap("payments", "granted")

	// Provider can view their own capabilities.
	status, body = apiReq(t, "GET", "/v1/capabilities", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("provider list: status %d", status)
	}
	if caps, ok := body["capabilities"].([]any); !ok || len(caps) != 2 {
		t.Fatalf("provider sees %v, want 2 capabilities", body["capabilities"])
	}

	// Revoke messaging capability.
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/capabilities/messaging/revoke", operatorToken, map[string]any{
		"reason": "policy violation",
	})
	if status != http.StatusOK {
		t.Fatalf("revoke: status %d, body %v", status, body)
	}

	// Verify it's revoked.
	status, body = apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/capabilities", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list after revoke: status %d, body %v", status, body)
	}
	caps, ok = body["capabilities"].([]any)
	if !ok {
		t.Fatalf("expected capabilities array, got status %d body %v", status, body)
	}
	findCap("messaging", "revoked")
	findCap("payments", "granted") // payments still granted (independent)

	// Invalid capability → 400.
	status, _ = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/capabilities/invalid_cap/grant", operatorToken, map[string]any{
		"granted_by": "admin",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid capability: status %d, want 400", status)
	}

	// Provider cannot grant capabilities (operator-only).
	status, _ = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/capabilities/domains/grant", apiKey, map[string]any{
		"granted_by": "self",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("provider grant attempt: status %d, want 401", status)
	}

	// Idempotent grant: re-grant messaging with a different granted_by.
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/capabilities/messaging/grant", operatorToken, map[string]any{
		"granted_by": "admin2@example.com",
		"reason":     "re-granted after review",
	})
	if status != http.StatusOK {
		t.Fatalf("re-grant messaging: status %d, body %v", status, body)
	}
	cap = body["capability"].(map[string]any)
	if cap["granted_by"] != "admin2@example.com" {
		t.Fatalf("granted_by = %v, want admin2@example.com", cap["granted_by"])
	}

	// Revoke again — granted_by must be preserved (DB CASE expression).
	status, _ = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/capabilities/messaging/revoke", operatorToken, map[string]any{
		"reason": "final revoke",
	})
	if status != http.StatusOK {
		t.Fatalf("final revoke: status %d", status)
	}

	// Verify granted_by is still admin2@example.com after revoke.
	status, body = apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/capabilities", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list after final revoke: status %d", status)
	}
	caps, ok = body["capabilities"].([]any)
	if !ok {
		t.Fatalf("expected capabilities array, got %v", body["capabilities"])
	}
	for _, c := range caps {
		cm := c.(map[string]any)
		if cm["capability"] == "messaging" {
			if cm["status"] != "revoked" {
				t.Fatalf("messaging status = %v, want revoked", cm["status"])
			}
			if cm["granted_by"] != "admin2@example.com" {
				t.Fatalf("granted_by = %v, want admin2@example.com (preserved on revoke)", cm["granted_by"])
			}
			break
		}
	}
}
