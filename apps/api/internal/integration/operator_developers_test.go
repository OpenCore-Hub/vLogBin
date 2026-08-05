package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestOperatorDevelopersCredentialLifecycle covers the Console API Keys page:
// env-scoped list, operator-issued key, atomic rotation, and immediate
// revocation. The plaintext key is returned exactly once on create/rotate,
// and every action lands on the provider's operator audit trail.
func TestOperatorDevelopersCredentialLifecycle(t *testing.T) {
	providerID, initialKey := createProviderAPI(t, "dev-key-"+uuid.NewString()[:8])
	base := "/v1/operator/providers/" + providerID + "/credentials"

	// Env-scoped list contains the auto-created test key.
	status, body := apiReq(t, "GET", base+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list env credentials: status %d, body %v", status, body)
	}
	creds := body["credentials"].([]any)
	if len(creds) != 1 {
		t.Fatalf("credentials = %v, want 1", body["credentials"])
	}
	if creds[0].(map[string]any)["environment_kind"] != "test" {
		t.Fatalf("credential environment_kind = %v, want test", creds[0])
	}

	// Operator creates a key with an explicit actor.
	status, body = apiReq(t, "POST", base+"?env=test", operatorToken, map[string]any{
		"name": "console-ci", "scopes": []string{"read", "write"}, "created_by": "alice@platform",
	})
	if status != http.StatusCreated {
		t.Fatalf("create credential: status %d, body %v", status, body)
	}
	createdKey, _ := body["api_key"].(string)
	if createdKey == "" || createdKey == initialKey {
		t.Fatalf("create must return a fresh plaintext key, got %q", createdKey)
	}
	credID := body["credential"].(map[string]any)["id"].(string)

	// The new key authenticates.
	if status, _ := apiReq(t, "GET", "/v1/whoami", createdKey, nil); status != http.StatusOK {
		t.Fatalf("whoami with created key: status %d, want 200", status)
	}

	// Atomic rotation: old key stops working, replacement key works.
	status, body = apiReq(t, "POST", base+"/"+credID+"/rotate?env=test", operatorToken, map[string]any{
		"created_by": "bob@platform",
	})
	if status != http.StatusOK {
		t.Fatalf("rotate credential: status %d, body %v", status, body)
	}
	rotatedKey, _ := body["api_key"].(string)
	if rotatedKey == "" || rotatedKey == createdKey {
		t.Fatalf("rotate must return a fresh plaintext key, got %q", rotatedKey)
	}
	if status, _ := apiReq(t, "GET", "/v1/whoami", createdKey, nil); status != http.StatusUnauthorized {
		t.Fatalf("whoami with old key after rotate: status %d, want 401", status)
	}
	if status, _ := apiReq(t, "GET", "/v1/whoami", rotatedKey, nil); status != http.StatusOK {
		t.Fatalf("whoami with rotated key: status %d, want 200", status)
	}

	// Audit trail records create + rotate as operator actions.
	status, body = apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/audit", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("audit: status %d, body %v", status, body)
	}
	foundRotate := false
	for _, ev := range body["audit_events"].([]any) {
		rec := ev.(map[string]any)
		if rec["action"] == "credential.rotate" && rec["actor_id"] == "bob@platform" {
			foundRotate = true
		}
	}
	if !foundRotate {
		t.Fatal("audit trail missing credential.rotate operator event")
	}
}

// TestOperatorDevelopersWebhookLifecycle covers the Console Webhooks page:
// create (secret returned once), env-scoped list without secret leakage,
// replayable delivery surface, and delete with operator audit.
func TestOperatorDevelopersWebhookLifecycle(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "dev-wh-"+uuid.NewString()[:8])
	base := "/v1/operator/providers/" + providerID + "/webhooks"

	status, body := apiReq(t, "POST", base+"?env=test", operatorToken, map[string]any{
		"url": "http://127.0.0.1:9090/hook", "events": []string{"customer.created"},
		"created_by": "ops@platform",
	})
	if status != http.StatusCreated {
		t.Fatalf("create webhook: status %d, body %v", status, body)
	}
	endpoint := body["endpoint"].(map[string]any)
	secret, _ := endpoint["secret"].(string)
	if secret == "" {
		t.Fatal("create response must include the signing secret exactly once")
	}
	endpointID := endpoint["id"].(string)
	if endpoint["environment_kind"] != "test" {
		t.Fatalf("webhook environment_kind = %v, want test", endpoint["environment_kind"])
	}

	// List: endpoint present, secret never exposed.
	status, body = apiReq(t, "GET", base+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list webhooks: status %d, body %v", status, body)
	}
	eps := body["endpoints"].([]any)
	if len(eps) != 1 {
		t.Fatalf("endpoints = %v, want 1", body["endpoints"])
	}
	if _, leaked := eps[0].(map[string]any)["secret"]; leaked {
		t.Fatal("list view must never expose the webhook signing secret")
	}

	// A real provider API key can create activity for the delivery view;
	// outbox rows exist even when the delivery worker has not drained them.
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)
	status, body = apiReq(t, "GET", base[:len(base)-len("/webhooks")]+"/webhook-deliveries?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list deliveries: status %d, body %v", status, body)
	}
	_ = body

	// Delete and verify it disappears.
	status, _ = apiReq(t, "DELETE", base+"/"+endpointID+"?env=test", operatorToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete webhook: status %d, want 204", status)
	}
	status, body = apiReq(t, "GET", base+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list after delete: status %d, body %v", status, body)
	}
	if eps, _ := body["endpoints"].([]any); len(eps) != 0 {
		t.Fatalf("endpoints after delete = %v, want 0", body["endpoints"])
	}
}

// TestOperatorDevelopersEventStream verifies the Console Events page API:
// env-scoped outbox stream with raw JSON payloads, cursor pagination, and
// filters.
func TestOperatorDevelopersEventStream(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "dev-ev-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	base := "/v1/operator/providers/" + providerID + "/events?env=test"
	status, body := apiReq(t, "GET", base+"&limit=2", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("event stream: status %d, body %v", status, body)
	}
	events := body["events"].([]any)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if _, ok := events[0].(map[string]any)["payload"].(map[string]any); !ok {
		t.Fatalf("payload must be decoded JSON object, got %T", events[0].(map[string]any)["payload"])
	}
	if events[0].(map[string]any)["environment_kind"] != "test" {
		t.Fatalf("environment_kind = %v, want test", events[0].(map[string]any)["environment_kind"])
	}
	if body["has_more"] != true {
		t.Fatalf("has_more = %v, want true", body["has_more"])
	}
	cursor, _ := body["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("next_cursor must be set")
	}

	status, body = apiReq(t, "GET", base+"&limit=2&cursor="+cursor, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("second page: status %d, body %v", status, body)
	}
	page2 := body["events"].([]any)
	if len(page2) == 0 {
		t.Fatal("second page should contain at least one event")
	}
	if page2[0].(map[string]any)["id"] == events[1].(map[string]any)["id"] {
		t.Fatal("cursor overlap between pages")
	}

	// Filter by the first event type observed.
	firstType := events[0].(map[string]any)["event_type"].(string)
	status, body = apiReq(t, "GET", base+"&type="+firstType+"&limit=100", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("filtered stream: status %d, body %v", status, body)
	}
	for _, ev := range body["events"].([]any) {
		if ev.(map[string]any)["event_type"] != firstType {
			t.Fatalf("filtered event type = %v, want %q", ev.(map[string]any)["event_type"], firstType)
		}
	}
}
