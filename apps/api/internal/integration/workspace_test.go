package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestSignupProvisionsWorkspace(t *testing.T) {
	email := "signup-" + uuid.NewString()[:8] + "@example.com"
	status, body := apiReq(t, "POST", "/v1/signup", operatorToken, map[string]any{
		"email": email,
		"name":  "Signup Tester",
	})
	if status != http.StatusOK {
		t.Fatalf("signup: status %d, body %v", status, body)
	}
	ws := body["workspace"].(map[string]any)
	wsID, _ := ws["id"].(string)
	if wsID == "" {
		t.Fatal("workspace id must be set")
	}
	wantSlug := email[:len(email)-len("@example.com")]
	if slug, _ := ws["slug"].(string); slug != wantSlug {
		t.Fatalf("slug = %q, want %q", slug, wantSlug)
	}
	mem := body["membership"].(map[string]any)
	if mem["role"] != "provider_admin" {
		t.Fatalf("first-user role = %v, want provider_admin", mem["role"])
	}
	if mem["status"] != "active" {
		t.Fatalf("membership status = %v, want active", mem["status"])
	}

	// §2.1: signup also records the 1:1 provider (REGISTERED, no region yet).
	prov, ok := body["provider"].(map[string]any)
	if !ok {
		t.Fatal("signup response must include the 1:1 provider record")
	}
	if prov["id"] != wsID {
		t.Fatalf("provider id = %v, want workspace id %v", prov["id"], wsID)
	}
	if prov["lifecycle_state"] != "REGISTERED" {
		t.Fatalf("provider lifecycle_state = %v, want REGISTERED", prov["lifecycle_state"])
	}
	if v, set := prov["home_region_id"]; set && v != nil {
		t.Fatalf("REGISTERED provider home_region_id = %v, want null", v)
	}

	// R11 idempotency: a second signup for the same user returns the same
	// workspace instead of provisioning a new one.
	status, body = apiReq(t, "POST", "/v1/signup", operatorToken, map[string]any{
		"email": email,
		"name":  "Signup Tester",
	})
	if status != http.StatusOK {
		t.Fatalf("signup again: status %d, body %v", status, body)
	}
	ws2 := body["workspace"].(map[string]any)
	if ws2["id"] != wsID {
		t.Fatalf("second signup returned different workspace: %v vs %v", ws2["id"], wsID)
	}
}

func TestSignupSelfHealsProvider(t *testing.T) {
	email := "heal-" + uuid.NewString()[:8] + "@example.com"
	status, body := apiReq(t, "POST", "/v1/signup", operatorToken, map[string]any{
		"email": email,
	})
	if status != http.StatusOK {
		t.Fatalf("signup: status %d, body %v", status, body)
	}
	wsID := body["workspace"].(map[string]any)["id"].(string)

	// Simulate a workspace provisioned before the signup→provider link
	// existed: the 1:1 provider record is missing. DML on providers is
	// operator-governed, so delete inside an operator-context transaction.
	tx, err := superPool.Begin(testCtx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(testCtx) //nolint:errcheck // best-effort cleanup
	if _, err := tx.Exec(testCtx, `SELECT set_config('app.is_operator', 'on', true)`); err != nil {
		t.Fatalf("set operator context: %v", err)
	}
	if _, err := tx.Exec(testCtx, `DELETE FROM providers WHERE id = $1`, wsID); err != nil {
		t.Fatalf("delete provider: %v", err)
	}
	if err := tx.Commit(testCtx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Idempotent signup must converge to the desired state (workspace +
	// provider), recreating the REGISTERED record for the same workspace.
	status, body = apiReq(t, "POST", "/v1/signup", operatorToken, map[string]any{
		"email": email,
	})
	if status != http.StatusOK {
		t.Fatalf("signup again: status %d, body %v", status, body)
	}
	prov := body["provider"].(map[string]any)
	if prov["id"] != wsID {
		t.Fatalf("self-healed provider id = %v, want %v", prov["id"], wsID)
	}
	if prov["lifecycle_state"] != "REGISTERED" {
		t.Fatalf("self-healed provider lifecycle_state = %v, want REGISTERED", prov["lifecycle_state"])
	}
}

func TestSignupRequiresAuth(t *testing.T) {
	status, _ := apiReq(t, "POST", "/v1/signup", "", map[string]any{"email": "x@example.com"})
	if status != http.StatusUnauthorized {
		t.Fatalf("signup without token: status %d, want 401", status)
	}
}

func TestWorkspaceRLSIsolation(t *testing.T) {
	email := "rls-" + uuid.NewString()[:8] + "@example.com"
	status, body := apiReq(t, "POST", "/v1/signup", operatorToken, map[string]any{
		"email": email,
		"name":  "RLS Tester",
	})
	if status != http.StatusOK {
		t.Fatalf("signup: status %d, body %v", status, body)
	}
	ws := body["workspace"].(map[string]any)
	wsID, _ := ws["id"].(string)
	sub := body["membership"].(map[string]any)["user_sub"].(string)

	// Data is durably stored (the superuser pool bypasses RLS).
	var cnt int
	if err := superPool.QueryRow(testCtx,
		`SELECT count(*) FROM workspaces WHERE id = $1`, wsID).Scan(&cnt); err != nil {
		t.Fatalf("query workspaces: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("workspace rows = %d, want 1", cnt)
	}
	var role string
	if err := superPool.QueryRow(testCtx,
		`SELECT role FROM workspace_members WHERE workspace_id = $1 AND user_sub = $2`,
		wsID, sub).Scan(&role); err != nil {
		t.Fatalf("query membership: %v", err)
	}
	if role != "provider_admin" {
		t.Fatalf("role = %q, want provider_admin", role)
	}

	// The 1:1 provider record is durable, REGISTERED and region-less.
	var lc string
	var regionNull bool
	if err := superPool.QueryRow(testCtx,
		`SELECT lifecycle_state, home_region_id IS NULL FROM providers WHERE id = $1`,
		wsID).Scan(&lc, &regionNull); err != nil {
		t.Fatalf("query provider: %v", err)
	}
	if lc != "REGISTERED" {
		t.Fatalf("provider lifecycle_state = %q, want REGISTERED", lc)
	}
	if !regionNull {
		t.Fatalf("REGISTERED provider must have no home_region_id")
	}

	// A non-operator platform_app connection (RLS enforced via FORCE) must
	// never see control-plane rows: policies require app.is_operator='on'.
	appPool := appStore.Pool()
	var visible int
	if err := appPool.QueryRow(testCtx, `SELECT count(*) FROM workspaces`).Scan(&visible); err != nil {
		t.Fatalf("query workspaces as app: %v", err)
	}
	if visible != 0 {
		t.Fatalf("workspaces visible outside operator context = %d, want 0", visible)
	}
	var memVisible int
	if err := appPool.QueryRow(testCtx, `SELECT count(*) FROM workspace_members`).Scan(&memVisible); err != nil {
		t.Fatalf("query workspace_members as app: %v", err)
	}
	if memVisible != 0 {
		t.Fatalf("workspace_members visible outside operator context = %d, want 0", memVisible)
	}
}
