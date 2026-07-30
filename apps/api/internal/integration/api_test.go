package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// apiReq performs a request against the test server and returns the status
// code plus the decoded JSON body (nil when empty).
func apiReq(t *testing.T, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, httpServer.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var decoded map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("%s %s: undecodable body %q: %v", method, path, raw, err)
		}
	}
	return resp.StatusCode, decoded
}

func createProviderAPI(t *testing.T, slug string) (providerID string, apiKey string) {
	t.Helper()
	status, body := apiReq(t, "POST", "/v1/operator/providers", operatorToken, map[string]any{
		"slug": slug, "name": slug + " name", "home_region_code": regionCode,
	})
	if status != http.StatusCreated {
		t.Fatalf("create provider: status %d, body %v", status, body)
	}
	apiKey, _ = body["api_key"].(string)
	if !strings.HasPrefix(apiKey, "pk_test_") {
		t.Fatalf("expected pk_test_ key, got %q", apiKey)
	}
	provider := body["provider"].(map[string]any)
	providerID, _ = provider["id"].(string)
	envs := body["environments"].([]any)
	if len(envs) != 1 {
		t.Fatalf("expected 1 auto-created test environment, got %d", len(envs))
	}
	env := envs[0].(map[string]any)
	if env["kind"] != "test" {
		t.Fatalf("auto environment kind = %v", env["kind"])
	}
	issuer, _ := env["issuer"].(string)
	if !strings.HasPrefix(issuer, "https://"+slug+".test."+baseDomain) {
		t.Fatalf("unexpected issuer %q", issuer)
	}
	if provider["lifecycle_state"] != "TEST_ACTIVE" {
		t.Fatalf("lifecycle = %v, want TEST_ACTIVE", provider["lifecycle_state"])
	}
	return providerID, apiKey
}

func TestOperatorProviderLifecycleFlow(t *testing.T) {
	slug := "flow-" + uuid.NewString()[:8]
	providerID, testKey := createProviderAPI(t, slug)

	// whoami with the test key.
	status, body := apiReq(t, "GET", "/v1/whoami", testKey, nil)
	if status != http.StatusOK {
		t.Fatalf("whoami: status %d, body %v", status, body)
	}
	if body["provider_id"] != providerID || body["slug"] != slug || body["environment_kind"] != "test" {
		t.Fatalf("whoami mismatch: %v", body)
	}
	if body["issuer"] != "https://"+slug+".test."+baseDomain {
		t.Fatalf("whoami issuer = %v", body["issuer"])
	}

	// List + get provider as operator.
	status, body = apiReq(t, "GET", "/v1/operator/providers", operatorToken, nil)
	if status != http.StatusOK || body["providers"] == nil {
		t.Fatalf("list providers: status %d, body %v", status, body)
	}
	status, body = apiReq(t, "GET", "/v1/operator/providers/"+providerID, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("get provider: status %d, body %v", status, body)
	}
	if envs := body["environments"].([]any); len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}

	// Regions and cells are listed for operators.
	if status, body := apiReq(t, "GET", "/v1/operator/regions", operatorToken, nil); status != http.StatusOK {
		t.Fatalf("regions: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, "GET", "/v1/operator/cells", operatorToken, nil); status != http.StatusOK {
		t.Fatalf("cells: status %d, body %v", status, body)
	}

	// Invalid transition: TEST_ACTIVE -> LIVE_ACTIVE must be rejected.
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle", operatorToken,
		map[string]any{"to": "LIVE_ACTIVE"})
	if status != http.StatusConflict {
		t.Fatalf("invalid transition: status %d, body %v", status, body)
	}
	if errObj := body["error"].(map[string]any); errObj["code"] != "invalid_transition" {
		t.Fatalf("error body = %v", body)
	}

	// TEST_ACTIVE -> LIVE_REVIEW -> LIVE_ACTIVE.
	if status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle", operatorToken,
		map[string]any{"to": "LIVE_REVIEW"}); status != http.StatusOK {
		t.Fatalf("LIVE_REVIEW: status %d, body %v", status, body)
	}
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle", operatorToken,
		map[string]any{"to": "LIVE_ACTIVE"})
	if status != http.StatusOK {
		t.Fatalf("LIVE_ACTIVE: status %d, body %v", status, body)
	}
	liveKey, _ := body["api_key"].(string)
	if !strings.HasPrefix(liveKey, "pk_live_") {
		t.Fatalf("expected pk_live_ key from live activation, got %q", liveKey)
	}

	// The live key resolves to the live environment.
	status, body = apiReq(t, "GET", "/v1/whoami", liveKey, nil)
	if status != http.StatusOK || body["environment_kind"] != "live" {
		t.Fatalf("live whoami: status %d, body %v", status, body)
	}
	if body["issuer"] != "https://"+slug+".live."+baseDomain {
		t.Fatalf("live issuer = %v", body["issuer"])
	}

	// Provider now has both environments; issuers are stable.
	status, body = apiReq(t, "GET", "/v1/operator/providers/"+providerID, operatorToken, nil)
	if status != http.StatusOK || len(body["environments"].([]any)) != 2 {
		t.Fatalf("expected 2 environments: status %d, body %v", status, body)
	}
}

func TestOperatorAuthFailures(t *testing.T) {
	if status, _ := apiReq(t, "GET", "/v1/operator/providers", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("no token: status %d, want 401", status)
	}
	if status, _ := apiReq(t, "GET", "/v1/operator/providers", "wrong-token", nil); status != http.StatusUnauthorized {
		t.Fatalf("wrong token: status %d, want 401", status)
	}
}

func TestAPIKeyAuthFailures(t *testing.T) {
	if status, _ := apiReq(t, "GET", "/v1/whoami", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("no key: status %d, want 401", status)
	}
	if status, _ := apiReq(t, "GET", "/v1/whoami", "pk_test_deadbeefdeadbeefdeadbeefdeadbeef", nil); status != http.StatusUnauthorized {
		t.Fatalf("unknown key: status %d, want 401", status)
	}
	if status, _ := apiReq(t, "GET", "/v1/whoami", "not-a-key", nil); status != http.StatusUnauthorized {
		t.Fatalf("malformed key: status %d, want 401", status)
	}
	// An operator token is not an API key.
	if status, _ := apiReq(t, "GET", "/v1/whoami", operatorToken, nil); status != http.StatusUnauthorized {
		t.Fatalf("operator token on provider route: status %d, want 401", status)
	}
}

func TestCredentialRotationAndRevocation(t *testing.T) {
	_, initialKey := createProviderAPI(t, "rot-"+uuid.NewString()[:8])

	// Create a second key (rotation step 1).
	status, body := apiReq(t, "POST", "/v1/credentials", initialKey, map[string]any{
		"name": "rotated", "scopes": []string{"read"},
	})
	if status != http.StatusCreated {
		t.Fatalf("create credential: status %d, body %v", status, body)
	}
	newKey, _ := body["api_key"].(string)
	newCredID := body["credential"].(map[string]any)["id"].(string)
	if !strings.HasPrefix(newKey, "pk_test_") {
		t.Fatalf("rotated key = %q", newKey)
	}

	// New key works (read scope).
	if status, _ := apiReq(t, "GET", "/v1/whoami", newKey, nil); status != http.StatusOK {
		t.Fatalf("new key whoami: status %d", status)
	}
	// ...but lacks credentials:manage.
	if status, body := apiReq(t, "POST", "/v1/credentials", newKey, map[string]any{
		"name": "x", "scopes": []string{"read"},
	}); status != http.StatusForbidden {
		t.Fatalf("insufficient scope: status %d, body %v", status, body)
	}
	// ...and lacks audit:read.
	if status, _ := apiReq(t, "GET", "/v1/audit-events", newKey, nil); status != http.StatusForbidden {
		t.Fatalf("audit without scope: status %d, want 403", status)
	}

	// List credentials: two rows, no hashes exposed.
	status, body = apiReq(t, "GET", "/v1/credentials", initialKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list credentials: status %d", status)
	}
	creds := body["credentials"].([]any)
	if len(creds) != 2 {
		t.Fatalf("expected 2 credentials, got %d", len(creds))
	}
	for _, c := range creds {
		cm := c.(map[string]any)
		if _, leaked := cm["key_hash"]; leaked {
			t.Fatal("key_hash must never be exposed")
		}
	}

	// Revoke the new key (rotation step 2), revocation is immediate.
	status, _ = apiReq(t, "POST", "/v1/credentials/"+newCredID+"/revoke", initialKey, nil)
	if status != http.StatusOK {
		t.Fatalf("revoke: status %d", status)
	}
	if status, body := apiReq(t, "GET", "/v1/whoami", newKey, nil); status != http.StatusUnauthorized {
		t.Fatalf("revoked key: status %d, body %v", status, body)
	}
}

func TestExpiredKeyRejected(t *testing.T) {
	_, initialKey := createProviderAPI(t, "exp-"+uuid.NewString()[:8])
	status, body := apiReq(t, "POST", "/v1/credentials", initialKey, map[string]any{
		"name": "shortlived", "scopes": []string{"read"}, "expires_at": "2000-01-01T00:00:00Z",
	})
	if status != http.StatusCreated {
		t.Fatalf("create expiring credential: status %d, body %v", status, body)
	}
	expiredKey := body["api_key"].(string)
	if status, body := apiReq(t, "GET", "/v1/whoami", expiredKey, nil); status != http.StatusUnauthorized {
		t.Fatalf("expired key: status %d, body %v", status, body)
	}
}

func TestTenantContextOverrideRejected(t *testing.T) {
	_, keyA := createProviderAPI(t, "ovr-a-"+uuid.NewString()[:8])
	idB, _ := createProviderAPI(t, "ovr-b-"+uuid.NewString()[:8])

	// Body override attempt -> 403 with the dedicated error code.
	status, body := apiReq(t, "POST", "/v1/credentials", keyA, map[string]any{
		"name": "evil", "scopes": []string{"read"}, "provider_id": idB,
	})
	if status != http.StatusForbidden {
		t.Fatalf("body override: status %d, body %v", status, body)
	}
	if errObj := body["error"].(map[string]any); errObj["code"] != "tenant_context_override" {
		t.Fatalf("error body = %v", body)
	}

	// Query override attempt -> 403.
	if status, _ := apiReq(t, "GET", "/v1/credentials?environment_id="+uuid.NewString(), keyA, nil); status != http.StatusForbidden {
		t.Fatalf("query override: status %d, want 403", status)
	}

	// The attempt was audited and visible to provider A via the audit API.
	status, body = apiReq(t, "GET", "/v1/audit-events", keyA, nil)
	if status != http.StatusOK {
		t.Fatalf("audit list: status %d", status)
	}
	found := false
	for _, e := range body["audit_events"].([]any) {
		if e.(map[string]any)["action"] == "tenant.context_override_attempt" {
			found = true
		}
	}
	if !found {
		t.Fatal("override attempt must produce an audit record")
	}
}

func TestCrossTenantViaAPI(t *testing.T) {
	_, keyA := createProviderAPI(t, "x-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "x-b-"+uuid.NewString()[:8])

	// A's credential list contains only A's credentials; B's only B's.
	status, body := apiReq(t, "GET", "/v1/credentials", keyA, nil)
	if status != http.StatusOK || len(body["credentials"].([]any)) != 1 {
		t.Fatalf("list A: status %d, body %v", status, body)
	}
	status, bodyB := apiReq(t, "GET", "/v1/credentials", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("list B: %d", status)
	}
	// Try to revoke B's credential with A's key: not found (RLS hides it).
	credB := bodyB["credentials"].([]any)[0].(map[string]any)["id"].(string)
	if status, _ := apiReq(t, "POST", "/v1/credentials/"+credB+"/revoke", keyA, nil); status != http.StatusNotFound {
		t.Fatalf("cross-tenant revoke: status %d, want 404", status)
	}
}

func TestOutboxVisibleViaAPI(t *testing.T) {
	_, key := createProviderAPI(t, "obx-"+uuid.NewString()[:8])
	status, body := apiReq(t, "GET", "/v1/outbox-events", key, nil)
	if status != http.StatusOK {
		t.Fatalf("outbox list: status %d", status)
	}
	events := body["outbox_events"].([]any)
	if len(events) == 0 {
		t.Fatal("provider creation must emit an outbox event")
	}
	found := false
	for _, e := range events {
		em := e.(map[string]any)
		if em["event_type"] == "provider.created" {
			found = true
			if em["payload_hash"] == "" || em["transaction_id"] == "" {
				t.Fatalf("outbox event missing hash/txid: %v", em)
			}
		}
	}
	if !found {
		t.Fatal("provider.created outbox event not found")
	}
}

func TestCredentialScopeAttenuation(t *testing.T) {
	_, initialKey := createProviderAPI(t, "att-"+uuid.NewString()[:8])

	// Mint a key holding only credentials:manage.
	status, body := apiReq(t, "POST", "/v1/credentials", initialKey, map[string]any{
		"name": "manager", "scopes": []string{"credentials:manage"},
	})
	if status != http.StatusCreated {
		t.Fatalf("create manager key: status %d, body %v", status, body)
	}
	managerKey, _ := body["api_key"].(string)

	// It can manage credentials, but must not mint a key carrying scopes
	// it does not hold itself.
	status, body = apiReq(t, "POST", "/v1/credentials", managerKey, map[string]any{
		"name": "escalated", "scopes": []string{"read"},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("scope escalation: status %d, body %v", status, body)
	}
	// Granting within its own scopes still works.
	status, body = apiReq(t, "POST", "/v1/credentials", managerKey, map[string]any{
		"name": "ok", "scopes": []string{"credentials:manage"},
	})
	if status != http.StatusCreated {
		t.Fatalf("attenuated grant: status %d, body %v", status, body)
	}
}
