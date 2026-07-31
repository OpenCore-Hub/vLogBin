package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestSCIMGroupCRUD(t *testing.T) {
	_, apiKey := createProviderAPI(t, "sg-crud-"+uuid.NewString()[:8])

	// Create group.
	status, body := apiReq(t, "POST", "/scim/v2/Groups", apiKey, map[string]any{
		"externalId":  "group-001",
		"displayName": "Engineering Team",
	})
	if status != http.StatusCreated {
		t.Fatalf("create group: status %d, body %v", status, body)
	}
	groupID := body["id"].(string)
	schemas := body["schemas"].([]any)
	if schemas[0] != "urn:ietf:params:scim:schemas:core:2.0:Group" {
		t.Fatalf("schema = %v", schemas[0])
	}

	// Get by ID.
	status, body = apiReq(t, "GET", "/scim/v2/Groups/"+groupID, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get group: status %d", status)
	}
	if body["displayName"] != "Engineering Team" {
		t.Fatalf("displayName = %v", body["displayName"])
	}

	// List groups.
	status, body = apiReq(t, "GET", "/scim/v2/Groups", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list groups: status %d", status)
	}
	if body["totalResults"].(float64) != 1 {
		t.Fatalf("totalResults = %v, want 1", body["totalResults"])
	}

	// Delete group.
	status, _ = apiReq(t, "DELETE", "/scim/v2/Groups/"+groupID, apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete group: status %d, want 204", status)
	}

	// Get after delete — 404.
	status, _ = apiReq(t, "GET", "/scim/v2/Groups/"+groupID, apiKey, nil)
	if status != http.StatusNotFound {
		t.Fatalf("get deleted group: status %d, want 404", status)
	}
}

func TestSCIMGroupIdempotency(t *testing.T) {
	_, apiKey := createProviderAPI(t, "sg-idm-"+uuid.NewString()[:8])

	// Create group.
	status, body1 := apiReq(t, "POST", "/scim/v2/Groups", apiKey, map[string]any{
		"externalId":  "group-idem",
		"displayName": "Team A",
	})
	if status != http.StatusCreated {
		t.Fatalf("create: status %d", status)
	}
	id1 := body1["id"].(string)

	// Create same group again — should return existing.
	status, body2 := apiReq(t, "POST", "/scim/v2/Groups", apiKey, map[string]any{
		"externalId":  "group-idem",
		"displayName": "Team A",
	})
	if status != http.StatusCreated {
		t.Fatalf("idempotent create: status %d", status)
	}
	if body2["id"] != id1 {
		t.Fatalf("idempotent create returned different id: %v vs %v", body2["id"], id1)
	}
}

func TestSCIMGroupCrossTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "sg-iso-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "sg-iso-b-"+uuid.NewString()[:8])

	// Provider A creates a group.
	status, body := apiReq(t, "POST", "/scim/v2/Groups", keyA, map[string]any{
		"externalId":  "iso-group",
		"displayName": "Iso Team",
	})
	groupAID := body["id"].(string)

	// Provider B cannot get A's group.
	status, _ = apiReq(t, "GET", "/scim/v2/Groups/"+groupAID, keyB, nil)
	if status != http.StatusNotFound {
		t.Fatalf("B get A's group: status %d, want 404", status)
	}

	// Provider B lists groups (should be empty).
	status, body = apiReq(t, "GET", "/scim/v2/Groups", keyB, nil)
	if body["totalResults"].(float64) != 0 {
		t.Fatalf("B: totalResults = %v, want 0 (RLS leak)", body["totalResults"])
	}
}

func TestSCIMPatchUser(t *testing.T) {
	_, apiKey := createProviderAPI(t, "sp-pat-"+uuid.NewString()[:8])

	// Create a user.
	status, body := apiReq(t, "POST", "/scim/v2/Users", apiKey, map[string]any{
		"externalId":  "patch-user",
		"displayName": "Original Name",
		"email":       "original@example.com",
		"active":      true,
	})
	if status != http.StatusCreated {
		t.Fatalf("create user: status %d", status)
	}
	userID := body["id"].(string)

	// PATCH: replace displayName and active.
	status, body = apiReq(t, "PATCH", "/scim/v2/Users/"+userID, apiKey, map[string]any{
		"Operations": []map[string]any{
			{"op": "replace", "path": "displayName", "value": "Patched Name"},
			{"op": "replace", "path": "active", "value": false},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("patch user: status %d, body %v", status, body)
	}
	if body["name"].(map[string]any)["displayName"] != "Patched Name" {
		t.Fatalf("displayName = %v, want Patched Name", body["name"].(map[string]any)["displayName"])
	}
	if body["active"] != false {
		t.Fatalf("active = %v, want false", body["active"])
	}

	// Verify with GET.
	status, body = apiReq(t, "GET", "/scim/v2/Users/"+userID, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get patched user: status %d", status)
	}
	if body["name"].(map[string]any)["displayName"] != "Patched Name" {
		t.Fatalf("displayName after GET = %v", body["name"].(map[string]any)["displayName"])
	}
	if body["active"] != false {
		t.Fatalf("active after GET = %v, want false", body["active"])
	}

	// PATCH: replace email.
	status, body = apiReq(t, "PATCH", "/scim/v2/Users/"+userID, apiKey, map[string]any{
		"Operations": []map[string]any{
			{"op": "replace", "path": "email", "value": "patched@example.com"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("patch email: status %d", status)
	}
	if body["userName"] != "patched@example.com" {
		t.Fatalf("userName = %v, want patched@example.com", body["userName"])
	}
}

func TestSCIMPatchUserNotFound(t *testing.T) {
	_, apiKey := createProviderAPI(t, "sp-nf-"+uuid.NewString()[:8])

	status, _ := apiReq(t, "PATCH", "/scim/v2/Users/"+uuid.NewString(), apiKey, map[string]any{
		"Operations": []map[string]any{
			{"op": "replace", "path": "active", "value": false},
		},
	})
	if status != http.StatusNotFound {
		t.Fatalf("patch non-existent user: status %d, want 404", status)
	}
}

func TestSCIMPatchGroupDisplayName(t *testing.T) {
	_, apiKey := createProviderAPI(t, "sg-pdn-"+uuid.NewString()[:8])

	// Create group.
	status, body := apiReq(t, "POST", "/scim/v2/Groups", apiKey, map[string]any{
		"externalId":  "patch-group",
		"displayName": "Original Group",
	})
	groupID := body["id"].(string)

	// PATCH: replace displayName.
	status, body = apiReq(t, "PATCH", "/scim/v2/Groups/"+groupID, apiKey, map[string]any{
		"Operations": []map[string]any{
			{"op": "replace", "path": "displayName", "value": "Patched Group"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("patch group: status %d, body %v", status, body)
	}
	if body["displayName"] != "Patched Group" {
		t.Fatalf("displayName = %v, want Patched Group", body["displayName"])
	}
}

func TestSCIMPatchGroupAddRemoveMembers(t *testing.T) {
	_, apiKey := createProviderAPI(t, "sg-arm-"+uuid.NewString()[:8])

	// Create a group.
	status, body := apiReq(t, "POST", "/scim/v2/Groups", apiKey, map[string]any{
		"externalId":  "member-group",
		"displayName": "Member Group",
	})
	groupID := body["id"].(string)

	// Create two users.
	var userIDs []string
	for i := 0; i < 2; i++ {
		status, body = apiReq(t, "POST", "/scim/v2/Users", apiKey, map[string]any{
			"externalId":  "mg-user-" + uuid.NewString()[:8],
			"displayName": "Member User",
			"email":       "mg-" + uuid.NewString()[:8] + "@example.com",
			"active":      true,
		})
		userIDs = append(userIDs, body["id"].(string))
	}

	// PATCH: add members.
	status, body = apiReq(t, "PATCH", "/scim/v2/Groups/"+groupID, apiKey, map[string]any{
		"Operations": []map[string]any{
			{
				"op": "add",
				"path": "members",
				"value": []map[string]any{
					{"value": userIDs[0]},
					{"value": userIDs[1]},
				},
			},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("add members: status %d, body %v", status, body)
	}

	// PATCH: remove one member.
	status, body = apiReq(t, "PATCH", "/scim/v2/Groups/"+groupID, apiKey, map[string]any{
		"Operations": []map[string]any{
			{
				"op":   "remove",
				"path": "members[value eq \"" + userIDs[0] + "\"]",
			},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("remove member: status %d, body %v", status, body)
	}

	// Verify group still exists.
	status, body = apiReq(t, "GET", "/scim/v2/Groups/"+groupID, apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get group after patch: status %d", status)
	}
	if body["displayName"] != "Member Group" {
		t.Fatalf("displayName = %v, want Member Group", body["displayName"])
	}
}

func TestSCIMPatchGroupNotFound(t *testing.T) {
	_, apiKey := createProviderAPI(t, "sg-pnf-"+uuid.NewString()[:8])

	status, _ := apiReq(t, "PATCH", "/scim/v2/Groups/"+uuid.NewString(), apiKey, map[string]any{
		"Operations": []map[string]any{
			{"op": "replace", "path": "displayName", "value": "New Name"},
		},
	})
	if status != http.StatusNotFound {
		t.Fatalf("patch non-existent group: status %d, want 404", status)
	}
}
