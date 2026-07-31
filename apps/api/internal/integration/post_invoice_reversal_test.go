package integration

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestUsagePostInvoiceReversal verifies Testing #6 post-invoice scenario:
// once usage has been included in a finalized invoice, direct reversal is
// rejected (409 usage_already_invoiced). The caller must issue a credit
// note instead — the finalized invoice is immutable (Decision #29).
func TestUsagePostInvoiceReversal(t *testing.T) {
	bc := setupBillingChain(t, "uso-postinv-"+uuid.NewString()[:8])
	txID := "tx-" + uuid.NewString()[:8]
	ts := time.Now().UTC().Format(time.RFC3339Nano)

	// Ingest usage.
	status, _ := ingestUsage(t, bc.apiKey, txID, bc.customerExt, "api_calls", ts, map[string]any{"count": 5})
	if status != http.StatusCreated {
		t.Fatalf("ingest: status %d", status)
	}

	// Simulate the usage being invoiced: insert a finalized invoice with
	// a line whose event_transaction_id matches the usage transaction_id.
	invID := insertTestInvoice(t, bc, "lago-postinv-1", "finalized", "subscription", true)
	pid, _ := uuid.Parse(bc.providerID)
	_, err := superPool.Exec(testCtx, `
		INSERT INTO invoice_lines (
            invoice_id, provider_id, environment_id, lago_fee_id,
            metric_code, item_type, item_name, units, precise_unit_amount,
            amount_cents, taxes_amount_cents, total_amount_cents, currency,
            event_transaction_id
        ) VALUES ($1, $2, $3, $4, 'api_calls', 'charge', 'API Calls', '5', '1.0', 400, 100, 500, 'USD', $5)`,
		invID, pid, bc.envID, "fee-postinv-1", txID)
	if err != nil {
		t.Fatalf("insert invoice line: %v", err)
	}

	// Attempt to reverse the usage → 409 usage_already_invoiced.
	status, body := apiReq(t, "POST", "/v1/usage/reverse", bc.apiKey, map[string]any{
		"original_transaction_id": txID,
	})
	if status != http.StatusConflict {
		t.Fatalf("post-invoice reversal: status %d, want 409, body %v", status, body)
	}
	if errObj, ok := body["error"].(map[string]any); ok {
		if errObj["code"] != "usage_already_invoiced" {
			t.Fatalf("error code = %v, want usage_already_invoiced", errObj["code"])
		}
		// Error message should mention credit note.
		msg, _ := errObj["message"].(string)
		if msg == "" || !strings.Contains(msg, "credit note") {
			t.Fatalf("error message should mention credit note, got: %v", errObj["message"])
		}
	} else {
		t.Fatalf("expected error object, got %v", body)
	}

	// The usage event is still intact (reversal was blocked).
	status, body = apiReq(t, "GET", "/v1/usage/events", bc.apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list usage events: status %d", status)
	}
	events, ok := body["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected 1 usage event (reversal blocked), got %v", body["events"])
	}
	// Verify it's still an ingestion (not reversed).
	ev := events[0].(map[string]any)
	if ev["kind"] != "ingestion" {
		t.Fatalf("usage event kind = %v, want ingestion (reversal blocked)", ev["kind"])
	}
}

// TestUsagePreInvoiceReversalSucceeds verifies that reversal still works
// when the usage has NOT been invoiced (the normal pre-invoice case).
func TestUsagePreInvoiceReversalSucceeds(t *testing.T) {
	bc := setupBillingChain(t, "uso-preinv-"+uuid.NewString()[:8])
	txID := "tx-" + uuid.NewString()[:8]
	ts := time.Now().UTC().Format(time.RFC3339Nano)

	// Ingest usage (no invoice created).
	status, _ := ingestUsage(t, bc.apiKey, txID, bc.customerExt, "api_calls", ts, map[string]any{"count": 5})
	if status != http.StatusCreated {
		t.Fatalf("ingest: status %d", status)
	}

	// Reverse should succeed (usage not invoiced).
	status, body := apiReq(t, "POST", "/v1/usage/reverse", bc.apiKey, map[string]any{
		"original_transaction_id": txID,
	})
	if status != http.StatusCreated {
		t.Fatalf("pre-invoice reversal: status %d, want 201, body %v", status, body)
	}

	// Two events: ingestion + reversal.
	status, body = apiReq(t, "GET", "/v1/usage/events", bc.apiKey, nil)
	events, ok := body["events"].([]any)
	if !ok || len(events) != 2 {
		t.Fatalf("expected 2 events, got %v", body["events"])
	}
}
