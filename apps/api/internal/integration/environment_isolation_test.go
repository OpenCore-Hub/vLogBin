package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func apiReqEnv(t *testing.T, method, path, token, env string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, httpServer.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if env != "" {
		req.Header.Set("X-Environment", env)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var decoded map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("undecodable body %q: %v", raw, err)
		}
	}
	return resp.StatusCode, decoded
}

func activateLiveEnv(t *testing.T, providerID string) string {
	t.Helper()
	base := "/v1/operator/providers/" + providerID + "/lifecycle"
	if status, body := apiReq(t, http.MethodPost, base, operatorToken, map[string]any{"to": "LIVE_REVIEW"}); status != http.StatusOK {
		t.Fatalf("to LIVE_REVIEW: status %d, body %v", status, body)
	}
	submitApprovedRiskReview(t, providerID)
	status, body := apiReq(t, http.MethodPost, base, operatorToken, map[string]any{"to": "LIVE_ACTIVE"})
	if status != http.StatusOK {
		t.Fatalf("to LIVE_ACTIVE: status %d, body %v", status, body)
	}
	liveKey, _ := body["api_key"].(string)
	if liveKey == "" {
		t.Fatalf("live activation must return an api_key, got %v", body)
	}
	return liveKey
}

// TestEnvironmentHeaderContract verifies the X-Environment header contract:
// optional on provider-domain requests, but when present it must match the
// environment bound to the credential.
func TestEnvironmentHeaderContract(t *testing.T) {
	_, testKey := createProviderAPI(t, "env-hdr-"+uuid.NewString()[:8])

	status, body := apiReqEnv(t, http.MethodGet, "/v1/whoami", testKey, "live")
	if status != http.StatusBadRequest {
		t.Fatalf("mismatched header: status %d, want 400, body %v", status, body)
	}
	if errObj, ok := body["error"].(map[string]any); ok {
		if code := errObj["code"]; code != "environment_mismatch" {
			t.Fatalf("error code = %v, want environment_mismatch", code)
		}
	}
	if status, _ := apiReqEnv(t, http.MethodGet, "/v1/whoami", testKey, "test"); status != http.StatusOK {
		t.Fatalf("matching header: status %d, want 200", status)
	}
	if status, _ := apiReqEnv(t, http.MethodGet, "/v1/whoami", testKey, ""); status != http.StatusOK {
		t.Fatalf("missing header: status %d, want 200", status)
	}
}

// TestEnvironmentIsolationEndToEnd walks the full billing chain in test,
// activates live, and verifies the two environments stay fully isolated:
// customers, subscriptions, usage, and catalog plans are invisible to the
// other environment's credential, and external ids may be reused per env.
func TestEnvironmentIsolationEndToEnd(t *testing.T) {
	providerID, testKey := createProviderAPI(t, "env-e2e-"+uuid.NewString()[:8])

	// Seed the test environment: published plan + customer + subscription + usage.
	versionID := createPublishedCatalog(t, testKey)
	status, body := apiReq(t, http.MethodPost, "/v1/customers", testKey, map[string]any{
		"external_id": "iso-cust", "account_type": "business", "display_name": "ISO Test",
	})
	if status != http.StatusCreated {
		t.Fatalf("create test customer: status %d, body %v", status, body)
	}
	status, _ = apiReq(t, http.MethodPost, "/v1/subscriptions", testKey, map[string]any{
		"external_id": "iso-sub", "customer_external_id": "iso-cust",
		"catalog_version_id": versionID, "plan_code": "starter",
	})
	if status != http.StatusCreated {
		t.Fatalf("create test subscription: status %d", status)
	}
	if status, _ := ingestUsage(t, testKey, "iso-tx", "iso-cust", "api_calls",
		"2026-08-01T00:00:00Z", map[string]any{"count": 1}); status != http.StatusCreated {
		t.Fatalf("ingest test usage: status %d", status)
	}

	liveKey := activateLiveEnv(t, providerID)

	// Test data is invisible to the live credential.
	if status, body := apiReq(t, http.MethodGet, "/v1/customers", liveKey, nil); status != http.StatusOK {
		t.Fatalf("live customers: status %d, body %v", status, body)
	} else if customers, _ := body["customers"].([]any); len(customers) != 0 {
		t.Fatalf("live customers = %v, want empty", body["customers"])
	}
	if status, body := apiReq(t, http.MethodGet, "/v1/subscriptions", liveKey, nil); status != http.StatusOK {
		t.Fatalf("live subscriptions: status %d, body %v", status, body)
	} else if subs, _ := body["subscriptions"].([]any); len(subs) != 0 {
		t.Fatalf("live subscriptions = %v, want empty", body["subscriptions"])
	}
	if status, body := apiReq(t, http.MethodGet, "/v1/usage/events", liveKey, nil); status != http.StatusOK {
		t.Fatalf("live usage: status %d, body %v", status, body)
	} else if events, _ := body["usage_events"].([]any); len(events) != 0 {
		t.Fatalf("live usage_events = %v, want empty", body["usage_events"])
	}

	// The same external id can exist independently in live (per-env unique).
	status, body = apiReq(t, http.MethodPost, "/v1/customers", liveKey, map[string]any{
		"external_id": "iso-cust", "account_type": "business", "display_name": "ISO Live",
	})
	if status != http.StatusCreated {
		t.Fatalf("create live customer: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, "/v1/customers", testKey, nil); status != http.StatusOK {
		t.Fatalf("test customers: status %d, body %v", status, body)
	} else if customers, _ := body["customers"].([]any); len(customers) != 1 {
		t.Fatalf("test customers after live create = %v, want 1", body["customers"])
	}
	if status, body := apiReq(t, http.MethodGet, "/v1/customers", liveKey, nil); status != http.StatusOK {
		t.Fatalf("live customers: status %d, body %v", status, body)
	} else if customers, _ := body["customers"].([]any); len(customers) != 1 {
		t.Fatalf("live customers = %v, want 1", body["customers"])
	}

	// Catalog plans are isolated through the Console control plane too.
	plansBase := "/v1/operator/providers/" + providerID + "/catalog/plans"
	if status, body := apiReq(t, http.MethodGet, plansBase+"?env=test", operatorToken, nil); status != http.StatusOK {
		t.Fatalf("test plans: status %d, body %v", status, body)
	} else if plans, _ := body["plans"].([]any); len(plans) != 1 {
		t.Fatalf("test plans = %v, want 1 (starter)", body["plans"])
	}
	if status, body := apiReq(t, http.MethodGet, plansBase+"?env=live", operatorToken, nil); status != http.StatusOK {
		t.Fatalf("live plans: status %d, body %v", status, body)
	} else if plans, _ := body["plans"].([]any); len(plans) != 0 {
		t.Fatalf("live plans = %v, want empty", body["plans"])
	}
	if status, body := apiReq(t, http.MethodPost, plansBase+"?env=live", operatorToken, map[string]any{
		"code": "live-starter", "name": "Live Starter", "interval": "monthly", "currency": "USD",
		"prices": []map[string]any{
			{"charge_model": "fixed", "properties": map[string]any{"amount_cents": 2000, "currency": "USD"}},
		},
	}); status != http.StatusCreated {
		t.Fatalf("create live plan: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, plansBase+"?env=live", operatorToken, nil); status != http.StatusOK {
		t.Fatalf("live plans after create: status %d, body %v", status, body)
	} else if plans, _ := body["plans"].([]any); len(plans) != 1 {
		t.Fatalf("live plans after create = %v, want 1", body["plans"])
	}
	var testBody map[string]any
	status, testBody = apiReq(t, http.MethodGet, plansBase+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("test plans recheck: status %d, body %v", status, testBody)
	}
	plans, _ := testBody["plans"].([]any)
	if len(plans) != 1 || plans[0].(map[string]any)["plan"].(map[string]any)["code"] != "starter" {
		t.Fatalf("test plans must stay starter-only, got %v", testBody["plans"])
	}
}
