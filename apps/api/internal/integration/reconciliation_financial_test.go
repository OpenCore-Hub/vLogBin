package integration

import (
	"context"
	"testing"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/service"
	"github.com/google/uuid"
)

// TestReconciliationFinancialChecks verifies the financial discrepancy
// reconciliation group added for the "hourly reconciliation, DLQ and
// financial discrepancy handling" P0 gate:
//
//  1. invoice_amount_consistency — finalized/voided invoices violating
//     Lago's amount invariants (incl = excl + taxes; total = incl - credit)
//  2. invoice_lines_total_match — line totals not summing to header total
//  3. unpaid_finalized_overdue — finalized invoices unpaid within 7 days
//
// Finalized/voided invoices and their lines are WORM-immutable (migration
// 0005 triggers): lines attached to a finalized invoice cannot be deleted,
// and the invoice's financial fields cannot change. The checks therefore
// assert *deltas* against a baseline captured at the start, which absorbs
// fixture rows left by other tests. Cleanup deletes the line-less invoices
// this test created; the two line-bearing WORM invoices are left in place
// (documented in PROGRESS.md).
func TestReconciliationFinancialChecks(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "fin-recon-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	ctx := context.Background()
	var custAccID uuid.UUID
	if err := superPool.QueryRow(ctx,
		`SELECT id FROM customer_accounts WHERE external_id = $1`, custExt).Scan(&custAccID); err != nil {
		t.Fatalf("lookup customer account: %v", err)
	}
	_ = providerID

	baseline := financialBaseline(t)

	// Stage 1: one self-consistent finalized invoice (with a matching line)
	// -> no new drift on any financial check.
	inv1 := insertInvoice(t, ctx, custAccID, invoiceSeed{
		invoiceType: "subscription", status: "finalized", paymentStatus: "succeeded",
		fees: 1000, subExcl: 1000, subIncl: 1000, total: 1000,
		finalizedAt: time.Now(),
	})
	insertInvoiceLine(t, ctx, inv1, 1000, 0, 1000)
	assertFinancialDelta(t, baseline, map[string]int64{
		"invoice_amount_consistency": 0,
		"invoice_lines_total_match":  0,
		"unpaid_finalized_overdue":   0,
	})

	// Stage 2: amount invariant violation (total != incl - credit notes).
	inv2 := insertInvoice(t, ctx, custAccID, invoiceSeed{
		invoiceType: "subscription", status: "finalized", paymentStatus: "succeeded",
		fees: 1000, subExcl: 1000, subIncl: 1000, total: 2000,
		finalizedAt: time.Now(),
	})
	assertFinancialDelta(t, baseline, map[string]int64{
		"invoice_amount_consistency": 1,
		"invoice_lines_total_match":  0,
		"unpaid_finalized_overdue":   0,
	})

	// Stage 3: line totals diverge from the header total.
	inv3 := insertInvoice(t, ctx, custAccID, invoiceSeed{
		invoiceType: "subscription", status: "finalized", paymentStatus: "succeeded",
		fees: 3000, subExcl: 3000, subIncl: 3000, total: 3000,
		finalizedAt: time.Now(),
	})
	insertInvoiceLine(t, ctx, inv3, 3000, 200, 3200)
	assertFinancialDelta(t, baseline, map[string]int64{
		"invoice_amount_consistency": 1,
		"invoice_lines_total_match":  1,
		"unpaid_finalized_overdue":   0,
	})

	// Stage 4: finalized invoice unpaid for more than 7 days.
	inv4 := insertInvoice(t, ctx, custAccID, invoiceSeed{
		invoiceType: "subscription", status: "finalized", paymentStatus: "pending",
		fees: 500, subExcl: 500, subIncl: 500, total: 500,
		finalizedAt: time.Now().Add(-10 * 24 * time.Hour),
	})
	assertFinancialDelta(t, baseline, map[string]int64{
		"invoice_amount_consistency": 1,
		"invoice_lines_total_match":  1,
		"unpaid_finalized_overdue":   1,
	})

	// Cleanup: line-less invoices can be deleted; inv1 (self-consistent) and
	// inv3 (drift fixture) carry lines and are WORM-immutable -> left behind.
	for _, id := range []uuid.UUID{inv2, inv4} {
		if _, err := superPool.Exec(ctx, "DELETE FROM invoices WHERE id = $1", id); err != nil {
			t.Errorf("cleanup invoice %s: %v", id, err)
		}
	}
}

type invoiceSeed struct {
	invoiceType   string
	status        string
	paymentStatus string
	fees          int64
	subExcl       int64
	subIncl       int64
	total         int64
	finalizedAt   time.Time
}

// insertInvoice inherits provider_id/environment_id from the customer
// account row so RLS and tenant columns stay consistent without knowing the
// environment UUID up front. Amount fields follow Lago's invariants by
// default; the caller overrides them to simulate drift.
func insertInvoice(t *testing.T, ctx context.Context, custAccID uuid.UUID, seed invoiceSeed) uuid.UUID {
	t.Helper()
	query := `INSERT INTO invoices (
		provider_id, environment_id, customer_account_id, lago_id,
		invoice_type, status, payment_status, currency, issuing_date, finalized_at,
		fees_amount_cents, coupons_amount_cents, credit_notes_amount_cents,
		sub_total_excluding_taxes_amount_cents, taxes_amount_cents,
		sub_total_including_taxes_amount_cents, total_amount_cents)
	SELECT provider_id, environment_id, $1, $2,
		$3, $4, $5, 'USD', $6::date, $7,
		$8, 0, 0,
		$9, 0,
		$10, $11
	FROM customer_accounts WHERE id = $1
	RETURNING id`
	var id uuid.UUID
	if err := superPool.QueryRow(ctx, query,
		custAccID, "lago-"+uuid.NewString(),
		seed.invoiceType, seed.status, seed.paymentStatus,
		seed.finalizedAt.UTC().Format("2006-01-02"), seed.finalizedAt.UTC(),
		seed.fees, seed.subExcl, seed.subIncl, seed.total,
	).Scan(&id); err != nil {
		t.Fatalf("insert invoice: %v", err)
	}
	return id
}

func insertInvoiceLine(t *testing.T, ctx context.Context, invoiceID uuid.UUID, amount, taxes, total int64) {
	t.Helper()
	query := `INSERT INTO invoice_lines (
		invoice_id, provider_id, environment_id, lago_fee_id, currency,
		amount_cents, taxes_amount_cents, total_amount_cents)
	SELECT $1, provider_id, environment_id, $2, 'USD', $3, $4, $5
	FROM invoices WHERE id = $1`
	if _, err := superPool.Exec(ctx, query, invoiceID, "fee-"+invoiceID.String()[:8], amount, taxes, total); err != nil {
		t.Fatalf("insert invoice line: %v", err)
	}
}

func financialBaseline(t *testing.T) map[string]int64 {
	t.Helper()
	results, err := svc.RunReconciliation(testCtx)
	if err != nil {
		t.Fatalf("RunReconciliation (baseline): %v", err)
	}
	base := map[string]int64{}
	for _, name := range []string{"invoice_amount_consistency", "invoice_lines_total_match", "unpaid_finalized_overdue"} {
		base[name] = financialDriftOf(t, results, name)
	}
	return base
}

// assertFinancialDelta runs a full reconciliation pass and asserts the
// *delta* of each financial check against the baseline captured at the
// start of the test (which absorbs WORM-immutable fixture rows).
func assertFinancialDelta(t *testing.T, baseline map[string]int64, want map[string]int64) {
	t.Helper()
	results, err := svc.RunReconciliation(testCtx)
	if err != nil {
		t.Fatalf("RunReconciliation: %v", err)
	}
	if len(results) < 8 {
		t.Fatalf("expected >= 8 checks, got %d", len(results))
	}
	for name, wantDelta := range want {
		got := financialDriftOf(t, results, name)
		delta := got - baseline[name]
		if delta != wantDelta {
			t.Fatalf("check %s delta = %d (current %d, baseline %d), want %d",
				name, delta, got, baseline[name], wantDelta)
		}
	}
}

func financialDriftOf(t *testing.T, results []service.ReconciliationCheck, name string) int64 {
	t.Helper()
	for _, r := range results {
		if r.Name == name {
			return int64(r.DriftCount)
		}
	}
	t.Fatalf("check %q not found in reconciliation results", name)
	return -1
}
