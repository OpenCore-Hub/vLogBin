package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestOperatorSettingsCustomDomainLifecycle covers the Console Settings
// security section: register → DNS verify → revoke → delete, all through the
// operator control plane with ?env= isolation.
func TestOperatorSettingsCustomDomainLifecycle(t *testing.T) {
	providerID, _ := createProviderAPI(t, "set-dom-"+uuid.NewString()[:8])
	basePath := "/v1/operator/providers/" + providerID + "/custom-domains"
	base := basePath + "?env=test"
	domain := "auth-" + uuid.NewString()[:8] + ".example.com"

	status, body := apiReq(t, "POST", base, operatorToken, map[string]any{"domain": domain})
	if status != http.StatusCreated {
		t.Fatalf("register domain: status %d, body %v", status, body)
	}
	domainID, _ := body["id"].(string)
	token, _ := body["verification_token"].(string)
	if domainID == "" || token == "" {
		t.Fatalf("register must return id + verification_token, got %v", body)
	}

	// List contains the pending domain.
	status, body = apiReq(t, "GET", base, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list domains: status %d, body %v", status, body)
	}
	domains := body["custom_domains"].([]any)
	if len(domains) != 1 || domains[0].(map[string]any)["status"] != "pending" {
		t.Fatalf("domains = %v, want 1 pending", body["custom_domains"])
	}

	// Seed the DNS TXT record and verify.
	testDNSMu.Lock()
	testDNSRecords["_vlogbin-verify."+domain] = token
	testDNSMu.Unlock()
	defer func() {
		testDNSMu.Lock()
		delete(testDNSRecords, "_vlogbin-verify."+domain)
		testDNSMu.Unlock()
	}()

	status, body = apiReq(t, "POST", basePath+"/"+domainID+"/verify?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("verify domain: status %d, body %v", status, body)
	}
	if body["status"] != "verified" {
		t.Fatalf("verify status = %v, want verified", body["status"])
	}

	// Revoke, then delete (verified domains cannot be deleted directly).
	status, body = apiReq(t, "POST", basePath+"/"+domainID+"/revoke?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("revoke domain: status %d, body %v", status, body)
	}
	if body["status"] != "revoked" {
		t.Fatalf("revoke status = %v, want revoked", body["status"])
	}
	if status, _ := apiReq(t, "DELETE", basePath+"/"+domainID+"?env=test", operatorToken, nil); status != http.StatusNoContent {
		t.Fatalf("delete domain: status %d, want 204", status)
	}
	status, body = apiReq(t, "GET", base, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list after delete: status %d, body %v", status, body)
	}
	if domains, _ := body["custom_domains"].([]any); len(domains) != 0 {
		t.Fatalf("domains after delete = %v, want 0", body["custom_domains"])
	}
}

// TestOperatorSettingsNotificationConfigLifecycle covers the Console Settings
// advanced section: save an email channel config, list it back, delete it.
func TestOperatorSettingsNotificationConfigLifecycle(t *testing.T) {
	providerID, _ := createProviderAPI(t, "set-not-"+uuid.NewString()[:8])
	basePath := "/v1/operator/providers/" + providerID + "/notification-configs"
	base := basePath + "?env=test"

	status, body := apiReq(t, "GET", base, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list notification configs: status %d, body %v", status, body)
	}
	if configs, _ := body["notification_configs"].([]any); len(configs) != 0 {
		t.Fatalf("initial configs = %v, want 0", body["notification_configs"])
	}

	status, body = apiReq(t, "PUT", base, operatorToken, map[string]any{
		"channel": "email", "provider_type": "smtp", "from_address": "noreply@example.com",
		"config":  map[string]any{"host": "smtp.example.com", "port": 465, "username": "svc", "password": "secret"},
		"enabled": true,
	})
	if status != http.StatusOK {
		t.Fatalf("set notification config: status %d, body %v", status, body)
	}
	if body["channel"] != "email" || body["enabled"] != true {
		t.Fatalf("saved config = %v", body)
	}
	cfg, _ := body["config"].(map[string]any)
	if cfg["host"] != "smtp.example.com" {
		t.Fatalf("decrypted config host = %v, want smtp.example.com", cfg["host"])
	}

	status, body = apiReq(t, "GET", base, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list after save: status %d, body %v", status, body)
	}
	configs := body["notification_configs"].([]any)
	if len(configs) != 1 || configs[0].(map[string]any)["channel"] != "email" {
		t.Fatalf("configs = %v, want 1 email", body["notification_configs"])
	}

	if status, _ := apiReq(t, "DELETE", basePath+"/email?env=test", operatorToken, nil); status != http.StatusNoContent {
		t.Fatalf("delete notification config: status %d, want 204", status)
	}
	status, body = apiReq(t, "GET", base, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list after delete: status %d, body %v", status, body)
	}
	if configs, _ := body["notification_configs"].([]any); len(configs) != 0 {
		t.Fatalf("configs after delete = %v, want 0", body["notification_configs"])
	}
}

// TestOperatorSettingsValidation covers the error contract for invalid env,
// unknown providers, and unknown domain ids.
func TestOperatorSettingsValidation(t *testing.T) {
	providerID, _ := createProviderAPI(t, "set-val-"+uuid.NewString()[:8])

	status, body := apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/custom-domains", operatorToken, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("missing env: status %d, body %v", status, body)
	}
	if status, _ := apiReq(t, "POST", "/v1/operator/providers/"+uuid.NewString()+"/custom-domains?env=test",
		operatorToken, map[string]any{"domain": "auth.example.com"}); status != http.StatusNotFound {
		t.Fatalf("unknown provider: status %d, want 404", status)
	}
	if status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/custom-domains?env=test",
		operatorToken, map[string]any{"domain": "not a domain"}); status != http.StatusBadRequest {
		t.Fatalf("invalid domain: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, "DELETE", "/v1/operator/providers/"+providerID+"/custom-domains/"+uuid.NewString()+"?env=test",
		operatorToken, nil); status != http.StatusNotFound {
		t.Fatalf("unknown domain: status %d, body %v", status, body)
	}
}
