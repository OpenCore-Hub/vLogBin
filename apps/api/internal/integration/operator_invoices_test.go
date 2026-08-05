package integration

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// TestOperatorInvoicesListAndDetail verifies the Console-facing invoice
// endpoints: env-scoped list with customer_external_id resolved, detail with
// line items, and the unchanged cross-environment operator list.
func TestOperatorInvoicesListAndDetail(t *testing.T) {
	bc := setupBillingChain(t, "op-inv-"+uuid.NewString()[:8])
	invID := insertTestInvoice(t, bc, "lago-op-inv-1", "draft", "subscription", false)
	insertTestInvoiceLine(t, bc, invID, "fee-op-1", "api_calls")

	base := "/v1/operator/providers/" + bc.providerID + "/invoices"

	// Env-scoped list.
	status, body := apiReq(t, http.MethodGet, base+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list env invoices: status %d, body %v", status, body)
	}
	invoices, _ := body["invoices"].([]any)
	if len(invoices) != 1 {
		t.Fatalf("invoices = %v, want 1", body["invoices"])
	}
	inv := invoices[0].(map[string]any)
	if inv["customer_external_id"] != bc.customerExt {
		t.Fatalf("invoice customer = %v, want %s", inv["customer_external_id"], bc.customerExt)
	}
	if inv["environment_kind"] != "test" {
		t.Fatalf("invoice environment_kind = %v, want test", inv["environment_kind"])
	}

	// Cross-environment list still works without ?env=.
	status, body = apiReq(t, http.MethodGet, base, operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list cross-env invoices: status %d, body %v", status, body)
	}
	if invoices, _ := body["invoices"].([]any); len(invoices) != 1 {
		t.Fatalf("cross-env invoices = %v, want 1", body["invoices"])
	}

	// Detail with line items.
	status, body = apiReq(t, http.MethodGet, base+"/"+invID.String()+"?env=test", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("get invoice detail: status %d, body %v", status, body)
	}
	detailInv, _ := body["invoice"].(map[string]any)
	if detailInv["number"] != "INV-lago-op-inv-1" {
		t.Fatalf("detail invoice number = %v", detailInv["number"])
	}
	lines, _ := body["lines"].([]any)
	if len(lines) != 1 {
		t.Fatalf("lines = %v, want 1", body["lines"])
	}
	if lines[0].(map[string]any)["metric_code"] != "api_calls" {
		t.Fatalf("line metric = %v", lines[0])
	}
}

// TestOperatorInvoicesValidation verifies the control-plane error contract.
func TestOperatorInvoicesValidation(t *testing.T) {
	bc := setupBillingChain(t, "op-invv-"+uuid.NewString()[:8])
	invID := insertTestInvoice(t, bc, "lago-op-invv-1", "draft", "subscription", false)
	base := "/v1/operator/providers/" + bc.providerID + "/invoices"

	if status, body := apiReq(t, http.MethodGet, base+"/"+invID.String(), operatorToken, nil); status != http.StatusBadRequest {
		t.Fatalf("missing env: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, base+"/"+invID.String()+"?env=staging", operatorToken, nil); status != http.StatusBadRequest {
		t.Fatalf("invalid env: status %d, body %v", status, body)
	}
	// setupBillingChain provisions only a test environment; live is absent.
	if status, body := apiReq(t, http.MethodGet, base+"/"+invID.String()+"?env=live", operatorToken, nil); status != http.StatusNotFound {
		t.Fatalf("unknown environment: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, base+"/not-a-uuid?env=test", operatorToken, nil); status != http.StatusBadRequest {
		t.Fatalf("invalid invoice id: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, base+"/"+uuid.NewString()+"?env=test", operatorToken, nil); status != http.StatusNotFound {
		t.Fatalf("unknown invoice: status %d, body %v", status, body)
	}
	if status, body := apiReq(t, http.MethodGet, "/v1/operator/providers/"+uuid.NewString()+"/invoices/"+invID.String()+"?env=test", operatorToken, nil); status != http.StatusNotFound {
		t.Fatalf("unknown provider: status %d, body %v", status, body)
	}
}
