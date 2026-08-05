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

// TestHostedAuthListRotateUpdate verifies the List / RotateSecret /
// UpdateRedirectURIs endpoints for OIDC Application management (M1
// control面 API 第 1 项).
func TestHostedAuthListRotateUpdate(t *testing.T) {
	providerID, _ := createProviderAPI(t, "authm1-"+uuid.NewString()[:8])

	var (
		createProjectCalls  int
		createOIDCAppCalls  int
		rotateSecretCalls   int
		updateRedirectCalls int
		deleteProjectCalls  int
	)

	mux := http.NewServeMux()

	mux.HandleFunc("/management/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			createProjectCalls++
			json.NewEncoder(w).Encode(map[string]any{"id": "proj-" + uuid.NewString()[:8]})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/management/v1/projects/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodPost && path != "":
			createOIDCAppCalls++
			json.NewEncoder(w).Encode(map[string]any{
				"appId":        "app-" + uuid.NewString()[:8],
				"clientId":     "client-" + uuid.NewString()[:8],
				"clientSecret": "secret-initial-" + uuid.NewString()[:8],
			})
		case r.Method == http.MethodPut && path != "":
			var body map[string]any
			bodyBytes := make([]byte, 0)
			buf := make([]byte, 1024)
			for {
				n, err := r.Body.Read(buf)
				if n > 0 {
					bodyBytes = append(bodyBytes, buf[:n]...)
				}
				if err != nil {
					break
				}
			}
			_ = json.Unmarshal(bodyBytes, &body)

			if _, ok := body["redirectUris"]; ok {
				updateRedirectCalls++
				w.WriteHeader(http.StatusOK)
				return
			}
			rotateSecretCalls++
			json.NewEncoder(w).Encode(map[string]any{
				"clientSecret": "secret-rotated-" + uuid.NewString()[:8],
			})
		case r.Method == http.MethodDelete:
			deleteProjectCalls++
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	zitadelServer := httptest.NewServer(mux)
	defer zitadelServer.Close()

	mgmtClient := zitadel.NewManagementClient(zitadelServer.URL, "test-pat")
	enc, _ := crypto.NewEncryptor("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	authSvc := service.New(appStore, baseDomain,
		service.WithCryptoEncryptor(enc),
		service.WithZITADELManagement(mgmtClient, zitadelServer.URL),
	)

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
	if createProjectCalls != 1 {
		t.Fatalf("createProjectCalls = %d, want 1", createProjectCalls)
	}
	if createOIDCAppCalls != 1 {
		t.Fatalf("createOIDCAppCalls = %d, want 1", createOIDCAppCalls)
	}

	// 2. List Hosted Auth configs — should return the one we just created.
	cfgs, err := authSvc.ListHostedAuthConfigs(testCtx, tc)
	if err != nil {
		t.Fatalf("ListHostedAuthConfigs: %v", err)
	}
	if len(cfgs) != 1 {
		t.Fatalf("len(cfgs) = %d, want 1", len(cfgs))
	}
	if cfgs[0].ZitadelClientID != cfg.Config.ZitadelClientID {
		t.Fatalf("client_id mismatch: %s vs %s", cfgs[0].ZitadelClientID, cfg.Config.ZitadelClientID)
	}

	// 3. Rotate secret — should return a new plaintext secret.
	rotated, err := authSvc.RotateHostedAuthSecret(testCtx, tc)
	if err != nil {
		t.Fatalf("RotateHostedAuthSecret: %v", err)
	}
	if rotated.ClientSecret == "" {
		t.Fatal("rotated client_secret should not be empty")
	}
	if rotateSecretCalls != 1 {
		t.Fatalf("rotateSecretCalls = %d, want 1", rotateSecretCalls)
	}

	// Verify the secret in DB is encrypted (not plaintext).
	var dbSecret string
	if err := superPool.QueryRow(testCtx,
		"SELECT zitadel_client_secret FROM provider_auth_configs WHERE provider_id = $1",
		providerID).Scan(&dbSecret); err != nil {
		t.Fatalf("query secret: %v", err)
	}
	if dbSecret == rotated.ClientSecret {
		t.Fatal("client_secret in DB must not be plaintext after rotation")
	}

	// 4. Update redirect URIs.
	updated, err := authSvc.UpdateHostedAuthRedirectURIs(testCtx, tc, service.UpdateHostedAuthRedirectURIsInput{
		RedirectURIs: []string{"https://new.example.com/callback", "https://alt.example.com/callback"},
	})
	if err != nil {
		t.Fatalf("UpdateHostedAuthRedirectURIs: %v", err)
	}
	if updateRedirectCalls != 1 {
		t.Fatalf("updateRedirectCalls = %d, want 1", updateRedirectCalls)
	}

	// Verify the redirect_uris are stored in DB.
	var dbURIs []byte
	if err := superPool.QueryRow(testCtx,
		"SELECT redirect_uris FROM provider_auth_configs WHERE provider_id = $1",
		providerID).Scan(&dbURIs); err != nil {
		t.Fatalf("query redirect_uris: %v", err)
	}
	var storedURIs []string
	if err := json.Unmarshal(dbURIs, &storedURIs); err != nil {
		t.Fatalf("unmarshal redirect_uris: %v", err)
	}
	if len(storedURIs) != 2 {
		t.Fatalf("len(storedURIs) = %d, want 2", len(storedURIs))
	}
	if storedURIs[0] != "https://new.example.com/callback" {
		t.Fatalf("storedURIs[0] = %s, want https://new.example.com/callback", storedURIs[0])
	}
	_ = updated

	// 5. Verify audit events were emitted for rotate + update.
	var auditCount int
	if err := superPool.QueryRow(testCtx,
		`SELECT count(*) FROM audit_events
		 WHERE provider_id = $1 AND action IN ('auth.hosted_auth_secret_rotate', 'auth.hosted_auth_redirect_uris_update')`,
		providerID).Scan(&auditCount); err != nil {
		t.Fatalf("query audit: %v", err)
	}
	if auditCount != 2 {
		t.Fatalf("auditCount = %d, want 2", auditCount)
	}
}

// TestHostedAuthRotateValidation verifies error cases for rotate and update.
func TestHostedAuthRotateValidation(t *testing.T) {
	providerID, _ := createProviderAPI(t, "authval-"+uuid.NewString()[:8])

	mux := http.NewServeMux()
	mux.HandleFunc("/management/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"id": "proj-" + uuid.NewString()[:8]})
	})
	mux.HandleFunc("/management/v1/projects/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			json.NewEncoder(w).Encode(map[string]any{
				"appId":        "app-" + uuid.NewString()[:8],
				"clientId":     "client-" + uuid.NewString()[:8],
				"clientSecret": "secret-" + uuid.NewString()[:8],
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	zitadelServer := httptest.NewServer(mux)
	defer zitadelServer.Close()

	mgmtClient := zitadel.NewManagementClient(zitadelServer.URL, "test-pat")
	enc, _ := crypto.NewEncryptor("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	authSvc := service.New(appStore, baseDomain,
		service.WithCryptoEncryptor(enc),
		service.WithZITADELManagement(mgmtClient, zitadelServer.URL),
	)

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

	// 1. Rotate without existing config → should fail (not found).
	_, err := authSvc.RotateHostedAuthSecret(testCtx, tc)
	if err == nil {
		t.Fatal("RotateHostedAuthSecret without config should fail")
	}

	// 2. Update redirect URIs without existing config → should fail.
	_, err = authSvc.UpdateHostedAuthRedirectURIs(testCtx, tc, service.UpdateHostedAuthRedirectURIsInput{
		RedirectURIs: []string{"https://example.com/callback"},
	})
	if err == nil {
		t.Fatal("UpdateHostedAuthRedirectURIs without config should fail")
	}

	// 3. Setup config first.
	_, err = authSvc.SetupHostedAuth(testCtx, tc, service.SetupHostedAuthInput{
		Name:         "my-app",
		RedirectURIs: []string{"https://example.com/callback"},
	})
	if err != nil {
		t.Fatalf("SetupHostedAuth: %v", err)
	}

	// 4. Update redirect URIs with empty list → should fail (validation).
	_, err = authSvc.UpdateHostedAuthRedirectURIs(testCtx, tc, service.UpdateHostedAuthRedirectURIsInput{
		RedirectURIs: []string{},
	})
	if err == nil {
		t.Fatal("UpdateHostedAuthRedirectURIs with empty list should fail")
	}
}
