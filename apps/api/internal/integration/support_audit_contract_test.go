package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestSupportSessionAuditCorrelation proves the JIT support lifecycle is
// fully auditable and that audit events carry request_id for support
// correlation.
func TestSupportSessionAuditCorrelation(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "sup-audit-"+uuid.NewString()[:8])
	envID := getTestEnvID(t, providerID)

	status, body := apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/support-sessions", operatorToken, map[string]any{
		"environment_id":   envID,
		"access_type":      "standard",
		"reason":           "support audit contract",
		"requested_scopes": []string{"read"},
		"duration_seconds": 600,
	})
	if status != http.StatusCreated {
		t.Fatalf("request support session: status %d, body %v", status, body)
	}
	sessionID := body["id"].(string)

	status, body = apiReq(t, "POST", "/v1/support-sessions/"+sessionID+"/approve", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("approve support session: status %d, body %v", status, body)
	}

	status, body = apiReq(t, "GET", "/v1/audit-events", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list audit events: status %d, body %v", status, body)
	}
	events := body["audit_events"].([]any)
	found := map[string]bool{}
	for _, item := range events {
		event := item.(map[string]any)
		if event["action"] == "support.request" || event["action"] == "support.approve" {
			found[event["action"].(string)] = true
			if requestID, ok := event["request_id"].(string); !ok || requestID == "" {
				t.Fatalf("support audit event %v missing request_id", event["action"])
			}
		}
	}
	if !found["support.request"] || !found["support.approve"] {
		t.Fatalf("audit events missing support lifecycle: %v", found)
	}
}
