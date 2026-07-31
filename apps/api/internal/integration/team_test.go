package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestInviteTeamMember(t *testing.T) {
	_, apiKey := createProviderAPI(t, "tm-inv-"+uuid.NewString()[:8])

	// Invite a developer.
	status, body := apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
		"email":        "dev@example.com",
		"display_name": "Dev User",
		"role":         "developer",
	})
	if status != http.StatusCreated {
		t.Fatalf("invite: status %d, body %v", status, body)
	}
	result := body
	member := result["member"].(map[string]any)
	if member["email"] != "dev@example.com" {
		t.Fatalf("email = %v", member["email"])
	}
	if member["role"] != "developer" {
		t.Fatalf("role = %v", member["role"])
	}
	if member["status"] != "active" {
		t.Fatalf("status = %v", member["status"])
	}
	apiKeyDev := result["api_key"].(string)
	if apiKeyDev == "" {
		t.Fatal("api_key must be returned")
	}

	// The new API key should work with read and write scopes.
	status, body = apiReq(t, "GET", "/v1/whoami", apiKeyDev, nil)
	if status != http.StatusOK {
		t.Fatalf("whoami with dev key: status %d", status)
	}
	scopes := body["scopes"].([]any)
	if len(scopes) != 2 {
		t.Fatalf("expected 2 scopes (read, write), got %d", len(scopes))
	}

	// The developer key should NOT have credentials:manage scope.
	status, _ = apiReq(t, "POST", "/v1/team-members", apiKeyDev, map[string]any{
		"email":        "evil@example.com",
		"display_name": "Evil",
		"role":         "developer",
	})
	if status != http.StatusForbidden {
		t.Fatalf("dev key invite: status %d, want 403", status)
	}
}

func TestListTeamMembers(t *testing.T) {
	_, apiKey := createProviderAPI(t, "tm-lst-"+uuid.NewString()[:8])

	// Initially no team members.
	status, body := apiReq(t, "GET", "/v1/team-members", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d", status)
	}
	members := body["team_members"].([]any)
	if len(members) != 0 {
		t.Fatalf("expected 0 members, got %d", len(members))
	}

	// Invite two members.
	for _, role := range []string{"developer", "billing_admin"} {
		apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
			"email":        role + "@example.com",
			"display_name": role,
			"role":         role,
		})
	}

	status, body = apiReq(t, "GET", "/v1/team-members", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list after invite: status %d", status)
	}
	members = body["team_members"].([]any)
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
}

func TestUpdateTeamMemberRole(t *testing.T) {
	_, apiKey := createProviderAPI(t, "tm-upd-"+uuid.NewString()[:8])

	// Invite a developer.
	status, body := apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
		"email":        "dev@example.com",
		"display_name": "Dev User",
		"role":         "developer",
	})
	if status != http.StatusCreated {
		t.Fatalf("invite: status %d, body %v", status, body)
	}
	memberID := body["member"].(map[string]any)["id"].(string)
	devKey := body["api_key"].(string)

	// Verify dev key has read+write only.
	status, body = apiReq(t, "GET", "/v1/whoami", devKey, nil)
	scopes := body["scopes"].([]any)
	if len(scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %d", len(scopes))
	}

	// Promote to admin.
	status, body = apiReq(t, "PATCH", "/v1/team-members/"+memberID, apiKey, map[string]any{
		"role": "admin",
	})
	if status != http.StatusOK {
		t.Fatalf("update role: status %d, body %v", status, body)
	}
	if body["role"] != "admin" {
		t.Fatalf("role = %v, want admin", body["role"])
	}

	// The same API key should now have admin scopes (6 scopes).
	status, body = apiReq(t, "GET", "/v1/whoami", devKey, nil)
	if status != http.StatusOK {
		t.Fatalf("whoami after role change: status %d", status)
	}
	scopes = body["scopes"].([]any)
	if len(scopes) != 6 {
		t.Fatalf("expected 6 scopes after promotion, got %d", len(scopes))
	}
}

func TestSuspendTeamMember(t *testing.T) {
	_, apiKey := createProviderAPI(t, "tm-sus-"+uuid.NewString()[:8])

	// Invite a developer.
	status, body := apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
		"email":        "dev@example.com",
		"display_name": "Dev User",
		"role":         "developer",
	})
	if status != http.StatusCreated {
		t.Fatalf("invite: status %d", status)
	}
	memberID := body["member"].(map[string]any)["id"].(string)
	devKey := body["api_key"].(string)

	// Suspend the member.
	status, body = apiReq(t, "POST", "/v1/team-members/"+memberID+"/suspend", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("suspend: status %d, body %v", status, body)
	}
	if body["status"] != "suspended" {
		t.Fatalf("status = %v, want suspended", body["status"])
	}

	// The dev key should be revoked (unauthorized).
	status, _ = apiReq(t, "GET", "/v1/whoami", devKey, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("suspended key: status %d, want 401", status)
	}

	// Cannot suspend an already-suspended member.
	status, _ = apiReq(t, "POST", "/v1/team-members/"+memberID+"/suspend", apiKey, nil)
	if status != http.StatusConflict {
		t.Fatalf("double suspend: status %d, want 409", status)
	}
}

func TestReactivateTeamMember(t *testing.T) {
	_, apiKey := createProviderAPI(t, "tm-rea-"+uuid.NewString()[:8])

	// Invite and suspend.
	status, body := apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
		"email":        "dev@example.com",
		"display_name": "Dev User",
		"role":         "developer",
	})
	if status != http.StatusCreated {
		t.Fatalf("invite: status %d", status)
	}
	memberID := body["member"].(map[string]any)["id"].(string)
	oldKey := body["api_key"].(string)

	apiReq(t, "POST", "/v1/team-members/"+memberID+"/suspend", apiKey, nil)

	// Reactivate.
	status, body = apiReq(t, "POST", "/v1/team-members/"+memberID+"/reactivate", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("reactivate: status %d, body %v", status, body)
	}
	if body["member"].(map[string]any)["status"] != "active" {
		t.Fatalf("status = %v, want active", body["member"].(map[string]any)["status"])
	}
	newKey := body["api_key"].(string)
	if newKey == "" {
		t.Fatal("new api_key must be returned")
	}
	if newKey == oldKey {
		t.Fatal("new key must differ from old key")
	}

	// Old key is still revoked.
	status, _ = apiReq(t, "GET", "/v1/whoami", oldKey, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("old key after reactivation: status %d, want 401", status)
	}

	// New key works.
	status, _ = apiReq(t, "GET", "/v1/whoami", newKey, nil)
	if status != http.StatusOK {
		t.Fatalf("new key: status %d, want 200", status)
	}
}

func TestRemoveTeamMember(t *testing.T) {
	_, apiKey := createProviderAPI(t, "tm-rmv-"+uuid.NewString()[:8])

	// Invite.
	status, body := apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
		"email":        "dev@example.com",
		"display_name": "Dev User",
		"role":         "developer",
	})
	if status != http.StatusCreated {
		t.Fatalf("invite: status %d", status)
	}
	memberID := body["member"].(map[string]any)["id"].(string)
	devKey := body["api_key"].(string)

	// Remove.
	status, _ = apiReq(t, "DELETE", "/v1/team-members/"+memberID, apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("remove: status %d, want 204", status)
	}

	// Key is revoked.
	status, _ = apiReq(t, "GET", "/v1/whoami", devKey, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("removed key: status %d, want 401", status)
	}

	// Cannot remove again.
	status, _ = apiReq(t, "DELETE", "/v1/team-members/"+memberID, apiKey, nil)
	if status != http.StatusConflict {
		t.Fatalf("double remove: status %d, want 409", status)
	}
}

func TestTeamMemberCrossTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "tm-iso-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "tm-iso-b-"+uuid.NewString()[:8])

	// Provider A invites a member.
	apiReq(t, "POST", "/v1/team-members", keyA, map[string]any{
		"email":        "dev@example.com",
		"display_name": "Dev A",
		"role":         "developer",
	})

	// Provider B cannot see A's members.
	status, body := apiReq(t, "GET", "/v1/team-members", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("B list: status %d", status)
	}
	members := body["team_members"].([]any)
	if len(members) != 0 {
		t.Fatalf("B sees %d members, want 0 (RLS leak)", len(members))
	}
}

func TestTeamMemberScopeAttenuation(t *testing.T) {
	_, apiKey := createProviderAPI(t, "tm-att-"+uuid.NewString()[:8])

	// Invite a developer (read + write scopes only).
	status, body := apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
		"email":        "dev@example.com",
		"display_name": "Dev User",
		"role":         "developer",
	})
	if status != http.StatusCreated {
		t.Fatalf("invite dev: status %d", status)
	}
	devKey := body["api_key"].(string)

	// Developer tries to invite an admin — should fail (no credentials:manage scope).
	status, _ = apiReq(t, "POST", "/v1/team-members", devKey, map[string]any{
		"email":        "admin@example.com",
		"display_name": "Admin User",
		"role":         "admin",
	})
	if status != http.StatusForbidden {
		t.Fatalf("dev invite admin: status %d, want 403", status)
	}

	// Invite a billing_admin (read + write + audit:read).
	status, body = apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
		"email":        "billing@example.com",
		"display_name": "Billing User",
		"role":         "billing_admin",
	})
	if status != http.StatusCreated {
		t.Fatalf("invite billing: status %d", status)
	}
	billingKey := body["api_key"].(string)

	// Billing admin tries to invite a developer — should fail (no credentials:manage).
	status, _ = apiReq(t, "POST", "/v1/team-members", billingKey, map[string]any{
		"email":        "dev2@example.com",
		"display_name": "Dev2",
		"role":         "developer",
	})
	if status != http.StatusForbidden {
		t.Fatalf("billing invite dev: status %d, want 403", status)
	}
}

func TestTeamMemberValidationErrors(t *testing.T) {
	_, apiKey := createProviderAPI(t, "tm-val-"+uuid.NewString()[:8])

	// Missing email.
	status, _ := apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
		"display_name": "User",
		"role":         "developer",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("missing email: status %d, want 400", status)
	}

	// Missing display_name.
	status, _ = apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
		"email": "user@example.com",
		"role":  "developer",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("missing display_name: status %d, want 400", status)
	}

	// Invalid role.
	status, _ = apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
		"email":        "user@example.com",
		"display_name": "User",
		"role":         "superadmin",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid role: status %d, want 400", status)
	}
}

func TestTeamMemberDuplicateEmail(t *testing.T) {
	_, apiKey := createProviderAPI(t, "tm-dup-"+uuid.NewString()[:8])

	// Invite first.
	apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
		"email":        "dev@example.com",
		"display_name": "Dev User",
		"role":         "developer",
	})

	// Invite same email again — conflict.
	status, _ := apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
		"email":        "dev@example.com",
		"display_name": "Another Dev",
		"role":         "developer",
	})
	if status != http.StatusConflict {
		t.Fatalf("duplicate email: status %d, want 409", status)
	}
}

func TestTeamMemberSuspendReactivateFlow(t *testing.T) {
	_, apiKey := createProviderAPI(t, "tm-srf-"+uuid.NewString()[:8])

	// Invite.
	status, body := apiReq(t, "POST", "/v1/team-members", apiKey, map[string]any{
		"email":        "dev@example.com",
		"display_name": "Dev User",
		"role":         "developer",
	})
	memberID := body["member"].(map[string]any)["id"].(string)

	// Suspend.
	apiReq(t, "POST", "/v1/team-members/"+memberID+"/suspend", apiKey, nil)

	// Reactivate.
	status, body = apiReq(t, "POST", "/v1/team-members/"+memberID+"/reactivate", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("reactivate: status %d", status)
	}
	newKey := body["api_key"].(string)

	// Update role to admin (should work on reactivated member).
	status, body = apiReq(t, "PATCH", "/v1/team-members/"+memberID, apiKey, map[string]any{
		"role": "admin",
	})
	if status != http.StatusOK {
		t.Fatalf("update role after reactivation: status %d, body %v", status, body)
	}

	// New key should have admin scopes.
	status, body = apiReq(t, "GET", "/v1/whoami", newKey, nil)
	if status != http.StatusOK {
		t.Fatalf("whoami: status %d", status)
	}
	scopes := body["scopes"].([]any)
	if len(scopes) != 6 {
		t.Fatalf("expected 6 scopes after admin promotion, got %d", len(scopes))
	}

	// Remove the member.
	status, _ = apiReq(t, "DELETE", "/v1/team-members/"+memberID, apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("remove: status %d, want 204", status)
	}

	// Cannot reactivate a removed member.
	status, _ = apiReq(t, "POST", "/v1/team-members/"+memberID+"/reactivate", apiKey, nil)
	if status != http.StatusConflict {
		t.Fatalf("reactivate removed: status %d, want 409", status)
	}
}
