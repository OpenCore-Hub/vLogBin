// Activation tests: signup creates a REGISTERED provider (no region, no cell,
// no test environment); the operator activation endpoint moves it to
// TEST_ACTIVE, assigning the home region and shared cell and provisioning the
// test environment with its initial API key (design baseline §2.1).
//
// The tests insert their own REGISTERED providers via the superuser pool
// instead of going through /v1/signup: in legacy static-token mode every
// signup maps to the same operator identity and therefore to the same
// workspace, which would make these tests order-dependent.
package integration

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func insertRegisteredProvider(t *testing.T) string {
	t.Helper()
	var id string
	if err := superPool.QueryRow(testCtx,
		`INSERT INTO providers (slug, name, lifecycle_state) VALUES ($1, $2, 'REGISTERED') RETURNING id`,
		"reg-"+uuid.NewString()[:8], "Registered Provider").Scan(&id); err != nil {
		t.Fatalf("insert registered provider: %v", err)
	}
	return id
}

func errorCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

// TestActivateProvider covers the full activation lifecycle on one provider.
// Sub-tests share it and run in order: reject transition, activate,
// re-activation conflict.
func TestActivateProvider(t *testing.T) {
	providerID := insertRegisteredProvider(t)

	// The generic lifecycle endpoint must reject REGISTERED → TEST_ACTIVE:
	// that path is exclusively the activation flow's (it assigns region/cell
	// and provisions the test environment).
	t.Run("lifecycle rejects registered to test active", func(t *testing.T) {
		status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle",
			operatorToken, map[string]any{"to": "TEST_ACTIVE"})
		if status != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body %v", status, body)
		}
		if code := errorCode(body); code != "invalid_transition" {
			t.Fatalf("code = %q, want invalid_transition", code)
		}
	})

	t.Run("activate assigns region and provisions test env", func(t *testing.T) {
		status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/activate",
			operatorToken, map[string]any{"home_region_code": regionCode})
		if status != http.StatusOK {
			t.Fatalf("activate: status %d, body %v", status, body)
		}
		prov := body["provider"].(map[string]any)
		if prov["lifecycle_state"] != "TEST_ACTIVE" {
			t.Fatalf("lifecycle_state = %v, want TEST_ACTIVE", prov["lifecycle_state"])
		}
		if region, _ := prov["home_region_id"].(string); region == "" {
			t.Fatal("activation must assign home_region_id")
		}
		env, ok := body["test_environment"].(map[string]any)
		if !ok {
			t.Fatal("activate response must include test_environment")
		}
		if env["kind"] != "test" {
			t.Fatalf("test_environment kind = %v, want test", env["kind"])
		}
		if key, _ := body["api_key"].(string); key == "" {
			t.Fatal("activate response must include the initial test api_key")
		}

		// Durable state, checked via the superuser connection (bypasses RLS).
		var lc string
		var regionID uuid.UUID
		if err := superPool.QueryRow(testCtx,
			`SELECT lifecycle_state, home_region_id FROM providers WHERE id = $1`,
			providerID).Scan(&lc, &regionID); err != nil {
			t.Fatalf("query provider: %v", err)
		}
		if lc != "TEST_ACTIVE" {
			t.Fatalf("persisted lifecycle_state = %q, want TEST_ACTIVE", lc)
		}
		if regionID == uuid.Nil {
			t.Fatal("persisted home_region_id must be assigned")
		}

		var envCount, keyCount, acctCount, outboxCount, auditCount int
		if err := superPool.QueryRow(testCtx,
			`SELECT count(*) FROM environments WHERE provider_id = $1 AND kind = 'test'`,
			providerID).Scan(&envCount); err != nil {
			t.Fatalf("query environments: %v", err)
		}
		if err := superPool.QueryRow(testCtx,
			`SELECT count(*) FROM credentials WHERE provider_id = $1 AND name = 'initial-test-key'`,
			providerID).Scan(&keyCount); err != nil {
			t.Fatalf("query credentials: %v", err)
		}
		if err := superPool.QueryRow(testCtx,
			`SELECT count(*) FROM commerce_accounts WHERE provider_id = $1 AND domain = 'platform'`,
			providerID).Scan(&acctCount); err != nil {
			t.Fatalf("query commerce_accounts: %v", err)
		}
		if err := superPool.QueryRow(testCtx,
			`SELECT count(*) FROM outbox_events WHERE provider_id = $1 AND event_type = 'provider.activated'`,
			providerID).Scan(&outboxCount); err != nil {
			t.Fatalf("query outbox: %v", err)
		}
		if err := superPool.QueryRow(testCtx,
			`SELECT count(*) FROM audit_events WHERE provider_id = $1 AND action = 'provider.activate'`,
			providerID).Scan(&auditCount); err != nil {
			t.Fatalf("query audit: %v", err)
		}
		if envCount != 1 {
			t.Fatalf("test environments = %d, want 1", envCount)
		}
		if keyCount != 1 {
			t.Fatalf("initial-test-key credentials = %d, want 1", keyCount)
		}
		if acctCount != 1 {
			t.Fatalf("platform commerce accounts = %d, want 1", acctCount)
		}
		if outboxCount != 1 {
			t.Fatalf("provider.activated outbox events = %d, want 1", outboxCount)
		}
		if auditCount != 1 {
			t.Fatalf("provider.activate audit events = %d, want 1", auditCount)
		}
	})

	t.Run("re-activating a TEST_ACTIVE provider conflicts", func(t *testing.T) {
		status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/activate",
			operatorToken, map[string]any{"home_region_code": regionCode})
		if status != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body %v", status, body)
		}
		if code := errorCode(body); code != "conflict" {
			t.Fatalf("code = %q, want conflict", code)
		}
	})
}

// TestActivateProviderNotFound: an unknown provider id yields 404 (region
// resolution succeeds first, then the guarded UPDATE matches no row).
func TestActivateProviderNotFound(t *testing.T) {
	status, body := apiReq(t, "POST", "/v1/operator/providers/"+uuid.NewString()+"/activate",
		operatorToken, map[string]any{"home_region_code": regionCode})
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body %v", status, body)
	}
	if code := errorCode(body); code != "not_found" {
		t.Fatalf("code = %q, want not_found", code)
	}
}

// TestActivateProviderMissingRegion: home_region_code is validated before any
// provider lookup, so an empty value yields 400 regardless of provider state.
func TestActivateProviderMissingRegion(t *testing.T) {
	status, body := apiReq(t, "POST", "/v1/operator/providers/"+uuid.NewString()+"/activate",
		operatorToken, map[string]any{"home_region_code": ""})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %v", status, body)
	}
	if code := errorCode(body); code != "invalid_request" {
		t.Fatalf("code = %q, want invalid_request", code)
	}
}

// TestOperatorAuditTrail: the operator audit endpoint returns the events
// recorded during activation (provider.activate) plus any events we inject
// with JSONB metadata, which must arrive as decoded JSON objects rather than
// base64-encoded []byte values.
func TestOperatorAuditTrail(t *testing.T) {
	providerID := insertRegisteredProvider(t)

	// Inject an event with structured metadata via the superuser pool, then
	// verify the endpoint decodes the JSONB column into a plain object.
	if _, err := superPool.Exec(testCtx,
		`INSERT INTO audit_events (provider_id, actor_type, actor_id, action, target_type, target_id, metadata)
		 VALUES ($1::uuid, 'operator', 'console', 'provider.region_assigned', 'provider', $1::uuid, $2::jsonb)`,
		providerID, `{"home_region_code":"cn-north-9","reason":"review"}`); err != nil {
		t.Fatalf("inject audit event: %v", err)
	}

	status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/activate",
		operatorToken, map[string]any{"home_region_code": regionCode})
	if status != http.StatusOK {
		t.Fatalf("activate status = %d, want 200; body %v", status, body)
	}

	status, body = apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/audit",
		operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("audit status = %d, want 200; body %v", status, body)
	}
	events, ok := body["audit_events"].([]any)
	if !ok {
		t.Fatalf("audit_events missing or not an array: %v", body)
	}
	if len(events) == 0 {
		t.Fatalf("expected audit events for activated provider, got none")
	}

	actions := map[string]bool{}
	var injectedMeta map[string]any
	for _, ev := range events {
		rec, ok := ev.(map[string]any)
		if !ok {
			t.Fatalf("event not an object: %v", ev)
		}
		action, _ := rec["action"].(string)
		actions[action] = true
		if action == "provider.region_assigned" {
			injectedMeta, _ = rec["metadata"].(map[string]any)
		}
	}
	if !actions["provider.activate"] {
		t.Errorf("missing provider.activate event; got %v", actions)
	}
	if !actions["provider.region_assigned"] {
		t.Errorf("missing injected provider.region_assigned event; got %v", actions)
	}
	if injectedMeta == nil {
		t.Fatalf("expected decoded metadata object for provider.region_assigned; events %v", body)
	}
	if injectedMeta["home_region_code"] != "cn-north-9" {
		t.Errorf("home_region_code = %v, want cn-north-9", injectedMeta["home_region_code"])
	}
	if injectedMeta["reason"] != "review" {
		t.Errorf("reason = %v, want review", injectedMeta["reason"])
	}
}

// TestLifecycleTransitionConcurrency: transitions are guarded by an optimistic
// UPDATE (WHERE lifecycle_state = observed-from), so concurrent transitions
// racing from the same source state can never silently overwrite each other.
// Whichever interleaving the database produces, exactly the winning transition
// must be reflected as the final state, and any loser must surface a
// lifecycle_conflict (409) instead of corrupting state.
func TestLifecycleTransitionConcurrency(t *testing.T) {
	providerID := insertRegisteredProvider(t)

	status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/activate",
		operatorToken, map[string]any{"home_region_code": regionCode})
	if status != http.StatusOK {
		t.Fatalf("activate status = %d, want 200; body %v", status, body)
	}
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle",
		operatorToken, map[string]any{"to": "LIVE_REVIEW"})
	if status != http.StatusOK {
		t.Fatalf("to LIVE_REVIEW status = %d, want 200; body %v", status, body)
	}
	// Go-live gate: approve the risk review so LIVE_ACTIVE is a legal target
	// of the concurrent transition race (architecture §15).
	submitApprovedRiskReview(t, providerID)

	type outcome struct {
		status int
		code   string
		target string
	}
	targets := []string{"LIVE_ACTIVE", "SUSPENDED"}
	results := make(chan outcome, len(targets))
	var wg sync.WaitGroup
	for _, target := range targets {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			st, resBody := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle",
				operatorToken, map[string]any{"to": target})
			code := ""
			if errObj, ok := resBody["error"].(map[string]any); ok {
				code, _ = errObj["code"].(string)
			}
			results <- outcome{status: st, code: code, target: target}
		}(target)
	}
	wg.Wait()
	close(results)

	wins, conflicts := 0, 0
	winningTargets := map[string]bool{}
	for r := range results {
		switch r.status {
		case http.StatusOK:
			wins++
			winningTargets[r.target] = true
		case http.StatusConflict:
			if r.code != "lifecycle_conflict" {
				t.Errorf("conflict code = %q, want lifecycle_conflict (target %s)", r.code, r.target)
			}
			conflicts++
		default:
			t.Errorf("unexpected status %d (code %q) for transition to %s", r.status, r.code, r.target)
		}
	}
	if wins == 0 {
		t.Fatalf("expected at least one transition to win; got %d wins %d conflicts", wins, conflicts)
	}

	// Whatever the interleaving, the final state must be the target of a
	// transition that actually reported success — never a mixed/stale value.
	var finalState string
	if err := superPool.QueryRow(testCtx,
		`SELECT lifecycle_state FROM providers WHERE id = $1`, providerID).Scan(&finalState); err != nil {
		t.Fatalf("query final state: %v", err)
	}
	if !winningTargets[finalState] {
		t.Errorf("final state = %s, but no successful transition targeted it (%v)", finalState, winningTargets)
	}

	// Deterministic check of the optimistic guard itself: an UPDATE built on a
	// stale source state matches no row (affected=0) and leaves state intact,
	// while the correct source state applies. The provider table is
	// operator-governed, so these statements run in an operator context.
	tx, err := superPool.Begin(testCtx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(testCtx) //nolint:errcheck // committed below
	if _, err := tx.Exec(testCtx, `SELECT set_config('app.is_operator', 'on', true)`); err != nil {
		t.Fatalf("set operator context: %v", err)
	}
	var staleTag, goodTag pgconn.CommandTag
	if tag, err := tx.Exec(testCtx,
		`UPDATE providers SET lifecycle_state = 'LIVE_ACTIVE' WHERE id = $1 AND lifecycle_state = 'OFFBOARDING'`, providerID); err != nil {
		t.Fatalf("stale update: %v", err)
	} else {
		staleTag = tag
	}
	if tag, err := tx.Exec(testCtx,
		`UPDATE providers SET lifecycle_state = 'OFFBOARDING' WHERE id = $1 AND lifecycle_state = $2`, providerID, finalState); err != nil {
		t.Fatalf("good update: %v", err)
	} else {
		goodTag = tag
	}
	if err := tx.Commit(testCtx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if n := staleTag.RowsAffected(); n != 0 {
		t.Errorf("stale update affected %d rows, want 0", n)
	}
	if n := goodTag.RowsAffected(); n != 1 {
		t.Errorf("good update affected %d rows, want 1", n)
	}
}

// TestLifecycleTransitionAuditTrail: lifecycle transitions are recorded with
// the full audit context — from/to states, the operator-supplied reason, and
// the acting identity — and the same context is mirrored into the outbox event
// so downstream consumers can trace the decision. Omitting reason/actor falls
// back to a reason-free event attributed to "operator".
func TestLifecycleTransitionAuditTrail(t *testing.T) {
	providerID := insertRegisteredProvider(t)

	status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/activate",
		operatorToken, map[string]any{"home_region_code": regionCode, "reason": "initial go-live", "actor": "op-raj"})
	if status != http.StatusOK {
		t.Fatalf("activate status = %d, want 200; body %v", status, body)
	}

	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle",
		operatorToken, map[string]any{"to": "LIVE_REVIEW", "reason": "grading pass", "actor": "op-sarah"})
	if status != http.StatusOK {
		t.Fatalf("to LIVE_REVIEW status = %d, want 200; body %v", status, body)
	}

	// A bare transition with no reason/actor must still be attributed and must
	// simply omit the reason key rather than record an empty one. The go-live
	// gate (architecture §15) is satisfied first with an approved risk review.
	submitApprovedRiskReview(t, providerID)
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle",
		operatorToken, map[string]any{"to": "LIVE_ACTIVE"})
	if status != http.StatusOK {
		t.Fatalf("to LIVE_ACTIVE status = %d, want 200; body %v", status, body)
	}

	status, body = apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/audit",
		operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("audit status = %d, want 200; body %v", status, body)
	}
	events, ok := body["audit_events"].([]any)
	if !ok {
		t.Fatalf("audit_events missing or not an array: %v", body)
	}

	activateMeta, activateActor := map[string]any(nil), ""
	reviewMeta, reviewActor := map[string]any(nil), ""
	liveMeta, liveActor := map[string]any(nil), ""
	for _, ev := range events {
		rec, ok := ev.(map[string]any)
		if !ok {
			t.Fatalf("event not an object: %v", ev)
		}
		action, _ := rec["action"].(string)
		actorID, _ := rec["actor_id"].(string)
		meta, _ := rec["metadata"].(map[string]any)
		switch action {
		case "provider.activate":
			activateMeta, activateActor = meta, actorID
		case "provider.lifecycle":
			to, _ := meta["to"].(string)
			switch to {
			case "LIVE_REVIEW":
				reviewMeta, reviewActor = meta, actorID
			case "LIVE_ACTIVE":
				liveMeta, liveActor = meta, actorID
			}
		}
	}

	if activateActor != "op-raj" {
		t.Errorf("provider.activate actor_id = %q, want op-raj", activateActor)
	}
	if activateMeta["reason"] != "initial go-live" {
		t.Errorf("provider.activate reason = %v, want initial go-live", activateMeta["reason"])
	}

	if reviewActor != "op-sarah" {
		t.Errorf("LIVE_REVIEW actor_id = %q, want op-sarah", reviewActor)
	}
	if reviewMeta["from"] != "TEST_ACTIVE" {
		t.Errorf("LIVE_REVIEW from = %v, want TEST_ACTIVE", reviewMeta["from"])
	}
	if reviewMeta["to"] != "LIVE_REVIEW" {
		t.Errorf("LIVE_REVIEW to = %v, want LIVE_REVIEW", reviewMeta["to"])
	}
	if reviewMeta["reason"] != "grading pass" {
		t.Errorf("LIVE_REVIEW reason = %v, want grading pass", reviewMeta["reason"])
	}

	if liveActor != "operator" {
		t.Errorf("LIVE_ACTIVE actor_id = %q, want operator fallback", liveActor)
	}
	if liveMeta["from"] != "LIVE_REVIEW" {
		t.Errorf("LIVE_ACTIVE from = %v, want LIVE_REVIEW", liveMeta["from"])
	}
	if liveMeta["to"] != "LIVE_ACTIVE" {
		t.Errorf("LIVE_ACTIVE to = %v, want LIVE_ACTIVE", liveMeta["to"])
	}
	if _, hasReason := liveMeta["reason"]; hasReason {
		t.Errorf("LIVE_ACTIVE should carry no reason, got %v", liveMeta["reason"])
	}

	// The outbox event must mirror the transition context for downstream
	// consumers (webhook/reconciliation workers).
	var payload string
	if err := superPool.QueryRow(testCtx,
		`SELECT payload::text FROM outbox_events
		 WHERE provider_id = $1 AND event_type = 'provider.lifecycle_changed' AND payload->>'to' = 'LIVE_REVIEW'`,
		providerID).Scan(&payload); err != nil {
		t.Fatalf("query lifecycle outbox event: %v", err)
	}
	for _, want := range []string{`"from": "TEST_ACTIVE"`, `"to": "LIVE_REVIEW"`, `"reason": "grading pass"`} {
		if !strings.Contains(payload, want) {
			t.Errorf("lifecycle outbox payload missing %s; got %s", want, payload)
		}
	}
}

// TestOperatorAuditTrailUnknownProvider: unknown providers yield 404, not an
// empty success, so console typos surface immediately.
func TestOperatorAuditTrailUnknownProvider(t *testing.T) {
	status, body := apiReq(t, "GET", "/v1/operator/providers/"+uuid.NewString()+"/audit",
		operatorToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body %v", status, body)
	}
}
