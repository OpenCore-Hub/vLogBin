package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
)

// countAuditAction returns the number of audit rows for a provider with a
// given action, straight from the database (superuser pool bypasses RLS).
func countAuditAction(t *testing.T, providerID uuid.UUID, action string) int64 {
	t.Helper()
	var n int64
	if err := superPool.QueryRow(testCtx,
		`SELECT count(*) FROM audit_events WHERE provider_id = $1 AND action = $2`,
		providerID, action).Scan(&n); err != nil {
		t.Fatalf("count audit events for %s: %v", action, err)
	}
	return n
}

// TestPurgeAuditEventsRetainsFresh verifies the retention sweeper deletes
// events past the cutoff while leaving events inside the window intact.
// The cutoff is two years back so the fixed past timestamps used by the
// audit_stats/audit_export tests (which may run after this file) can never be
// swept, keeping the suites order-independent.
func TestPurgeAuditEventsRetainsFresh(t *testing.T) {
	providerID, _ := createProviderAPI(t, "aud-ret-"+uuid.NewString()[:8])
	pid := uuid.MustParse(providerID)
	now := time.Now().UTC()
	injectAuditEventAt(t, pid, now.Add(-3*365*24*time.Hour), "credential", "svc-a", "qa.retention.expired")
	injectAuditEventAt(t, pid, now, "credential", "svc-a", "qa.retention.fresh")

	total, err := svc.PurgeExpiredAuditEvents(testCtx, now.Add(-2*365*24*time.Hour), 1000)
	if err != nil {
		t.Fatalf("PurgeExpiredAuditEvents: %v", err)
	}
	if total < 1 {
		t.Fatalf("purged %d events, want at least the expired one", total)
	}
	if got := countAuditAction(t, pid, "qa.retention.expired"); got != 0 {
		t.Fatalf("expired events remaining = %d, want 0", got)
	}
	if got := countAuditAction(t, pid, "qa.retention.fresh"); got != 1 {
		t.Fatalf("fresh events remaining = %d, want 1", got)
	}
}

// TestPurgeAuditEventsBatched verifies the sweeper walks a backlog larger than
// one batch in multiple short transactions and deletes every row.
func TestPurgeAuditEventsBatched(t *testing.T) {
	providerID, _ := createProviderAPI(t, "aud-bat-"+uuid.NewString()[:8])
	pid := uuid.MustParse(providerID)
	base := time.Now().UTC().Add(-3 * 365 * 24 * time.Hour)
	for i := range 5 {
		injectAuditEventAt(t, pid, base.Add(time.Duration(i)*time.Minute), "credential", "svc-a", "qa.retention.old")
	}

	total, err := svc.PurgeExpiredAuditEvents(testCtx, time.Now().UTC().Add(-2*365*24*time.Hour), 2)
	if err != nil {
		t.Fatalf("PurgeExpiredAuditEvents: %v", err)
	}
	if total < 5 {
		t.Fatalf("purged %d events, want at least the 5 injected", total)
	}
	if got := countAuditAction(t, pid, "qa.retention.old"); got != 0 {
		t.Fatalf("old events remaining = %d, want 0", got)
	}
}

// TestPurgeAuditEventsRequiresOperator verifies tenant context cannot drive
// the purge: the SECURITY DEFINER function rejects callers without
// app.is_operator = 'on', so the append-only guarantee is not weakened.
func TestPurgeAuditEventsRequiresOperator(t *testing.T) {
	providerID, _ := createProviderAPI(t, "aud-op-"+uuid.NewString()[:8])
	pid := uuid.MustParse(providerID)
	err := appStore.WithTenant(testCtx, tenantOf(t, pid, uuid.New()), func(tx pgx.Tx, qr *store.Queries) error {
		_, err := qr.PurgeExpiredAuditEvents(testCtx, storegen.PurgeExpiredAuditEventsParams{
			Cutoff:  time.Now().UTC(),
			MaxRows: 10,
		})
		return err
	})
	if err == nil {
		t.Fatal("tenant-context purge succeeded, want operator-only rejection")
	}
}

// TestAuditEventsAppendOnlyInvariant verifies the append-only trigger still
// rejects direct DELETEs even from a superuser connection: purge_audit_events
// is the only deletion path.
func TestAuditEventsAppendOnlyInvariant(t *testing.T) {
	providerID, _ := createProviderAPI(t, "aud-ro-"+uuid.NewString()[:8])
	pid := uuid.MustParse(providerID)
	injectAuditEventAt(t, pid, time.Now().UTC(), "credential", "svc-a", "qa.retention.ro")
	_, err := superPool.Exec(testCtx, `DELETE FROM audit_events WHERE provider_id = $1`, pid)
	if err == nil {
		t.Fatal("direct DELETE on audit_events succeeded, want append-only trigger rejection")
	}
}
