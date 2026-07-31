package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/crypto"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/zitadel"
	"github.com/google/uuid"
)

// TestHostedAuthLifecycle verifies the full Hosted Auth lifecycle:
// setup (creates ZITADEL project + OIDC app) → get config → disable.
func TestHostedAuthLifecycle(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "auth-"+uuid.NewString()[:8])
	_ = apiKey

	// Mock ZITADEL Management API server.
	var createdProjectID string
	mux := http.NewServeMux()

	// POST /management/v1/projects — create project.
	mux.HandleFunc("/management/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		createdProjectID = "proj-" + uuid.NewString()[:8]
		json.NewEncoder(w).Encode(map[string]any{"id": createdProjectID})
	})

	// POST /management/v1/projects/{id}/apps/oidc — create OIDC app.
	mux.HandleFunc("/management/v1/projects/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path != "" {
			json.NewEncoder(w).Encode(map[string]any{
				"appId":        "app-" + uuid.NewString()[:8],
				"clientId":     "client-" + uuid.NewString()[:8],
				"clientSecret": "secret-" + uuid.NewString()[:8],
			})
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	zitadelServer := httptest.NewServer(mux)
	defer zitadelServer.Close()

	// Create service with mock ZITADEL management client + encryptor.
	mgmtClient := zitadel.NewManagementClient(zitadelServer.URL, "test-pat")
	enc, _ := crypto.NewEncryptor("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	authSvc := service.New(appStore, baseDomain,
		service.WithCryptoEncryptor(enc),
		service.WithZITADELManagement(mgmtClient, zitadelServer.URL),
	)

	// Resolve the test environment for the tenant context.
	var envID uuid.UUID
	if err := superPool.QueryRow(testCtx,
		"SELECT id FROM environments WHERE provider_id = $1 AND kind = 'test' LIMIT 1",
		providerID).Scan(&envID); err != nil {
		t.Fatalf("resolve env: %v", err)
	}
	tc := tenant.Ctx{
		ProviderID:    uuid.MustParse(providerID),
		EnvironmentID: envID,
	}

	// 1. Setup Hosted Auth.
	cfg, err := authSvc.SetupHostedAuth(testCtx, tc, service.SetupHostedAuthInput{
		Name:         "my-app",
		RedirectURIs: []string{"https://example.com/callback"},
	})
	if err != nil {
		t.Fatalf("SetupHostedAuth: %v", err)
	}
	if cfg.Config.ZitadelProjectID != createdProjectID {
		t.Fatalf("project_id = %s, want %s", cfg.Config.ZitadelProjectID, createdProjectID)
	}
	if cfg.ClientID == "" {
		t.Fatal("client_id should not be empty")
	}
	if cfg.IssuerURL != zitadelServer.URL {
		t.Fatalf("issuer_url = %s, want %s", cfg.IssuerURL, zitadelServer.URL)
	}

	// Verify the client secret is encrypted in the database.
	var dbSecret string
	if err := superPool.QueryRow(testCtx,
		"SELECT zitadel_client_secret FROM provider_auth_configs WHERE provider_id = $1",
		providerID).Scan(&dbSecret); err != nil {
		t.Fatalf("query secret: %v", err)
	}
	if dbSecret == cfg.ClientID {
		t.Fatal("client_secret in DB must not be plaintext")
	}

	// 2. Get Hosted Auth config.
	cfg2, err := authSvc.GetHostedAuthConfig(testCtx, tc)
	if err != nil {
		t.Fatalf("GetHostedAuthConfig: %v", err)
	}
	if cfg2.Config.ZitadelClientID != cfg.Config.ZitadelClientID {
		t.Fatalf("client_id mismatch: %s vs %s", cfg2.Config.ZitadelClientID, cfg.Config.ZitadelClientID)
	}

	// 3. Disable Hosted Auth.
	if err := authSvc.DisableHostedAuth(testCtx, tc); err != nil {
		t.Fatalf("DisableHostedAuth: %v", err)
	}

	// 4. Verify config is deleted.
	_, err = authSvc.GetHostedAuthConfig(testCtx, tc)
	if err == nil {
		t.Fatal("GetHostedAuthConfig should fail after disable")
	}

}
