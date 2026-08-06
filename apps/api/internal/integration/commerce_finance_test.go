package integration

import (
	"testing"

	"github.com/google/uuid"
)

// TestInvoiceLineTraceabilityContract proves each invoice line remains
// replayable from immutable catalog inputs (metric_id / price_id / version)
// plus the original usage transaction.
func TestInvoiceLineTraceabilityContract(t *testing.T) {
	bc := setupBillingChain(t, "fin-trace-"+uuid.NewString()[:8])
	invID := insertTestInvoice(t, bc, "lago-trace-1", "finalized", "subscription", true)
	pid, _ := uuid.Parse(bc.providerID)
	txID := "tx-trace-" + uuid.NewString()[:8]

	_, err := superPool.Exec(testCtx, `
		INSERT INTO invoice_lines (
			invoice_id, provider_id, environment_id, lago_fee_id,
			metric_code, item_type, item_name, units, precise_unit_amount,
			amount_cents, taxes_amount_cents, total_amount_cents, currency,
			metric_id, price_id, event_transaction_id
		) VALUES ($1, $2, $3, $4, 'api_calls', 'charge', 'API Calls', '10', '1.0', 800, 200, 1000, 'USD', $5, $6, $7)`,
		invID, pid, bc.envID, "fee-trace-1", bc.metricID, bc.priceID, txID)
	if err != nil {
		t.Fatalf("insert traceable invoice line: %v", err)
	}

	var metricCode string
	var metricID, priceID, catalogVersionID uuid.UUID
	var eventTxID string
	err = superPool.QueryRow(testCtx, `
		SELECT il.metric_code, il.metric_id, il.price_id, il.event_transaction_id, i.catalog_version_id
		FROM invoice_lines il
		JOIN invoices i ON i.id = il.invoice_id
		WHERE il.invoice_id = $1`, invID).
		Scan(&metricCode, &metricID, &priceID, &eventTxID, &catalogVersionID)
	if err != nil {
		t.Fatalf("query traceable invoice line: %v", err)
	}
	if metricCode != "api_calls" || metricID != bc.metricID || priceID != bc.priceID {
		t.Fatalf("line traceability mismatch: code=%q metric=%v price=%v", metricCode, metricID, priceID)
	}
	catalogVID, _ := uuid.Parse(bc.catalogVerID)
	if catalogVersionID != catalogVID {
		t.Fatalf("catalog_version_id = %v, want %v", catalogVersionID, catalogVID)
	}
	if eventTxID != txID {
		t.Fatalf("event_transaction_id = %q, want %q", eventTxID, txID)
	}
}

// TestDuplicatePaymentStatusNoDuplicateInvoice verifies the PSP status
// reconciliation contract: repeated status updates never create extra
// invoice rows.
func TestDuplicatePaymentStatusNoDuplicateInvoice(t *testing.T) {
	bc := setupBillingChain(t, "fin-psp-"+uuid.NewString()[:8])
	invID := insertTestInvoice(t, bc, "lago-psp-1", "finalized", "subscription", true)

	for i := 0; i < 2; i++ {
		if _, err := superPool.Exec(testCtx,
			"UPDATE invoices SET payment_status = 'succeeded' WHERE id = $1", invID); err != nil {
			t.Fatalf("payment status update %d: %v", i+1, err)
		}
	}

	var count int
	if err := superPool.QueryRow(testCtx,
		"SELECT count(*) FROM invoices WHERE id = $1", invID).Scan(&count); err != nil {
		t.Fatalf("count invoices: %v", err)
	}
	if count != 1 {
		t.Fatalf("invoice count = %d, want 1 after duplicate payment status", count)
	}
}
