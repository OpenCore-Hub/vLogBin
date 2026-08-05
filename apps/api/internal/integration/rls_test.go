package integration

import (
	"testing"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// withTenantTx runs fn as platform_app with the given tenant settings,
// using raw SQL to prove the isolation holds below the repository layer.
func withTenantTx(t *testing.T, tc tenant.Ctx, fn func(tx pgx.Tx)) {
	t.Helper()
	err := appStore.WithTenant(testCtx, tc, func(tx pgx.Tx, q *store.Queries) error {
		fn(tx)
		return nil
	})
	if err != nil {
		t.Fatalf("WithTenant: %v", err)
	}
}

func countWhere(t *testing.T, tx pgx.Tx, query string, args ...any) int {
	t.Helper()
	var n int
	if err := tx.QueryRow(testCtx, query, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

func TestCrossTenantIsolation(t *testing.T) {
	a := createProvider(t, "rls-a")
	b := createProvider(t, "rls-b")
	tcA := tenantOf(t, a.Provider.ID, a.Environments[0].ID)

	withTenantTx(t, tcA, func(tx pgx.Tx) {
		// Provider A sees exactly one provider row: its own.
		if n := countWhere(t, tx, `SELECT count(*) FROM providers`); n != 1 {
			t.Errorf("tenant A sees %d provider rows, want 1", n)
		}
		if n := countWhere(t, tx, `SELECT count(*) FROM providers WHERE id = $1`, b.Provider.ID); n != 0 {
			t.Errorf("tenant A can see provider B row")
		}
		// Tenant A sees only its own credentials.
		if n := countWhere(t, tx, `SELECT count(*) FROM credentials WHERE provider_id = $1`, b.Provider.ID); n != 0 {
			t.Errorf("tenant A can see provider B credentials")
		}
		if n := countWhere(t, tx, `SELECT count(*) FROM credentials`); n == 0 {
			t.Errorf("tenant A should see its own credentials")
		}
		// UPDATE against provider B matches zero rows.
		tag, err := tx.Exec(testCtx, `UPDATE providers SET name = 'pwned' WHERE id = $1`, b.Provider.ID)
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		if tag.RowsAffected() != 0 {
			t.Errorf("tenant A updated provider B row")
		}
	})

	// INSERT as tenant A with provider B's id must be rejected by the policy.
	err := appStore.WithTenant(testCtx, tcA, func(tx pgx.Tx, q *store.Queries) error {
		_, err := tx.Exec(testCtx,
			`INSERT INTO credentials (provider_id, environment_id, name, key_prefix, key_hash)
			 VALUES ($1, $2, 'evil', 'pk_test_evil', $3)`,
			b.Provider.ID, b.Environments[0].ID, uuid.NewString())
		if err == nil {
			t.Fatalf("cross-tenant INSERT must fail")
		}
		return err
	})
	if err == nil {
		t.Fatal("expected RLS violation error")
	}
}

func TestEnvironmentIsolation(t *testing.T) {
	a := createProvider(t, "env-iso")
	testEnv := a.Environments[0]

	if _, err := svc.TransitionLifecycle(testCtx, a.Provider.ID, service.LifecycleTransitionInput{To: domain.StateLiveReview}); err != nil {
		t.Fatalf("to LIVE_REVIEW: %v", err)
	}
	// Go-live gate (architecture §15): an approved risk review is required
	// before LIVE_REVIEW → LIVE_ACTIVE.
	if _, err := svc.SubmitRiskReview(testCtx, a.Provider.ID, service.RiskReviewInput{
		RiskScore: 20,
		Checks: map[string]bool{
			"email_and_company_domain": true,
			"tos_dpa":                  true,
			"custom_domain_ownership":  true,
			"payment_tax_connection":   true,
			"webhook_destination":      true,
			"initial_quota":            true,
			"security_contact":         true,
		},
		Decision:   domain.RiskDecisionApproved,
		Reason:     "go-live checklist verified",
		ReviewedBy: "op-test",
	}); err != nil {
		t.Fatalf("submit approved risk review: %v", err)
	}
	res, err := svc.TransitionLifecycle(testCtx, a.Provider.ID, service.LifecycleTransitionInput{To: domain.StateLiveActive})
	if err != nil {
		t.Fatalf("to LIVE_ACTIVE: %v", err)
	}
	if res.LiveAPIKey == "" {
		t.Fatal("live activation must return the initial live key")
	}

	detail, err := svc.GetProvider(testCtx, a.Provider.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	var liveEnvID uuid.UUID
	for _, e := range detail.Environments {
		if e.Kind == domain.EnvKindLive {
			liveEnvID = e.ID
		}
	}
	if liveEnvID == uuid.Nil {
		t.Fatal("live environment not created")
	}

	// Test-env context: live credentials are invisible.
	withTenantTx(t, tenantOf(t, a.Provider.ID, testEnv.ID), func(tx pgx.Tx) {
		if n := countWhere(t, tx, `SELECT count(*) FROM credentials WHERE environment_id = $1`, liveEnvID); n != 0 {
			t.Errorf("test-env context can see live-env credentials")
		}
		if n := countWhere(t, tx, `SELECT count(*) FROM credentials WHERE environment_id = $1`, testEnv.ID); n == 0 {
			t.Errorf("test-env context should see its own credentials")
		}
	})
	// Live-env context: test credentials are invisible (strict separation).
	liveTC := tenantOf(t, a.Provider.ID, liveEnvID)
	liveTC.EnvironmentKind = domain.EnvKindLive
	withTenantTx(t, liveTC, func(tx pgx.Tx) {
		if n := countWhere(t, tx, `SELECT count(*) FROM credentials WHERE environment_id = $1`, testEnv.ID); n != 0 {
			t.Errorf("live-env context can see test-env credentials")
		}
	})
}

func TestOperatorContextReadsAll(t *testing.T) {
	createProvider(t, "op-a")
	createProvider(t, "op-b")
	err := appStore.WithOperator(testCtx, func(tx pgx.Tx, q *store.Queries) error {
		var n int
		if err := tx.QueryRow(testCtx, `SELECT count(*) FROM providers`).Scan(&n); err != nil {
			return err
		}
		if n < 2 {
			t.Errorf("operator sees %d providers, want >= 2", n)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithOperator: %v", err)
	}
}

func TestNoTenantContextSeesNothing(t *testing.T) {
	createProvider(t, "worker-iso")
	// A query without any SET LOCAL (e.g. a misconfigured worker) must see
	// zero rows: cross-tenant access is impossible by default.
	var n int
	if err := appStore.Pool().QueryRow(testCtx, `SELECT count(*) FROM providers`).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 0 {
		t.Fatalf("query without tenant context returned %d provider rows, want 0", n)
	}
}

func TestAuditAppendOnly(t *testing.T) {
	a := createProvider(t, "audit-ro")
	tc := tenantOf(t, a.Provider.ID, a.Environments[0].ID)

	// Tenant can INSERT audit rows...
	err := appStore.WithTenant(testCtx, tc, func(tx pgx.Tx, q *store.Queries) error {
		_, err := tx.Exec(testCtx,
			`INSERT INTO audit_events (provider_id, environment_id, actor_type, actor_id, action)
			 VALUES ($1, $2, 'credential', 'x', 'test.write')`, tc.ProviderID, tc.EnvironmentID)
		return err
	})
	if err != nil {
		t.Fatalf("audit INSERT: %v", err)
	}

	// ...but UPDATE and DELETE are rejected by the database.
	for _, stmt := range []string{
		`UPDATE audit_events SET action = 'tampered' WHERE provider_id = $1`,
		`DELETE FROM audit_events WHERE provider_id = $1`,
	} {
		err := appStore.WithTenant(testCtx, tc, func(tx pgx.Tx, q *store.Queries) error {
			_, err := tx.Exec(testCtx, stmt, tc.ProviderID)
			if err == nil {
				return errAssertion{stmt}
			}
			return err
		})
		if err == nil {
			t.Fatalf("%s must be rejected", stmt)
		}
	}

	// Defense in depth: even the superuser is stopped by the trigger.
	if _, err := superPool.Exec(testCtx, `UPDATE audit_events SET action = 'tampered'`); err == nil {
		t.Fatal("superuser UPDATE on audit_events must be rejected by trigger")
	}
	if _, err := superPool.Exec(testCtx, `DELETE FROM audit_events`); err == nil {
		t.Fatal("superuser DELETE on audit_events must be rejected by trigger")
	}
}

type errAssertion struct{ stmt string }

func (e errAssertion) Error() string { return "statement unexpectedly succeeded: " + e.stmt }

func TestOutboxUniquenessAndIdempotentRetry(t *testing.T) {
	a := createProvider(t, "outbox-uniq")
	tc := tenantOf(t, a.Provider.ID, a.Environments[0].ID)
	txID := uuid.NewString()

	insert := `INSERT INTO outbox_events (provider_id, environment_id, aggregate_type, aggregate_id, event_type, payload, payload_hash, transaction_id)
	           VALUES ($1, $2, 'usage', 'x', 'usage.recorded', '{}', 'h', $3)`

	// First insert succeeds.
	err := appStore.WithTenant(testCtx, tc, func(tx pgx.Tx, q *store.Queries) error {
		_, err := tx.Exec(testCtx, insert, tc.ProviderID, tc.EnvironmentID, txID)
		return err
	})
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	// Second insert with the same transaction_id violates the unique constraint.
	err = appStore.WithTenant(testCtx, tc, func(tx pgx.Tx, q *store.Queries) error {
		_, err := tx.Exec(testCtx, insert, tc.ProviderID, tc.EnvironmentID, txID)
		if err == nil {
			return errAssertion{"duplicate outbox insert"}
		}
		return err
	})
	if err == nil {
		t.Fatal("duplicate transaction_id must violate uniqueness")
	}
	// An idempotent retry (ON CONFLICT DO NOTHING) inserts exactly once.
	for i := 0; i < 2; i++ {
		err = appStore.WithTenant(testCtx, tc, func(tx pgx.Tx, q *store.Queries) error {
			_, err := tx.Exec(testCtx, insert+` ON CONFLICT DO NOTHING`, tc.ProviderID, tc.EnvironmentID, txID)
			return err
		})
		if err != nil {
			t.Fatalf("retry insert: %v", err)
		}
	}
	withTenantTx(t, tc, func(tx pgx.Tx) {
		if n := countWhere(t, tx, `SELECT count(*) FROM outbox_events WHERE transaction_id = $1`, txID); n != 1 {
			t.Fatalf("outbox has %d rows for transaction_id, want 1", n)
		}
	})
}

func TestCommerceDomainIsolation(t *testing.T) {
	a := createProvider(t, "comm-a")
	b := createProvider(t, "comm-b")
	// Give provider B a provider-domain commerce account (as operator).
	err := appStore.WithOperator(testCtx, func(tx pgx.Tx, q *store.Queries) error {
		_, err := tx.Exec(testCtx,
			`INSERT INTO commerce_accounts (domain, provider_id, environment_id, display_name)
			 VALUES ('provider', $1, NULL, 'b-shop')`, b.Provider.ID)
		return err
	})
	if err != nil {
		t.Fatalf("insert provider-domain account: %v", err)
	}

	withTenantTx(t, tenantOf(t, a.Provider.ID, a.Environments[0].ID), func(tx pgx.Tx) {
		if n := countWhere(t, tx, `SELECT count(*) FROM commerce_accounts WHERE domain = 'platform'`); n != 0 {
			t.Errorf("provider A can see %d platform-domain commerce rows", n)
		}
		if n := countWhere(t, tx, `SELECT count(*) FROM commerce_accounts WHERE provider_id = $1`, b.Provider.ID); n != 0 {
			t.Errorf("provider A can see provider B commerce rows")
		}
	})
	// The operator still sees everything, including the seeded platform row.
	err = appStore.WithOperator(testCtx, func(tx pgx.Tx, q *store.Queries) error {
		var n int
		if err := tx.QueryRow(testCtx, `SELECT count(*) FROM commerce_accounts WHERE domain = 'platform'`).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			t.Errorf("operator must see platform-domain commerce rows")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("operator commerce query: %v", err)
	}
}

func TestIssuerStableAcrossCellReassignment(t *testing.T) {
	a := createProvider(t, "issuer")
	issuerBefore := a.Environments[0].Issuer

	// Simulate a Cell migration: create a dedicated cell and reassign the
	// provider to it (superuser bypasses RLS, as a migration would).
	var regionID uuid.UUID
	if err := superPool.QueryRow(testCtx, `SELECT home_region_id FROM providers WHERE id = $1`, a.Provider.ID).Scan(&regionID); err != nil {
		t.Fatalf("region lookup: %v", err)
	}
	var newCellID uuid.UUID
	if err := superPool.QueryRow(testCtx,
		`INSERT INTO cells (region_id, code, cell_type) VALUES ($1, $2, 'dedicated') RETURNING id`,
		regionID, "dedicated-"+uuid.NewString()[:8]).Scan(&newCellID); err != nil {
		t.Fatalf("create cell: %v", err)
	}
	// Cell reassignment is an operator action: the operator-guard trigger
	// requires app.is_operator = 'on' even for the superuser.
	tx, err := superPool.Begin(testCtx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(testCtx) //nolint:errcheck // best-effort cleanup
	if _, err := tx.Exec(testCtx, `SELECT set_config('app.is_operator', 'on', true)`); err != nil {
		t.Fatalf("set operator context: %v", err)
	}
	if _, err := tx.Exec(testCtx, `UPDATE providers SET cell_id = $1 WHERE id = $2`, newCellID, a.Provider.ID); err != nil {
		t.Fatalf("reassign cell: %v", err)
	}
	if err := tx.Commit(testCtx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	detail, err := svc.GetProvider(testCtx, a.Provider.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if detail.Provider.CellID.UUID != newCellID {
		t.Fatalf("cell reassignment not applied")
	}
	if got := detail.Environments[0].Issuer; got != issuerBefore {
		t.Fatalf("issuer changed after cell migration: %q -> %q", issuerBefore, got)
	}
}

// TestTenantCannotSelfEscalate proves the operator-guard trigger: RLS lets
// a tenant read its own providers/environments rows, but lifecycle_state,
// issuer and other registry fields are platform-governed — a tenant
// transaction must not be able to UPDATE them (spec: lifecycle transitions
// only via operator review; issuer immutable).
func TestTenantCannotSelfEscalate(t *testing.T) {
	a := createProvider(t, "esc")
	tcA := tenantOf(t, a.Provider.ID, a.Environments[0].ID)

	cases := []struct {
		name string
		stmt string
		arg  uuid.UUID
	}{
		{"lifecycle self-escalation", `UPDATE providers SET lifecycle_state = 'LIVE_ACTIVE' WHERE id = $1`, a.Provider.ID},
		{"issuer rewrite", `UPDATE environments SET issuer = 'https://evil.example' WHERE id = $1`, a.Environments[0].ID},
	}
	for _, c := range cases {
		// Propagating the exec error makes WithTenant roll back instead of
		// committing an aborted transaction.
		err := appStore.WithTenant(testCtx, tcA, func(tx pgx.Tx, q *store.Queries) error {
			_, execErr := tx.Exec(testCtx, c.stmt, c.arg)
			return execErr
		})
		if err == nil {
			t.Errorf("%s: tenant modified its own registry row", c.name)
		}
	}
}
