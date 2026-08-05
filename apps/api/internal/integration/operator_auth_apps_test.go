package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestOperatorAuthAppsLifecycle verifies the Console-facing OIDC Application
// management endpoints (M2 Applications page): create → list → rotate →
// update redirect URIs → disable, all scoped by ?env=test.
func TestOperatorAuthAppsLifecycle(t *testing.T) {
	providerID, _ := createProviderAPI(t, "op-auth-"+uuid.NewString()[:8])
	base := "/v1/operator/providers/" + providerID + "/auth/zitadel"

	// 1. Create an OIDC application.
	status, body := apiReq(t, http.MethodPost, base+"/setup?env=test", operatorToken, map[string]any{
		"name":          "acme-web",
		"redirect_uris": []string{"https://acme.example.com/callback"},
	})
	if status != http.StatusCreated {
		t.Fatalf("setup: status %d, body %v", status, body)
	}
	app, ok := body["app"].(map[string]any)
	if !ok {
		t.Fatalf("setup: missing app in %v", body)
	}
	clientID, _ := app["client_id"].(string)
	if clientID == "" {
		t.Fatalf("setup: client_id missing: %v", app)
	}
	if app["name"] != "acme-web" {
		t.Fatalf("setup: name = %v, want acme-web", app["name"])
	}
	redirects, _ := app["redirect_uris"].([]any)
	if len(redirects) != 1 || redirects[0] != "https://acme.example.com/callback" {
		t.Fatalf("setup: redirect_uris = %v", app["redirect_uris"])
	}
	if _, leaked := app["client_secret"]; leaked {
		t.Fatalf("setup must not return a client_secret: %v", app)
	}

	// 2. List apps — the created application is visible with decoded URIs.
	status, body = apiReq(t, http.MethodGet, base+"/apps?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d, body %v", status, body)
	}
	apps, _ := body["apps"].([]any)
	if len(apps) != 1 {
		t.Fatalf("list: len(apps) = %d, want 1", len(apps))
	}
	first := apps[0].(map[string]any)
	if first["client_id"] != clientID {
		t.Fatalf("list: client_id mismatch %v vs %s", first["client_id"], clientID)
	}
	if _, leaked := first["client_secret"]; leaked {
		t.Fatalf("list must never expose client_secret: %v", first)
	}

	// 3. Rotate the client secret — plaintext returned exactly once.
	status, body = apiReq(t, http.MethodPost, base+"/rotate-secret?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("rotate: status %d, body %v", status, body)
	}
	rotated, _ := body["app"].(map[string]any)
	secret, _ := rotated["client_secret"].(string)
	if secret == "" {
		t.Fatalf("rotate: client_secret missing: %v", rotated)
	}

	// The secret must be stored encrypted, never as plaintext.
	var dbSecret string
	if err := superPool.QueryRow(testCtx,
		"SELECT zitadel_client_secret FROM provider_auth_configs WHERE provider_id = $1",
		providerID).Scan(&dbSecret); err != nil {
		t.Fatalf("query secret: %v", err)
	}
	if dbSecret == secret {
		t.Fatal("client_secret in DB must not be plaintext")
	}

	// 4. Update redirect URIs.
	status, body = apiReq(t, http.MethodPut, base+"/redirect-uris?env=test", operatorToken, map[string]any{
		"redirect_uris": []string{"https://acme.example.com/callback", "https://alt.acme.example.com/callback"},
	})
	if status != http.StatusOK {
		t.Fatalf("update redirects: status %d, body %v", status, body)
	}
	status, body = apiReq(t, http.MethodGet, base+"/apps?env=test", operatorToken, nil)
	apps, _ = body["apps"].([]any)
	redirects, _ = apps[0].(map[string]any)["redirect_uris"].([]any)
	if len(redirects) != 2 {
		t.Fatalf("list after update: redirect_uris = %v", apps[0])
	}

	// 5. Disable (delete) the application.
	status, _ = apiReq(t, http.MethodDelete, base+"?env=test", operatorToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("disable: status %d, want 204", status)
	}
	status, body = apiReq(t, http.MethodGet, base+"/apps?env=test", operatorToken, nil)
	apps, _ = body["apps"].([]any)
	if len(apps) != 0 {
		t.Fatalf("list after disable: len(apps) = %d, want 0", len(apps))
	}
}

// TestOperatorAuthAppsValidation verifies error contracts: invalid ids,
// missing/unknown environments, unknown providers, and empty inputs.
func TestOperatorAuthAppsValidation(t *testing.T) {
	providerID, _ := createProviderAPI(t, "op-authv-"+uuid.NewString()[:8])
	base := "/v1/operator/providers/" + providerID + "/auth/zitadel"

	if status, body := apiReq(t, http.MethodGet, base+"/apps", operatorToken, nil); status != http.StatusBadRequest {
		t.Fatalf("missing env: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, base+"/apps?env=staging", operatorToken, nil); status != http.StatusBadRequest {
		t.Fatalf("invalid env: status %d, body %v", status, body)
	}
	// createProviderAPI provisions only a test environment; live is absent.
	if status, body := apiReq(t, http.MethodGet, base+"/apps?env=live", operatorToken, nil); status != http.StatusNotFound {
		t.Fatalf("unknown environment: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, "/v1/operator/providers/not-a-uuid/auth/zitadel/apps?env=test", operatorToken, nil); status != http.StatusBadRequest {
		t.Fatalf("invalid id: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, "/v1/operator/providers/"+uuid.NewString()+"/auth/zitadel/apps?env=test", operatorToken, nil); status != http.StatusNotFound {
		t.Fatalf("unknown provider: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodPost, base+"/setup?env=test", operatorToken, map[string]any{
		"name":          "",
		"redirect_uris": []string{"https://acme.example.com/callback"},
	}); status != http.StatusBadRequest {
		t.Fatalf("empty name: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodPost, base+"/setup?env=test", operatorToken, map[string]any{
		"name": "acme-web",
	}); status != http.StatusBadRequest {
		t.Fatalf("missing redirects: status %d, body %v", status, body)
	}
}
