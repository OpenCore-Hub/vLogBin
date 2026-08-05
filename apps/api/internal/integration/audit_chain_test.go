package integration

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
)

// injectChainEvent inserts an audit event and returns its id, so chain tests
// can address exact rows (the shared injectAuditEventAt helper discards ids).
// The insert goes through the audit_events_hash trigger, which computes
// prev_hash/event_hash automatically.
func injectChainEvent(t *testing.T, providerID uuid.UUID, at time.Time, action string) int64 {
	t.Helper()
	var id int64
	if err := superPool.QueryRow(testCtx,
		`INSERT INTO audit_events (provider_id, actor_type, actor_id, action, target_type, target_id, created_at)
		 VALUES ($1, 'credential', 'svc-test', $2, 'provider', $3, $4) RETURNING id`,
		providerID, action, providerID, at).Scan(&id); err != nil {
		t.Fatalf("inject audit event %s: %v", action, err)
	}
	return id
}

// chainRow returns (prev_hash, event_hash) for an audit event, failing when the
// hash trigger did not run.
func chainRow(t *testing.T, eventID int64) (string, string) {
	t.Helper()
	var prev, hash pgtype.Text
	if err := superPool.QueryRow(testCtx,
		`SELECT prev_hash, event_hash FROM audit_events WHERE id = $1`, eventID).Scan(&prev, &hash); err != nil {
		t.Fatalf("load chain row %d: %v", eventID, err)
	}
	if !hash.Valid || hash.String == "" {
		t.Fatalf("audit event %d has no event_hash: hash trigger did not run", eventID)
	}
	prevHash := ""
	if prev.Valid {
		prevHash = prev.String
	}
	return prevHash, hash.String
}

// assertChainVerify calls the verify endpoint and asserts the reported OK
// flag, returning the decoded body for further assertions.
func assertChainVerify(t *testing.T, from, to int64, wantOK bool) map[string]any {
	t.Helper()
	path := "/v1/operator/audit/chain/verify"
	if from > 0 || to > 0 {
		path += "?"
		if from > 0 {
			path += "from=" + strconv.FormatInt(from, 10)
		}
		if to > 0 {
			if from > 0 {
				path += "&"
			}
			path += "to=" + strconv.FormatInt(to, 10)
		}
	}
	status, body := apiReq(t, http.MethodGet, path, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("verify chain status = %d, body = %v", status, body)
	}
	if ok, _ := body["ok"].(bool); ok != wantOK {
		t.Fatalf("verify ok = %v, want %v (body = %v)", ok, wantOK, body)
	}
	return body
}

// TestAuditChainHashAppended verifies the insert trigger hashes every new event
// and links it to its predecessor, and that the state endpoint agrees with the
// database tail.
func TestAuditChainHashAppended(t *testing.T) {
	providerID, _ := createProviderAPI(t, "aud-ch1-"+uuid.NewString()[:8])
	pid := uuid.MustParse(providerID)
	now := time.Now().UTC()
	e1 := injectChainEvent(t, pid, now, "qa.chain.one")
	e2 := injectChainEvent(t, pid, now.Add(time.Second), "qa.chain.two")
	e3 := injectChainEvent(t, pid, now.Add(2*time.Second), "qa.chain.three")

	_, h1 := chainRow(t, e1)
	p2, h2 := chainRow(t, e2)
	p3, _ := chainRow(t, e3)
	if p2 != h1 {
		t.Fatalf("event %d prev_hash = %q, want event %d hash %q", e2, p2, e1, h1)
	}
	if p3 != h2 {
		t.Fatalf("event %d prev_hash = %q, want event %d hash %q", e3, p3, e2, h2)
	}

	status, body := apiReq(t, http.MethodGet, "/v1/operator/audit/chain", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("chain state status = %d, body = %v", status, body)
	}
	tailID, ok := body["tail_event_id"].(float64)
	if !ok || int64(tailID) < e3 {
		t.Fatalf("tail_event_id = %v, want >= %d", body["tail_event_id"], e3)
	}
	tailHash, _ := body["tail_hash"].(string)
	if tailHash == "" {
		t.Fatalf("tail_hash missing from state: %v", body)
	}
	if _, dbTail := chainRow(t, int64(tailID)); dbTail != tailHash {
		t.Fatalf("state tail_hash %q does not match DB row %d (%q)", tailHash, int64(tailID), dbTail)
	}
}

// TestAuditChainTamperDetection verifies that modifying a row behind the
// append-only trigger's back is caught: the stored event_hash no longer matches
// the recomputed hash, and verify reports the exact broken row.
func TestAuditChainTamperDetection(t *testing.T) {
	providerID, _ := createProviderAPI(t, "aud-ch2-"+uuid.NewString()[:8])
	pid := uuid.MustParse(providerID)
	now := time.Now().UTC()
	e1 := injectChainEvent(t, pid, now, "qa.chain.base")
	e2 := injectChainEvent(t, pid, now.Add(time.Second), "qa.chain.target")

	// Simulate an attacker with direct table access: disable the append-only
	// trigger, rewrite the action, and re-enable it. The event_hash column is
	// left stale, so the chain must flag the row.
	if _, err := superPool.Exec(testCtx,
		`ALTER TABLE audit_events DISABLE TRIGGER audit_events_append_only`); err != nil {
		t.Fatalf("disable append-only trigger: %v", err)
	}
	if _, err := superPool.Exec(testCtx,
		`UPDATE audit_events SET action = 'qa.chain.tampered' WHERE id = $1`, e2); err != nil {
		t.Fatalf("tamper with event %d: %v", e2, err)
	}
	if _, err := superPool.Exec(testCtx,
		`ALTER TABLE audit_events ENABLE TRIGGER audit_events_append_only`); err != nil {
		t.Fatalf("re-enable append-only trigger: %v", err)
	}
	// Keep the row restored even if an assertion below fails, so later tests
	// that verify the whole chain are unaffected.
	defer restoreAuditEvent(t, e2, "qa.chain.target")

	body := assertChainVerify(t, e1, 0, false)
	if broken, ok := body["broken_at"].(float64); !ok || int64(broken) != e2 {
		t.Fatalf("broken_at = %v, want %d (body = %v)", body["broken_at"], e2, body)
	}
	if reason, _ := body["reason"].(string); !strings.Contains(reason, "mismatch") {
		t.Fatalf("reason = %q, want a hash/link mismatch detail", reason)
	}

	// Restoring the original action heals the chain. This must happen before
	// the verification below, not only in the deferred cleanup.
	restoreAuditEvent(t, e2, "qa.chain.target")
	assertChainVerify(t, e1, 0, true)
}

// restoreAuditEvent rewrites an audit row's action behind the append-only
// trigger (same technique as the tamper above) to repair a test fixture.
func restoreAuditEvent(t *testing.T, eventID int64, action string) {
	t.Helper()
	if _, err := superPool.Exec(testCtx,
		`ALTER TABLE audit_events DISABLE TRIGGER audit_events_append_only`); err != nil {
		t.Fatalf("disable append-only trigger: %v", err)
	}
	if _, err := superPool.Exec(testCtx,
		`UPDATE audit_events SET action = $2 WHERE id = $1`, eventID, action); err != nil {
		t.Fatalf("restore event %d: %v", eventID, err)
	}
	if _, err := superPool.Exec(testCtx,
		`ALTER TABLE audit_events ENABLE TRIGGER audit_events_append_only`); err != nil {
		t.Fatalf("re-enable append-only trigger: %v", err)
	}
}

// TestAuditChainSurvivesRetentionPurge verifies the retention sweeper moves the
// pruned head forward and that the remaining chain still verifies end to end:
// verification re-anchors at the first surviving row instead of a deleted one.
func TestAuditChainSurvivesRetentionPurge(t *testing.T) {
	providerID, _ := createProviderAPI(t, "aud-ch3-"+uuid.NewString()[:8])
	pid := uuid.MustParse(providerID)
	now := time.Now().UTC()
	old := injectChainEvent(t, pid, now.Add(-3*365*24*time.Hour), "qa.chain.expired")
	injectChainEvent(t, pid, now, "qa.chain.fresh")

	total, err := svc.PurgeExpiredAuditEvents(testCtx, now.Add(-2*365*24*time.Hour), 1000)
	if err != nil {
		t.Fatalf("PurgeExpiredAuditEvents: %v", err)
	}
	if total < 1 {
		t.Fatalf("purged %d events, want >= 1", total)
	}
	if got := countAuditAction(t, pid, "qa.chain.expired"); got != 0 {
		t.Fatalf("expired events remaining = %d, want 0", got)
	}

	var pruned int64
	if err := superPool.QueryRow(testCtx,
		`SELECT COALESCE(pruned_through_id, 0) FROM audit_chain_tail WHERE id = 1`).Scan(&pruned); err != nil {
		t.Fatalf("load chain tail: %v", err)
	}
	if pruned < old {
		t.Fatalf("pruned_through_id = %d, want >= deleted row %d", pruned, old)
	}

	assertChainVerify(t, 0, 0, true)
}

// TestAuditChainAnchorLifecycle verifies anchor creation at the tail, that the
// state endpoint reports the latest checkpoint, and that incremental
// verification from an anchored event passes; a second anchor covers exactly
// the events appended since the first.
func TestAuditChainAnchorLifecycle(t *testing.T) {
	providerID, _ := createProviderAPI(t, "aud-ch4-"+uuid.NewString()[:8])
	pid := uuid.MustParse(providerID)
	now := time.Now().UTC()
	e1 := injectChainEvent(t, pid, now, "qa.chain.anchor1")

	status, body := apiReq(t, http.MethodPost, "/v1/operator/audit/chain/anchor", operatorToken, nil)
	if status != http.StatusCreated {
		t.Fatalf("anchor status = %d, body = %v", status, body)
	}
	anchorID, _ := body["anchor_id"].(float64)
	firstTail, _ := body["tail_event_id"].(float64)
	firstHash, _ := body["tail_hash"].(string)
	if int64(anchorID) <= 0 || int64(firstTail) < e1 || firstHash == "" {
		t.Fatalf("unexpected first anchor: %v", body)
	}

	injectChainEvent(t, pid, now.Add(time.Second), "qa.chain.anchor2")

	status, state := apiReq(t, http.MethodGet, "/v1/operator/audit/chain", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("chain state status = %d, body = %v", status, state)
	}
	if last, _ := state["last_anchor_id"].(float64); int64(last) != int64(anchorID) {
		t.Fatalf("last_anchor_id = %v, want %v", state["last_anchor_id"], anchorID)
	}
	if le, _ := state["last_anchor_event_id"].(float64); int64(le) != int64(firstTail) {
		t.Fatalf("last_anchor_event_id = %v, want %v", state["last_anchor_event_id"], firstTail)
	}
	if lh, _ := state["last_anchor_hash"].(string); lh != firstHash {
		t.Fatalf("last_anchor_hash = %q, want %q", lh, firstHash)
	}

	// Incremental verification anchored at the first checkpoint passes even
	// though events before it may have been purged by earlier tests.
	assertChainVerify(t, int64(firstTail), 0, true)

	status, body = apiReq(t, http.MethodPost, "/v1/operator/audit/chain/anchor", operatorToken, nil)
	if status != http.StatusCreated {
		t.Fatalf("second anchor status = %d, body = %v", status, body)
	}
	secondTail, _ := body["tail_event_id"].(float64)
	covered, _ := body["events_covered"].(float64)
	if want := int64(secondTail) - int64(firstTail); int64(covered) != want {
		t.Fatalf("events_covered = %v, want %d", covered, want)
	}
}

// TestAuditChainOperatorOnly verifies both the SQL surface and the HTTP surface
// reject non-operator callers: tenant contexts cannot run verify/anchor, and
// provider API keys (or no token) are refused on operator routes.
func TestAuditChainOperatorOnly(t *testing.T) {
	providerID, _ := createProviderAPI(t, "aud-ch5-"+uuid.NewString()[:8])
	pid := uuid.MustParse(providerID)

	err := appStore.WithTenant(testCtx, tenantOf(t, pid, uuid.New()), func(tx pgx.Tx, qr *store.Queries) error {
		_, err := qr.VerifyAuditChain(testCtx, storegen.VerifyAuditChainParams{FromID: 0, ToID: 0})
		return err
	})
	if err == nil {
		t.Fatal("tenant-context VerifyAuditChain succeeded, want operator-only rejection")
	}

	err = appStore.WithTenant(testCtx, tenantOf(t, pid, uuid.New()), func(tx pgx.Tx, qr *store.Queries) error {
		_, err := qr.AnchorAuditChain(testCtx, "tenant")
		return err
	})
	if err == nil {
		t.Fatal("tenant-context AnchorAuditChain succeeded, want operator-only rejection")
	}

	if status, _ := apiReq(t, http.MethodGet, "/v1/operator/audit/chain", "", nil); status != http.StatusUnauthorized {
		t.Fatalf("anonymous chain state status = %d, want 401", status)
	}
}
