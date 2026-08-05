// Operator credential lifecycle tests: the operator console can list a
// provider's API keys across all environments and revoke them immediately.
// Revocation is durable (credentials.revoked_at), recorded on the provider's
// audit trail with actor_type=operator, and enforced by the authentication
// middleware, so a revoked key stops working instantly. The operator view
// never exposes key_hash.
package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestOperatorCredentialLifecycle exercises list + revoke on one activated
// provider, then verifies the revoked key is rejected by the API and the
// action is visible on the provider's audit trail.
func TestOperatorCredentialLifecycle(t *testing.T) {
	providerID := insertRegisteredProvider(t)

	status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/activate",
		operatorToken, map[string]any{"home_region_code": regionCode})
	if status != http.StatusOK {
		t.Fatalf("activate: status %d, body %v", status, body)
	}
	testKey, _ := body["api_key"].(string)
	if testKey == "" {
		t.Fatal("activate must return the initial test api_key")
	}

	// The key works before revocation.
	if status, _ := apiReq(t, "GET", "/v1/whoami", testKey, nil); status != http.StatusOK {
		t.Fatalf("whoami before revoke: status %d, want 200", status)
	}

	// List: exactly one key (initial-test-key), no key_hash, test environment.
	status, body = apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/credentials",
		operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list credentials: status %d, body %v", status, body)
	}
	creds, ok := body["credentials"].([]any)
	if !ok || len(creds) != 1 {
		t.Fatalf("credentials = %v, want exactly 1", body["credentials"])
	}
	cred := creds[0].(map[string]any)
	if _, leaked := cred["key_hash"]; leaked {
		t.Fatal("operator view must never expose key_hash")
	}
	if prefix, _ := cred["key_prefix"].(string); prefix == "" {
		t.Fatal("key_prefix must identify the key")
	}
	if cred["environment_kind"] != "test" {
		t.Fatalf("environment_kind = %v, want test", cred["environment_kind"])
	}
	credID, _ := cred["id"].(string)
	if credID == "" {
		t.Fatal("credential id missing from view")
	}

	// Revoke with an explicit actor identity.
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/credentials/"+credID+"/revoke",
		operatorToken, map[string]any{"revoked_by": "alice@platform"})
	if status != http.StatusOK {
		t.Fatalf("revoke: status %d, body %v", status, body)
	}
	revoked, _ := body["credential"].(map[string]any)
	if revoked["revoked_at"] == nil {
		t.Fatalf("revoked_at must be set after revocation: %v", body)
	}

	// The revoked key is rejected immediately (middleware checks revoked_at).
	if status, _ := apiReq(t, "GET", "/v1/whoami", testKey, nil); status != http.StatusUnauthorized {
		t.Fatalf("whoami after revoke: status %d, want 401", status)
	}

	// List again: the same key now shows revoked_at.
	status, body = apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/credentials",
		operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list after revoke: status %d, body %v", status, body)
	}
	after := body["credentials"].([]any)[0].(map[string]any)
	if after["revoked_at"] == nil {
		t.Fatal("credential must show revoked_at after revocation")
	}

	// The audit trail records the operator action with the actor's identity.
	status, body = apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/audit",
		operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("audit: status %d, body %v", status, body)
	}
	found := false
	for _, ev := range body["audit_events"].([]any) {
		rec, ok := ev.(map[string]any)
		if !ok || rec["action"] != "credential.revoke" {
			continue
		}
		found = true
		if rec["actor_type"] != "operator" {
			t.Errorf("actor_type = %v, want operator", rec["actor_type"])
		}
		if rec["actor_id"] != "alice@platform" {
			t.Errorf("actor_id = %v, want alice@platform", rec["actor_id"])
		}
	}
	if !found {
		t.Fatal("audit trail missing credential.revoke event")
	}

	// Revoking an already-revoked key conflicts.
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/credentials/"+credID+"/revoke",
		operatorToken, nil)
	if status != http.StatusConflict {
		t.Fatalf("double revoke: status %d, want 409; body %v", status, body)
	}
	if code := errorCode(body); code != "conflict" {
		t.Fatalf("double revoke code = %q, want conflict", code)
	}
}

// TestOperatorCredentialsUnknownProvider: unknown providers yield 404 for both
// list and revoke so console typos surface immediately.
func TestOperatorCredentialsUnknownProvider(t *testing.T) {
	id := uuid.NewString()
	status, body := apiReq(t, "GET", "/v1/operator/providers/"+id+"/credentials",
		operatorToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("list: status %d, want 404; body %v", status, body)
	}
	if code := errorCode(body); code != "not_found" {
		t.Fatalf("list code = %q, want not_found", code)
	}
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+id+"/credentials/"+uuid.NewString()+"/revoke",
		operatorToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("revoke: status %d, want 404; body %v", status, body)
	}
}

// TestOperatorRevokeUnknownCredential: a credential id that does not belong to
// the provider yields 404, never a cross-provider revocation.
func TestOperatorRevokeUnknownCredential(t *testing.T) {
	providerID := insertRegisteredProvider(t)
	if status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/activate",
		operatorToken, map[string]any{"home_region_code": regionCode}); status != http.StatusOK {
		t.Fatalf("activate: status %d, body %v", status, body)
	}
	status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/credentials/"+uuid.NewString()+"/revoke",
		operatorToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body %v", status, body)
	}
	if code := errorCode(body); code != "not_found" {
		t.Fatalf("code = %q, want not_found", code)
	}
}
