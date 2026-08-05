package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestOperatorCustomersCrudAndList verifies the Console-facing customer
// endpoints: create → list (env-scoped) → duplicate conflict, plus the
// environment/provider validation matrix shared with the other control plane.
func TestOperatorCustomersCrudAndList(t *testing.T) {
	providerID, _ := createProviderAPI(t, "op-cust-"+uuid.NewString()[:8])
	base := "/v1/operator/providers/" + providerID + "/customers"

	// Empty list before any customer exists.
	status, body := apiReq(t, http.MethodGet, base+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list empty: status %d, body %v", status, body)
	}
	if customers, _ := body["customers"].([]any); len(customers) != 0 {
		t.Fatalf("customers = %v, want empty", body["customers"])
	}

	// Create a customer through the operator control plane.
	status, body = apiReq(t, http.MethodPost, base+"?env=test", operatorToken, map[string]any{
		"external_id":  "acme",
		"account_type": "business",
		"display_name": "Acme Corp",
	})
	if status != http.StatusCreated {
		t.Fatalf("create customer: status %d, body %v", status, body)
	}
	customer, _ := body["customer"].(map[string]any)
	if customer["external_id"] != "acme" || customer["display_name"] != "Acme Corp" {
		t.Fatalf("created customer = %v", customer)
	}

	// Duplicate external id in the same environment → 409.
	status, body = apiReq(t, http.MethodPost, base+"?env=test", operatorToken, map[string]any{
		"external_id":  "acme",
		"account_type": "business",
		"display_name": "Acme Again",
	})
	if status != http.StatusConflict {
		t.Fatalf("duplicate customer: status %d, want 409, body %v", status, body)
	}

	// List now returns exactly one customer, env-scoped.
	status, body = apiReq(t, http.MethodGet, base+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d, body %v", status, body)
	}
	customers, _ := body["customers"].([]any)
	if len(customers) != 1 {
		t.Fatalf("customers = %v, want 1", body["customers"])
	}
}

// TestOperatorCustomerDetail verifies the customer detail payload: one
// request returns the customer plus its subscriptions, usage events and
// invoices, all filtered to that customer by the API.
func TestOperatorCustomerDetail(t *testing.T) {
	bc := setupBillingChain(t, "op-custd-"+uuid.NewString()[:8])

	// Seed one usage event for the customer.
	txID := "cust-detail-" + uuid.NewString()[:8]
	status, _ := ingestUsage(t, bc.apiKey, txID, bc.customerExt, "api_calls",
		"2026-08-01T00:00:00Z", map[string]any{"count": 1})
	if status != http.StatusCreated {
		t.Fatalf("ingest usage: status %d", status)
	}

	// Seed one invoice for the same customer.
	insertTestInvoice(t, bc, "lago-cust-detail-1", "draft", "subscription", false)

	base := "/v1/operator/providers/" + bc.providerID + "/customers"
	status, body := apiReq(t, http.MethodGet, base+"/"+bc.customerExt+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("get customer detail: status %d, body %v", status, body)
	}
	customer, _ := body["customer"].(map[string]any)
	if customer["external_id"] != bc.customerExt {
		t.Fatalf("detail customer = %v", customer)
	}
	subs, _ := body["subscriptions"].([]any)
	if len(subs) != 1 {
		t.Fatalf("subscriptions = %v, want 1", body["subscriptions"])
	}
	if subs[0].(map[string]any)["plan_code"] != "starter" {
		t.Fatalf("subscription plan = %v", subs[0])
	}
	events, _ := body["usage_events"].([]any)
	if len(events) != 1 || events[0].(map[string]any)["transaction_id"] != txID {
		t.Fatalf("usage_events = %v, want %s", body["usage_events"], txID)
	}
	invoices, _ := body["invoices"].([]any)
	if len(invoices) != 1 {
		t.Fatalf("invoices = %v, want 1", body["invoices"])
	}

	// Unknown customer → 404.
	status, body = apiReq(t, http.MethodGet, base+"/missing-customer?env=test", operatorToken, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown customer: status %d, want 404, body %v", status, body)
	}
}

// TestOperatorCustomersValidation verifies the control-plane error contract.
func TestOperatorCustomersValidation(t *testing.T) {
	providerID, _ := createProviderAPI(t, "op-custv-"+uuid.NewString()[:8])
	base := "/v1/operator/providers/" + providerID + "/customers"

	if status, body := apiReq(t, http.MethodPost, base, operatorToken, map[string]any{
		"external_id": "x", "account_type": "business", "display_name": "X",
	}); status != http.StatusBadRequest {
		t.Fatalf("missing env: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodPost, base+"?env=staging", operatorToken, map[string]any{
		"external_id": "x", "account_type": "business", "display_name": "X",
	}); status != http.StatusBadRequest {
		t.Fatalf("invalid env: status %d, body %v", status, body)
	}
	// createProviderAPI provisions only a test environment; live is absent.
	if status, body := apiReq(t, http.MethodPost, base+"?env=live", operatorToken, map[string]any{
		"external_id": "x", "account_type": "business", "display_name": "X",
	}); status != http.StatusNotFound {
		t.Fatalf("unknown environment: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodPost, "/v1/operator/providers/not-a-uuid/customers?env=test", operatorToken, map[string]any{
		"external_id": "x", "account_type": "business", "display_name": "X",
	}); status != http.StatusBadRequest {
		t.Fatalf("invalid id: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodPost, "/v1/operator/providers/"+uuid.NewString()+"/customers?env=test", operatorToken, map[string]any{
		"external_id": "x", "account_type": "business", "display_name": "X",
	}); status != http.StatusNotFound {
		t.Fatalf("unknown provider: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodPost, base+"?env=test", operatorToken, map[string]any{
		"external_id": "", "account_type": "business", "display_name": "X",
	}); status != http.StatusBadRequest {
		t.Fatalf("empty external id: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodPost, base+"?env=test", operatorToken, map[string]any{
		"external_id": "x", "account_type": "enterprise", "display_name": "X",
	}); status != http.StatusBadRequest {
		t.Fatalf("invalid account type: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, base+"/missing?env=test", operatorToken, nil); status != http.StatusNotFound {
		t.Fatalf("unknown customer get: status %d, body %v", status, body)
	}
}
