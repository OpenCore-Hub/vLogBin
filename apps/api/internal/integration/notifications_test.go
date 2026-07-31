package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestNotificationConfigCRUD(t *testing.T) {
	_, apiKey := createProviderAPI(t, "nc-crud-"+uuid.NewString()[:8])

	// Set email config.
	status, body := apiReq(t, "PUT", "/v1/notification-configs", apiKey, map[string]any{
		"channel":       "email",
		"provider_type": "smtp",
		"config":        map[string]any{"host": "smtp.example.com", "port": 587, "username": "user", "password": "secret"},
		"from_address":  "noreply@example.com",
		"enabled":       true,
	})
	if status != http.StatusOK {
		t.Fatalf("set email config: status %d, body %v", status, body)
	}
	if body["channel"] != "email" {
		t.Fatalf("channel = %v", body["channel"])
	}
	// Verify config is decrypted in response.
	cfg := body["config"].(map[string]any)
	if cfg["host"] != "smtp.example.com" {
		t.Fatalf("config.host = %v", cfg["host"])
	}

	// Get email config.
	status, body = apiReq(t, "GET", "/v1/notification-configs/email", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get email config: status %d", status)
	}
	if body["from_address"] != "noreply@example.com" {
		t.Fatalf("from_address = %v", body["from_address"])
	}

	// List configs.
	status, body = apiReq(t, "GET", "/v1/notification-configs", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d", status)
	}
	configs := body["notification_configs"].([]any)
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}

	// Set SMS config.
	apiReq(t, "PUT", "/v1/notification-configs", apiKey, map[string]any{
		"channel":       "sms",
		"provider_type": "twilio",
		"config":        map[string]any{"account_sid": "AC123", "auth_token": "tok"},
		"from_address":  "+1234567890",
		"enabled":       true,
	})

	status, body = apiReq(t, "GET", "/v1/notification-configs", apiKey, nil)
	configs = body["notification_configs"].([]any)
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}

	// Delete email config.
	status, _ = apiReq(t, "DELETE", "/v1/notification-configs/email", apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete: status %d, want 204", status)
	}

	// Verify deletion.
	status, _ = apiReq(t, "GET", "/v1/notification-configs/email", apiKey, nil)
	if status != http.StatusNotFound {
		t.Fatalf("get deleted: status %d, want 404", status)
	}
}

func TestNotificationConfigValidation(t *testing.T) {
	_, apiKey := createProviderAPI(t, "nc-val-"+uuid.NewString()[:8])

	// Invalid channel.
	status, _ := apiReq(t, "PUT", "/v1/notification-configs", apiKey, map[string]any{
		"channel":       "fax",
		"provider_type": "smtp",
		"config":        map[string]any{"host": "smtp.example.com"},
		"from_address":  "noreply@example.com",
		"enabled":       true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid channel: status %d, want 400", status)
	}

	// Missing provider_type.
	status, _ = apiReq(t, "PUT", "/v1/notification-configs", apiKey, map[string]any{
		"channel":      "email",
		"config":       map[string]any{"host": "smtp.example.com"},
		"from_address": "noreply@example.com",
		"enabled":      true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("missing provider_type: status %d, want 400", status)
	}

	// Empty config.
	status, _ = apiReq(t, "PUT", "/v1/notification-configs", apiKey, map[string]any{
		"channel":       "email",
		"provider_type": "smtp",
		"config":        map[string]any{},
		"from_address":  "noreply@example.com",
		"enabled":       true,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("empty config: status %d, want 400", status)
	}
}

func TestNotificationConfigCrossTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "nc-iso-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "nc-iso-b-"+uuid.NewString()[:8])

	// Provider A sets a config.
	apiReq(t, "PUT", "/v1/notification-configs", keyA, map[string]any{
		"channel":       "email",
		"provider_type": "smtp",
		"config":        map[string]any{"host": "smtp.a.com"},
		"from_address":  "noreply@a.com",
		"enabled":       true,
	})

	// Provider B cannot see A's config.
	status, body := apiReq(t, "GET", "/v1/notification-configs", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("B list: status %d", status)
	}
	configs := body["notification_configs"].([]any)
	if len(configs) != 0 {
		t.Fatalf("B: expected 0 configs, got %d (RLS leak)", len(configs))
	}
}
