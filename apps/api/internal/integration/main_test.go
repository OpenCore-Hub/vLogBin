// Package integration runs the Phase 0 acceptance tests against a real
// PostgreSQL 16 (testcontainers): RLS isolation, tenant context, outbox,
// commerce boundaries and the HTTP API.
package integration

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/billing"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/crypto"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/httpapi"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/webhook"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

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
)

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
	svc = service.New(appStore, baseDomain,
		service.WithBillingAdapter(billing.NewNoop(nil)),
		service.WithURLValidator(webhook.ValidateURLAllowLoopback),
		service.WithCryptoEncryptor(testEncryptor),
	)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	httpServer = httptest.NewServer(httpapi.NewServer(appStore, svc, operatorToken, logger).Router())
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
