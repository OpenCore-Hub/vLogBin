package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestAPIVersionInfo(t *testing.T) {
	// The API version endpoint is public (no auth required).
	status, body := apiReq(t, "GET", "/v1/api-version", "", nil)
	if status != http.StatusOK {
		t.Fatalf("api-version: status %d, body %v", status, body)
	}
	if body["current_version"] != "v1" {
		t.Fatalf("current_version = %v, want v1", body["current_version"])
	}
	supported := body["supported_versions"].([]any)
	if len(supported) != 1 || supported[0] != "v1" {
		t.Fatalf("supported_versions = %v, want [v1]", supported)
	}
	policy := body["compatibility_policy"].(string)
	if policy == "" {
		t.Fatal("compatibility_policy must be non-empty")
	}
	if body["webhook_schema_version"] != "1.0" {
		t.Fatalf("webhook_schema_version = %v, want 1.0", body["webhook_schema_version"])
	}
	// deprecated_endpoints should be an empty array (no deprecated endpoints yet).
	deprecated := body["deprecated_endpoints"].([]any)
	if len(deprecated) != 0 {
		t.Fatalf("expected 0 deprecated endpoints, got %d", len(deprecated))
	}
}

func TestAPIVersionInfoNoAuth(t *testing.T) {
	// The API version endpoint must be accessible without authentication
	// so that anyone can check the compatibility policy.
	status, _ := apiReq(t, "GET", "/v1/api-version", "", nil)
	if status != http.StatusOK {
		t.Fatalf("no-auth api-version: status %d, want 200", status)
	}
}

func TestWebhookSchemaVersionHeader(t *testing.T) {
	// Verify that webhook deliveries include the X-Webhook-Schema-Version header.
	// This satisfies spec Section 7.2: "Webhook payload 包含 schema_version".
	_, apiKey := createProviderAPI(t, "api-wsv-"+uuid.NewString()[:8])

	// Create a webhook endpoint that captures headers.
	var capturedHeaders http.Header
	wc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer wc.Close()

	// Register the webhook.
	apiReq(t, "POST", "/v1/webhooks", apiKey, map[string]any{
		"url":    wc.URL,
		"events": []string{},
	})

	// Create a catalog and customer to generate outbox events.
	versionID := createPublishedCatalog(t, apiKey)
	createCustomerAndSubscription(t, apiKey, versionID)

	// Drain outbox and webhook.
	drainAllPendingOutbox(t)
	drainWebhookUntilDelivered(t, apiKey)

	// Verify the schema version header was sent.
	if capturedHeaders == nil {
		t.Fatal("webhook was not delivered")
	}
	schemaVersion := capturedHeaders.Get("X-Webhook-Schema-Version")
	if schemaVersion != "1.0" {
		t.Fatalf("X-Webhook-Schema-Version = %q, want 1.0", schemaVersion)
	}
	// Verify other required headers are present.
	if capturedHeaders.Get("X-Webhook-Signature") == "" {
		t.Fatal("X-Webhook-Signature header missing")
	}
	if capturedHeaders.Get("X-Webhook-Event-Type") == "" {
		t.Fatal("X-Webhook-Event-Type header missing")
	}
}
