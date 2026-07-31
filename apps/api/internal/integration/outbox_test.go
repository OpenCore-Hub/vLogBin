package integration

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/billing"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/outbox"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestOutboxRelayPublishesPending(t *testing.T) {
	// Pre-drain accumulated events from prior tests to prevent the relay
	// from spending all its time processing stale events.
	preRelay := outbox.NewRelay(appStore, billing.NewNoop(nil), 50*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for i := 0; i < 10; i++ {
		_ = preRelay.DrainOnce(testCtx)
		time.Sleep(100 * time.Millisecond)
	}

	a := createProvider(t, "relay")
	tc := tenantOf(t, a.Provider.ID, a.Environments[0].ID)

	relay := outbox.NewRelay(appStore, billing.NewNoop(nil), 50*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// All outbox events of this tenant are now published.
	withTenantTx(t, tc, func(tx pgx.Tx) {
		if n := countWhere(t, tx, `SELECT count(*) FROM outbox_events WHERE status != 'published'`); n != 0 {
			t.Fatalf("%d outbox events not published after relay drain", n)
		}
		if n := countWhere(t, tx, `SELECT count(*) FROM outbox_events WHERE status = 'published' AND published_at IS NOT NULL`); n == 0 {
			t.Fatal("expected published events with published_at set")
		}
	})

	// Run(ctx) shuts down gracefully on cancellation.
	ctx, cancel := context.WithCancel(testCtx)
	done := make(chan error, 1)
	go func() { done <- relay.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("relay Run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("relay did not shut down after context cancel")
	}
}

// TestWithTenantNeverSetsOperator proves the tenant helper cannot escalate:
// inside a tenant transaction, app.is_operator stays off.
func TestWithTenantNeverSetsOperator(t *testing.T) {
	a := createProvider(t, "no-esc")
	tc := tenantOf(t, a.Provider.ID, a.Environments[0].ID)
	err := appStore.WithTenant(testCtx, tc, func(tx pgx.Tx, q *store.Queries) error {
		var setting string
		if err := tx.QueryRow(testCtx, `SELECT coalesce(current_setting('app.is_operator', true), 'off')`).Scan(&setting); err != nil {
			return err
		}
		if setting == "on" {
			t.Fatal("tenant transaction must never enable the operator bypass")
		}
		// And the tenant context actually matches what was set.
		var pid string
		if err := tx.QueryRow(testCtx, `SELECT current_setting('app.provider_id', true)`).Scan(&pid); err != nil {
			return err
		}
		if pid != tc.ProviderID.String() {
			t.Fatalf("app.provider_id = %q, want %q", pid, tc.ProviderID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithTenant: %v", err)
	}
}
