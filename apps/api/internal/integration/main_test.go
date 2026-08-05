// Package integration runs the Phase 0 acceptance tests against a real
// PostgreSQL 16 (testcontainers): RLS isolation, tenant context, outbox,
// commerce boundaries and the HTTP API.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/billing"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/crypto"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/httpapi"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/webhook"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/zitadel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func pathHasSuffix(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}

const (
	operatorToken = "test-operator-token"
	baseDomain    = "platform.local"
	regionCode    = "cn-shanghai"
)

var (
	testCtx    = context.Background()
	superPool  *pgxpool.Pool // superuser connection (bypasses RLS)
	appStore   *store.Store  // connects as platform_app (RLS enforced)
	svc        *service.Service
	httpServer *httptest.Server
	// testDNSRecords stores TXT records for the test DNS resolver.
	testDNSRecords = make(map[string]string)
	testDNSMu      sync.RWMutex
)

// testDNSResolver simulates DNS TXT lookups for custom domain verification.
// Tests add entries to testDNSRecords before calling the verify endpoint.
// This is the same injection pattern as WithURLValidator for webhooks.
func testDNSResolver(_ context.Context, name string) ([]string, error) {
	testDNSMu.RLock()
	defer testDNSMu.RUnlock()
	if val, ok := testDNSRecords[name]; ok {
		return []string{val}, nil
	}
	return nil, fmt.Errorf("no TXT records found for %s", name)
}

func TestMain(m *testing.M) {
	ctr, err := tcpostgres.Run(testCtx, "postgres:16-alpine",
		tcpostgres.WithDatabase("platform"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2)),
	)
	if err != nil {
		log.Fatalf("start postgres container: %v", err)
	}
	defer func() { _ = ctr.Terminate(testCtx) }()

	superURL, err := ctr.ConnectionString(testCtx, "sslmode=disable")
	if err != nil {
		log.Fatalf("connection string: %v", err)
	}

	superPool, err = pgxpool.New(testCtx, superURL)
	if err != nil {
		log.Fatalf("superuser pool: %v", err)
	}
	defer superPool.Close()

	// The app role is created out of band (compose init script in dev,
	// here in test setup); migrations assume it exists for GRANT/REVOKE.
	if _, err := superPool.Exec(testCtx, `
		DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'platform_app') THEN
				CREATE ROLE platform_app LOGIN PASSWORD 'platform_app_test';
			END IF;
		END $$;`); err != nil {
		log.Fatalf("create platform_app role: %v", err)
	}

	if err := store.Migrate(testCtx, superURL); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	u, err := url.Parse(superURL)
	if err != nil {
		log.Fatalf("parse url: %v", err)
	}
	u.User = url.UserPassword("platform_app", "platform_app_test")

	appStore, err = store.New(testCtx, u.String())
	if err != nil {
		log.Fatalf("app store: %v", err)
	}
	defer appStore.Close()

	testEncryptor, _ := crypto.NewEncryptor("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	// The global test server carries a ZITADEL Management mock so the
	// operator Console endpoints (OIDC Application management) can be
	// exercised over real HTTP. Existing Hosted Auth tests keep their own
	// per-test mock instances for call counting.
	zitadelMux := http.NewServeMux()
	zitadelMux.HandleFunc("/management/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "proj-" + uuid.NewString()[:8]})
	})
	zitadelMux.HandleFunc("/management/v1/projects/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			json.NewEncoder(w).Encode(map[string]any{
				"appId":        "app-" + uuid.NewString()[:8],
				"clientId":     "client-" + uuid.NewString()[:8],
				"clientSecret": "secret-" + uuid.NewString()[:8],
			})
		case http.MethodPut:
			if pathHasSuffix(r.URL.Path, "/oidc_config/secret") {
				json.NewEncoder(w).Encode(map[string]any{"clientSecret": "secret-" + uuid.NewString()[:8]})
				return
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	zitadelServer := httptest.NewServer(zitadelMux)
	defer zitadelServer.Close()
	mgmtClient := zitadel.NewManagementClient(zitadelServer.URL, "test-pat")

	svc = service.New(appStore, baseDomain,
		service.WithBillingAdapter(billing.NewNoop(nil)),
		service.WithURLValidator(webhook.ValidateURLAllowLoopback),
		service.WithCryptoEncryptor(testEncryptor),
		service.WithDNSResolver(testDNSResolver),
		service.WithZITADELManagement(mgmtClient, zitadelServer.URL),
	)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	apiServer := httpapi.NewServer(appStore, svc, operatorToken, logger)
	apiServer.SetStartupComplete() // migrations + pool + service are ready
	httpServer = httptest.NewServer(apiServer.Router())
	defer httpServer.Close()

	os.Exit(m.Run())
}

// tenantOf builds a tenant context for the given provider/environment ids.
func tenantOf(t *testing.T, providerID, envID uuid.UUID) tenant.Ctx {
	t.Helper()
	return tenant.Ctx{
		CredentialID:    uuid.New(),
		ProviderID:      providerID,
		EnvironmentID:   envID,
		EnvironmentKind: domain.EnvKindTest,
		Scopes:          domain.AllScopes,
	}
}

// createProvider creates a provider via the service (operator path) and
// returns the result with a unique slug.
func createProvider(t *testing.T, slugPrefix string) *service.CreatedProvider {
	t.Helper()
	slug := fmt.Sprintf("%s-%s", slugPrefix, uuid.NewString()[:8])
	created, err := svc.CreateProvider(testCtx, slug, slug+" name", regionCode)
	if err != nil {
		t.Fatalf("CreateProvider(%s): %v", slug, err)
	}
	return created
}
