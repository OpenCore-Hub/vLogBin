package integration

import (
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// workspaceForOperator returns the workspace provisioned for the static
// operator identity ("operator" in legacy token mode). The first signup for
// that identity created it (idempotent); when run standalone it provisions
// one on demand. The zz_ prefix keeps this file after workspace_test.go so
// the pre-existing signup tests keep their first-provisioned workspace.
func workspaceForOperator(t *testing.T) string {
	t.Helper()
	var id string
	err := superPool.QueryRow(testCtx,
		`SELECT workspace_id FROM workspace_members WHERE user_sub = 'operator' ORDER BY created_at LIMIT 1`,
	).Scan(&id)
	if err == nil {
		return id
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("query operator workspace: %v", err)
	}
	status, body := apiReq(t, "POST", "/v1/signup", operatorToken, map[string]any{
		"email": "members-" + uuid.NewString()[:8] + "@example.com",
	})
	if status != http.StatusOK {
		t.Fatalf("signup: status %d, body %v", status, body)
	}
	ws := body["workspace"].(map[string]any)
	id, _ = ws["id"].(string)
	if id == "" {
		t.Fatal("workspace id must be set")
	}
	return id
}

func TestWorkspaceMemberInviteAndRoleManagement(t *testing.T) {
	wsID := workspaceForOperator(t)
	alice := "user-" + uuid.NewString()[:8]

	// Invite a developer.
	status, body := apiReq(t, "POST", "/v1/me/workspaces/"+wsID+"/members", operatorToken,
		map[string]any{"user_sub": alice, "role": "provider_developer"})
	if status != http.StatusCreated {
		t.Fatalf("invite: status %d, body %v", status, body)
	}
	if got := body["member"].(map[string]any)["user_sub"]; got != alice {
		t.Fatalf("member user_sub = %v, want %s", got, alice)
	}

	// Member list contains the owner and the invitee.
	status, body = apiReq(t, "GET", "/v1/me/workspaces/"+wsID+"/members", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d, body %v", status, body)
	}
	if members := body["members"].([]any); len(members) != 2 {
		t.Fatalf("member count = %d, want 2", len(members))
	}

	// Re-inviting the same user is idempotent and keeps a single membership row.
	status, body = apiReq(t, "POST", "/v1/me/workspaces/"+wsID+"/members", operatorToken,
		map[string]any{"user_sub": alice, "role": "provider_developer"})
	if status != http.StatusCreated {
		t.Fatalf("re-invite: status %d, body %v", status, body)
	}
	var cnt int
	if err := superPool.QueryRow(testCtx,
		`SELECT count(*) FROM workspace_members WHERE workspace_id = $1 AND user_sub = $2`,
		wsID, alice).Scan(&cnt); err != nil {
		t.Fatalf("count membership: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("membership rows for invitee = %d, want 1", cnt)
	}

	// Change the role.
	status, body = apiReq(t, "PATCH", "/v1/me/workspaces/"+wsID+"/members/"+alice, operatorToken,
		map[string]any{"role": "provider_billing"})
	if status != http.StatusOK {
		t.Fatalf("update role: status %d, body %v", status, body)
	}
	if got := body["member"].(map[string]any)["role"]; got != "provider_billing" {
		t.Fatalf("role = %v, want provider_billing", got)
	}

	// Re-activation: a removed member rejoins on invite (status back to active).
	if _, err := superPool.Exec(testCtx,
		`UPDATE workspace_members SET status = 'removed' WHERE workspace_id = $1 AND user_sub = $2`,
		wsID, alice); err != nil {
		t.Fatalf("mark removed: %v", err)
	}
	status, body = apiReq(t, "POST", "/v1/me/workspaces/"+wsID+"/members", operatorToken,
		map[string]any{"user_sub": alice, "role": "provider_developer"})
	if status != http.StatusCreated {
		t.Fatalf("re-invite removed member: status %d, body %v", status, body)
	}
	if got := body["member"].(map[string]any)["status"]; got != "active" {
		t.Fatalf("re-activated status = %v, want active", got)
	}

	// A suspended member cannot be re-roled.
	if _, err := superPool.Exec(testCtx,
		`UPDATE workspace_members SET status = 'suspended' WHERE workspace_id = $1 AND user_sub = $2`,
		wsID, alice); err != nil {
		t.Fatalf("mark suspended: %v", err)
	}
	status, body = apiReq(t, "PATCH", "/v1/me/workspaces/"+wsID+"/members/"+alice, operatorToken,
		map[string]any{"role": "provider_developer"})
	if status != http.StatusConflict {
		t.Fatalf("role update on suspended member: status %d, want 409 (body %v)", status, body)
	}
	if _, err := superPool.Exec(testCtx,
		`UPDATE workspace_members SET status = 'active' WHERE workspace_id = $1 AND user_sub = $2`,
		wsID, alice); err != nil {
		t.Fatalf("restore active: %v", err)
	}

	// A non-admin (API always authenticates as the owner) cannot invite.
	demoteOperator(t, wsID)
	status, body = apiReq(t, "POST", "/v1/me/workspaces/"+wsID+"/members", operatorToken,
		map[string]any{"user_sub": "user-" + uuid.NewString()[:8], "role": "provider_developer"})
	if status != http.StatusForbidden {
		t.Fatalf("non-admin invite: status %d, want 403 (body %v)", status, body)
	}
	restoreOperatorAdmin(t, wsID)

	// Remove the invitee.
	status, _ = apiReq(t, "DELETE", "/v1/me/workspaces/"+wsID+"/members/"+alice, operatorToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("remove: status %d, want 204", status)
	}
	status, body = apiReq(t, "GET", "/v1/me/workspaces/"+wsID+"/members", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list after remove: status %d, body %v", status, body)
	}
	if members := body["members"].([]any); len(members) != 1 {
		t.Fatalf("member count after remove = %d, want 1", len(members))
	}

	// Removing a non-existent member is a 404.
	status, body = apiReq(t, "DELETE", "/v1/me/workspaces/"+wsID+"/members/"+alice, operatorToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("remove missing member: status %d, want 404 (body %v)", status, body)
	}
}

func TestWorkspaceLastAdminProtection(t *testing.T) {
	wsID := workspaceForOperator(t)

	// The owner is the only active admin: demoting or removing them must fail.
	status, body := apiReq(t, "PATCH", "/v1/me/workspaces/"+wsID+"/members/operator", operatorToken,
		map[string]any{"role": "provider_developer"})
	if status != http.StatusConflict {
		t.Fatalf("demote last admin: status %d, want 409 (body %v)", status, body)
	}
	status, body = apiReq(t, "DELETE", "/v1/me/workspaces/"+wsID+"/members/operator", operatorToken, nil)
	if status != http.StatusConflict {
		t.Fatalf("remove last admin: status %d, want 409 (body %v)", status, body)
	}

	// With a second admin present, the owner can be demoted (protection does
	// not block legitimate changes).
	second := "admin-" + uuid.NewString()[:8]
	status, body = apiReq(t, "POST", "/v1/me/workspaces/"+wsID+"/members", operatorToken,
		map[string]any{"user_sub": second, "role": "provider_admin"})
	if status != http.StatusCreated {
		t.Fatalf("invite second admin: status %d, body %v", status, body)
	}
	status, body = apiReq(t, "PATCH", "/v1/me/workspaces/"+wsID+"/members/operator", operatorToken,
		map[string]any{"role": "provider_developer"})
	if status != http.StatusOK {
		t.Fatalf("demote owner with backup admin: status %d, body %v", status, body)
	}

	// Restore the owner as admin, then clean up the second admin.
	restoreOperatorAdmin(t, wsID)
	status, body = apiReq(t, "DELETE", "/v1/me/workspaces/"+wsID+"/members/"+second, operatorToken, nil)
	if status != http.StatusNoContent {
		t.Fatalf("remove second admin: status %d, body %v", status, body)
	}
}

func TestWorkspaceVisibilityAndValidation(t *testing.T) {
	wsID := workspaceForOperator(t)

	// Unknown workspace id surfaces as 404 (no existence leak).
	status, _ := apiReq(t, "GET", "/v1/me/workspaces/"+uuid.NewString(), operatorToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown workspace: status %d, want 404", status)
	}

	// Malformed workspace id is rejected.
	status, _ = apiReq(t, "GET", "/v1/me/workspaces/not-a-uuid", operatorToken, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("malformed id: status %d, want 400", status)
	}

	// Invalid role is rejected.
	status, body := apiReq(t, "POST", "/v1/me/workspaces/"+wsID+"/members", operatorToken,
		map[string]any{"user_sub": "x", "role": "super_admin"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid role: status %d, want 400 (body %v)", status, body)
	}

	// Missing member user_sub is rejected.
	status, _ = apiReq(t, "POST", "/v1/me/workspaces/"+wsID+"/members", operatorToken,
		map[string]any{"role": "provider_developer"})
	if status != http.StatusBadRequest {
		t.Fatalf("missing user_sub: status %d, want 400", status)
	}

	// Update workspace: rename only, then slug validation.
	status, body = apiReq(t, "PATCH", "/v1/me/workspaces/"+wsID, operatorToken,
		map[string]any{"name": "Renamed Workspace"})
	if status != http.StatusOK {
		t.Fatalf("rename: status %d, body %v", status, body)
	}
	if got := body["workspace"].(map[string]any)["name"]; got != "Renamed Workspace" {
		t.Fatalf("name = %v, want Renamed Workspace", got)
	}
	status, body = apiReq(t, "PATCH", "/v1/me/workspaces/"+wsID, operatorToken,
		map[string]any{"slug": "Bad Slug!"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid slug: status %d, want 400 (body %v)", status, body)
	}

	// Empty update body is rejected.
	status, _ = apiReq(t, "PATCH", "/v1/me/workspaces/"+wsID, operatorToken, map[string]any{})
	if status != http.StatusBadRequest {
		t.Fatalf("empty patch: status %d, want 400", status)
	}

	// Get workspace detail as an active member.
	status, body = apiReq(t, "GET", "/v1/me/workspaces/"+wsID, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("get workspace: status %d, body %v", status, body)
	}
	if got := body["workspace"].(map[string]any)["id"]; got != wsID {
		t.Fatalf("workspace id = %v, want %s", got, wsID)
	}
}

// demoteOperator demotes the static operator identity to developer, simulating
// a non-admin caller (the API always authenticates as "operator" in legacy
// token mode, so role changes are seeded directly through the DB).
func demoteOperator(t *testing.T, wsID string) {
	t.Helper()
	if _, err := superPool.Exec(testCtx,
		`UPDATE workspace_members SET role = 'provider_developer' WHERE workspace_id = $1 AND user_sub = 'operator'`,
		wsID); err != nil {
		t.Fatalf("demote operator: %v", err)
	}
}

// restoreOperatorAdmin restores the static operator identity to admin after a
// demoteOperator call.
func restoreOperatorAdmin(t *testing.T, wsID string) {
	t.Helper()
	if _, err := superPool.Exec(testCtx,
		`UPDATE workspace_members SET role = 'provider_admin' WHERE workspace_id = $1 AND user_sub = 'operator'`,
		wsID); err != nil {
		t.Fatalf("restore operator admin: %v", err)
	}
}
