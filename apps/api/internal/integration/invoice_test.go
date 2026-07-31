package integration

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// billingChain holds the IDs needed to insert invoices and lines directly
// via the superuser pool after setting up a full billing chain through the
// public API.
type billingChain struct {
	providerID     string
	envID          uuid.UUID
	apiKey         string
	customerID     uuid.UUID
	customerExt    string
	subscriptionID uuid.UUID
	catalogVerID   string
	metricID       uuid.UUID // api_calls metric from the catalog version
	priceID        uuid.UUID // fixed price for api_calls in the starter plan
}

// setupBillingChain creates a provider, publishes a catalog, and creates a
// customer + subscription. Returns all IDs needed for direct DB insertion.
func setupBillingChain(t *testing.T, slug string) billingChain {
	t.Helper()
	providerID, apiKey := createProviderAPI(t, slug)
	versionID := createPublishedCatalog(t, apiKey)

	// Create customer.
	custExt := "cust-" + uuid.NewString()[:8]
	status, body := apiReq(t, "POST", "/v1/customers", apiKey, map[string]any{
		"external_id":  custExt,
		"account_type": "business",
		"display_name": "Invoice Test Customer",
	})
	if status != http.StatusCreated {
		t.Fatalf("create customer: status %d, body %v", status, body)
	}
	customerID, _ := uuid.Parse(body["customer"].(map[string]any)["id"].(string))

	// Create subscription.
	subExt := "sub-" + uuid.NewString()[:8]
	status, body = apiReq(t, "POST", "/v1/subscriptions", apiKey, map[string]any{
		"external_id":         subExt,
		"customer_external_id": custExt,
		"catalog_version_id":  versionID,
		"plan_code":           "starter",
	})
	if status != http.StatusCreated {
		t.Fatalf("create subscription: status %d, body %v", status, body)
	}
	subID, _ := uuid.Parse(body["subscription"].(map[string]any)["id"].(string))

	// Resolve the test environment ID (needed for direct DB insertion).
	var envID uuid.UUID
	if err := superPool.QueryRow(testCtx,
		"SELECT id FROM environments WHERE provider_id = $1 AND kind = 'test' LIMIT 1",
		providerID).Scan(&envID); err != nil {
		t.Fatalf("resolve env: %v", err)
	}

	// Resolve metric_id and price_id for per-line traceability (Testing #9).
	vid, _ := uuid.Parse(versionID)
	var metricID uuid.UUID
	if err := superPool.QueryRow(testCtx,
		"SELECT id FROM metrics WHERE catalog_version_id = $1 AND code = 'api_calls' LIMIT 1",
		vid).Scan(&metricID); err != nil {
		t.Fatalf("resolve metric: %v", err)
	}
	var priceID uuid.UUID
	if err := superPool.QueryRow(testCtx,
		"SELECT id FROM prices WHERE catalog_version_id = $1 AND metric_id = $2 LIMIT 1",
		vid, metricID).Scan(&priceID); err != nil {
		t.Fatalf("resolve price: %v", err)
	}

	return billingChain{
		providerID:     providerID,
		envID:          envID,
		apiKey:         apiKey,
		customerID:     customerID,
		customerExt:    custExt,
		subscriptionID: subID,
		catalogVerID:   versionID,
		metricID:       metricID,
		priceID:        priceID,
	}
}

// insertTestInvoice inserts an invoice via the superuser pool (bypassing
// RLS) and returns the invoice ID. This is a white-box helper for testing
// DB-level constraints and read APIs without a running Lago.
func insertTestInvoice(t *testing.T, bc billingChain, lagoID, status, invoiceType string, finalize bool) uuid.UUID {
	t.Helper()
	pid, _ := uuid.Parse(bc.providerID)
	vid, _ := uuid.Parse(bc.catalogVerID)
	var finalizedAt *time.Time
	if finalize {
		now := time.Now().UTC()
		finalizedAt = &now
	}
	var id uuid.UUID
	err := superPool.QueryRow(testCtx, `
		INSERT INTO invoices (
            provider_id, environment_id, lago_id, number,
            customer_account_id, subscription_id, catalog_version_id,
            issuing_date, invoice_type, status, payment_status, currency,
            total_amount_cents, finalized_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'pending', 'USD', 1000, $11)
        RETURNING id`,
		pid, bc.envID, lagoID, "INV-"+lagoID,
		bc.customerID, bc.subscriptionID, vid,
		time.Now().UTC().Truncate(24*time.Hour), invoiceType, status, finalizedAt,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert invoice: %v", err)
	}
	return id
}

// insertTestInvoiceLine inserts a single invoice line via the superuser pool.
// metricID and priceID enable per-line catalog traceability (Testing #9).
func insertTestInvoiceLine(t *testing.T, bc billingChain, invoiceID uuid.UUID, lagoFeeID, metricCode string) {
	t.Helper()
	pid, _ := uuid.Parse(bc.providerID)
	_, err := superPool.Exec(testCtx, `
		INSERT INTO invoice_lines (
            invoice_id, provider_id, environment_id, lago_fee_id,
            metric_code, item_type, item_name, units, precise_unit_amount,
            amount_cents, taxes_amount_cents, total_amount_cents, currency,
            metric_id, price_id
        ) VALUES ($1, $2, $3, $4, $5, 'charge', 'API Calls', '10', '1.0', 800, 200, 1000, 'USD', $6, $7)`,
		invoiceID, pid, bc.envID, lagoFeeID, metricCode, bc.metricID, bc.priceID)
	if err != nil {
		t.Fatalf("insert invoice line: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Testing #9 (partial): finalized invoice immutability at DB level
// ---------------------------------------------------------------------------

func TestInvoiceFinalizedImmutability(t *testing.T) {
	bc := setupBillingChain(t, "inv-immut-"+uuid.NewString()[:8])
	invID := insertTestInvoice(t, bc, "lago-finalized-1", "finalized", "subscription", true)
	insertTestInvoiceLine(t, bc, invID, "fee-1", "api_calls")

	// Attempt to modify a financial field on a finalized invoice → trigger error.
	_, err := superPool.Exec(testCtx,
		"UPDATE invoices SET total_amount_cents = 99999 WHERE id = $1", invID)
	if err == nil {
		t.Fatal("expected trigger error when modifying finalized invoice financial field")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("expected 'immutable' in error, got: %v", err)
	}

	// Attempt to delete a line from a finalized invoice → trigger error.
	_, err = superPool.Exec(testCtx,
		"DELETE FROM invoice_lines WHERE invoice_id = $1", invID)
	if err == nil {
		t.Fatal("expected trigger error when deleting line from finalized invoice")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected 'not allowed' in error, got: %v", err)
	}

	// Non-financial fields (payment_status, file_url) CAN be updated.
	_, err = superPool.Exec(testCtx,
		"UPDATE invoices SET payment_status = 'succeeded', file_url = 'https://example.com/invoice.pdf' WHERE id = $1", invID)
	if err != nil {
		t.Fatalf("updating non-financial field on finalized invoice should succeed, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Invoice list and get API
// ---------------------------------------------------------------------------

func TestInvoiceListAndGet(t *testing.T) {
	bc := setupBillingChain(t, "inv-api-"+uuid.NewString()[:8])
	invID := insertTestInvoice(t, bc, "lago-list-1", "finalized", "subscription", true)
	insertTestInvoiceLine(t, bc, invID, "fee-list-1", "api_calls")
	insertTestInvoiceLine(t, bc, invID, "fee-list-2", "api_calls")

	// List invoices via provider API.
	status, body := apiReq(t, "GET", "/v1/invoices", bc.apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list invoices: status %d, body %v", status, body)
	}
	invoices, ok := body["invoices"].([]any)
	if !ok || len(invoices) != 1 {
		t.Fatalf("expected 1 invoice, got %v", body["invoices"])
	}
	inv := invoices[0].(map[string]any)
	if inv["status"] != "finalized" {
		t.Fatalf("status = %v, want finalized", inv["status"])
	}
	if inv["total_amount_cents"].(float64) != 1000 {
		t.Fatalf("total_amount_cents = %v, want 1000", inv["total_amount_cents"])
	}

	// Get invoice detail with lines.
	status, body = apiReq(t, "GET", "/v1/invoices/"+invID.String(), bc.apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get invoice: status %d, body %v", status, body)
	}
	lines, ok := body["lines"].([]any)
	if !ok || len(lines) != 2 {
		t.Fatalf("expected 2 invoice lines, got %v", body)
	}
	// Verify line has metric_code for traceability.
	line := lines[0].(map[string]any)
	if line["metric_code"] != "api_calls" {
		t.Fatalf("metric_code = %v, want api_calls", line["metric_code"])
	}
	// Verify per-line catalog traceability (Testing #9): metric_id and
	// price_id link the line to the exact metric and price in the
	// catalog version that was active when the subscription was created.
	if line["metric_id"] == nil {
		t.Fatal("metric_id is null; expected non-null for per-line traceability")
	}
	if line["price_id"] == nil {
		t.Fatal("price_id is null; expected non-null for per-line traceability")
	}
}

// ---------------------------------------------------------------------------
// Credit invoice as new record (does not modify original finalized invoice)
// ---------------------------------------------------------------------------

func TestInvoiceCreditAsNewRecord(t *testing.T) {
	bc := setupBillingChain(t, "inv-credit-"+uuid.NewString()[:8])

	// Insert the original finalized invoice.
	origID := insertTestInvoice(t, bc, "lago-orig-1", "finalized", "subscription", true)
	insertTestInvoiceLine(t, bc, origID, "fee-orig-1", "api_calls")

	// Insert a credit note as a NEW invoice record (not modifying the original).
	creditID := insertTestInvoice(t, bc, "lago-credit-1", "finalized", "credit", true)
	insertTestInvoiceLine(t, bc, creditID, "fee-credit-1", "api_calls")

	if origID == creditID {
		t.Fatal("credit invoice must be a separate record")
	}

	// List invoices — both should appear.
	status, body := apiReq(t, "GET", "/v1/invoices", bc.apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list: status %d", status)
	}
	invoices, ok := body["invoices"].([]any)
	if !ok || len(invoices) != 2 {
		t.Fatalf("expected 2 invoices (original + credit), got %v", body["invoices"])
	}

	// Verify the original invoice is unchanged.
	status, body = apiReq(t, "GET", "/v1/invoices/"+origID.String(), bc.apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get original: status %d", status)
	}
	if body["invoice"].(map[string]any)["invoice_type"] != "subscription" {
		t.Fatalf("original invoice_type changed")
	}
	if body["invoice"].(map[string]any)["total_amount_cents"].(float64) != 1000 {
		t.Fatalf("original total_amount_cents changed")
	}

	// Verify the credit invoice has type=credit.
	status, body = apiReq(t, "GET", "/v1/invoices/"+creditID.String(), bc.apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("get credit: status %d", status)
	}
	if body["invoice"].(map[string]any)["invoice_type"] != "credit" {
		t.Fatalf("credit invoice_type = %v, want credit", body["invoice"].(map[string]any)["invoice_type"])
	}
}

// ---------------------------------------------------------------------------
// Draft invoice can be modified (lines replaced on re-sync)
// ---------------------------------------------------------------------------

func TestInvoiceDraftCanBeModified(t *testing.T) {
	bc := setupBillingChain(t, "inv-draft-"+uuid.NewString()[:8])
	invID := insertTestInvoice(t, bc, "lago-draft-1", "draft", "subscription", false)
	insertTestInvoiceLine(t, bc, invID, "fee-draft-1", "api_calls")

	// Draft invoice: financial fields CAN be updated.
	_, err := superPool.Exec(testCtx,
		"UPDATE invoices SET total_amount_cents = 2000 WHERE id = $1", invID)
	if err != nil {
		t.Fatalf("updating draft invoice should succeed, got: %v", err)
	}

	// Draft invoice lines CAN be deleted (for re-sync replacement).
	_, err = superPool.Exec(testCtx,
		"DELETE FROM invoice_lines WHERE invoice_id = $1", invID)
	if err != nil {
		t.Fatalf("deleting lines from draft invoice should succeed, got: %v", err)
	}
}
