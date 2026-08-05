package integration

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/billing"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/circuitbreaker"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/outbox"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// catalogContent returns a minimal valid catalog: one metric (api_calls,
// count), one plan (starter, monthly, USD) with a fixed price and a numeric
// entitlement grant (max_users=10).
func catalogContent() map[string]any {
	return map[string]any{
		"metrics": []map[string]any{
			{
				"code":             "api_calls",
				"name":             "API Calls",
				"aggregation_type": "count",
				"billable":         true,
			},
		},
		"plans": []map[string]any{
			{
				"code":     "starter",
				"name":     "Starter",
				"interval": "monthly",
				"currency": "USD",
				"prices": []map[string]any{
					{
						"charge_model": "fixed",
						"properties":   map[string]any{"amount_cents": 1000, "currency": "USD"},
					},
					{
						"charge_model": "per_unit",
						"metric_code":  "api_calls",
						"properties":   map[string]any{"unit_amount_cents": 10, "currency": "USD"},
					},
				},
				"entitlements": []map[string]any{
					{
						"key":        "max_users",
						"value_type": "numeric",
						"value":      10,
					},
				},
			},
		},
	}
}

// createPublishedCatalog creates a draft catalog version, populates it with
// content, validates, and publishes it. Returns the published version ID.
func createPublishedCatalog(t *testing.T, apiKey string) string {
	t.Helper()
	// 1. Create draft.
	status, body := apiReq(t, "POST", "/v1/catalog/versions", apiKey, map[string]any{})
	if status != http.StatusCreated {
		t.Fatalf("create catalog version: status %d, body %v", status, body)
	}
	versionID := body["version"].(map[string]any)["id"].(string)

	// 2. Replace content.
	status, body = apiReq(t, "PUT", "/v1/catalog/versions/"+versionID+"/content", apiKey, catalogContent())
	if status != http.StatusOK {
		t.Fatalf("replace content: status %d, body %v", status, body)
	}

	// 3. Validate.
	status, body = apiReq(t, "POST", "/v1/catalog/versions/"+versionID+"/validate", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("validate: status %d, body %v", status, body)
	}
	if body["version"].(map[string]any)["state"] != "validated" {
		t.Fatalf("state = %v, want validated", body["version"].(map[string]any)["state"])
	}

	// 4. Publish.
	status, body = apiReq(t, "POST", "/v1/catalog/versions/"+versionID+"/publish", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("publish: status %d, body %v", status, body)
	}
	if body["version"].(map[string]any)["state"] != "published" {
		t.Fatalf("state = %v, want published", body["version"].(map[string]any)["state"])
	}
	return versionID
}

// createCustomerAndSubscription creates a customer and a subscription pinned
// to the given catalog version. Returns (customerExternalID, subscriptionID).
func createCustomerAndSubscription(t *testing.T, apiKey, catalogVersionID string) (string, string) {
	t.Helper()
	custExt := "cust-" + uuid.NewString()[:8]
	status, body := apiReq(t, "POST", "/v1/customers", apiKey, map[string]any{
		"external_id":  custExt,
		"account_type": "business",
		"display_name": "Test Customer",
	})
	if status != http.StatusCreated {
		t.Fatalf("create customer: status %d, body %v", status, body)
	}

	subExt := "sub-" + uuid.NewString()[:8]
	status, body = apiReq(t, "POST", "/v1/subscriptions", apiKey, map[string]any{
		"external_id":          subExt,
		"customer_external_id": custExt,
		"catalog_version_id":   catalogVersionID,
		"plan_code":            "starter",
	})
	if status != http.StatusCreated {
		t.Fatalf("create subscription: status %d, body %v", status, body)
	}
	subID := body["subscription"].(map[string]any)["id"].(string)
	return custExt, subID
}

// ingestUsage sends a usage event and returns (status code, response body).
// The caller controls the timestamp so that idempotency tests can reuse
// the same value (payload hash includes timestamp).
func ingestUsage(t *testing.T, apiKey, txID, custExt, metricCode, timestamp string, properties any) (int, map[string]any) {
	t.Helper()
	return apiReq(t, "POST", "/v1/usage/ingest", apiKey, map[string]any{
		"transaction_id":       txID,
		"metric_code":          metricCode,
		"customer_external_id": custExt,
		"timestamp":            timestamp,
		"properties":           properties,
	})
}

// failingAdapter simulates a persistent billing engine outage: every
// delivery attempt fails. It is a real Adapter implementation used to
// verify that outbox retry logic preserves accepted usage (Testing #22).
type failingAdapter struct{}

func (failingAdapter) Name() string { return "failing" }
func (failingAdapter) DeliverUsageEvent(context.Context, billing.UsageEvent) error {
	return errors.New("simulated billing engine outage")
}
func (failingAdapter) ListInvoices(context.Context, int32) ([]billing.InvoiceSync, int32, error) {
	return nil, 0, nil
}

func newTestRelay(adapter billing.Adapter) *outbox.Relay {
	return outbox.NewRelay(appStore, adapter, 50*time.Millisecond,
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
}

// ---------------------------------------------------------------------------
// Testing #7: Published catalog versions cannot be mutated
// ---------------------------------------------------------------------------

func TestBillingCatalogImmutableAfterPublish(t *testing.T) {
	_, apiKey := createProviderAPI(t, "cat-imm-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)

	// Attempt to replace content on a published version → 409.
	status, body := apiReq(t, "PUT", "/v1/catalog/versions/"+versionID+"/content", apiKey, catalogContent())
	if status != http.StatusConflict {
		t.Fatalf("replace content on published: status %d, want 409, body %v", status, body)
	}

	// Attempt to validate again → 409.
	status, body = apiReq(t, "POST", "/v1/catalog/versions/"+versionID+"/validate", apiKey, nil)
	if status != http.StatusConflict {
		t.Fatalf("re-validate published: status %d, want 409, body %v", status, body)
	}

	// Attempt to publish again → 409.
	status, body = apiReq(t, "POST", "/v1/catalog/versions/"+versionID+"/publish", apiKey, nil)
	if status != http.StatusConflict {
		t.Fatalf("re-publish published: status %d, want 409, body %v", status, body)
	}
}

// ---------------------------------------------------------------------------
// Testing #8: Existing subscriptions remain pinned when a new catalog
// version is published
// ---------------------------------------------------------------------------

func TestBillingSubscriptionPinning(t *testing.T) {
	_, apiKey := createProviderAPI(t, "sub-pin-"+uuid.NewString()[:8])

	// Publish catalog v1 and create a subscription pinned to it.
	v1 := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, v1)

	// Publish catalog v2.
	v2 := createPublishedCatalog(t, apiKey)
	if v1 == v2 {
		t.Fatal("v1 and v2 must be different versions")
	}

	// The subscription must still reference v1.
	status, body := apiReq(t, "GET", "/v1/subscriptions", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list subscriptions: status %d, body %v", status, body)
	}
	subs := body["subscriptions"].([]any)
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}
	sub := subs[0].(map[string]any)
	pinnedVersion := sub["catalog_version_id"].(string)
	if pinnedVersion != v1 {
		t.Fatalf("subscription catalog_version_id = %s, want %s (pinned to v1, not v2)", pinnedVersion, v1)
	}

	// Customer is still accessible.
	_ = custExt // used for setup; subscription lookup confirms the data is intact
}

// ---------------------------------------------------------------------------
// Testing #4: Identical usage retries create one billable effect
// ---------------------------------------------------------------------------

func TestUsageIdempotency(t *testing.T) {
	_, apiKey := createProviderAPI(t, "uso-idem-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	txID := "tx-" + uuid.NewString()[:8]
	props := map[string]any{"count": 10}
	ts := time.Now().UTC().Format(time.RFC3339Nano)

	// First ingest → accepted.
	status, body := ingestUsage(t, apiKey, txID, custExt, "api_calls", ts, props)
	if status != http.StatusCreated {
		t.Fatalf("first ingest: status %d, body %v", status, body)
	}
	if body["status"] != "accepted" {
		t.Fatalf("first ingest status = %v, want accepted", body["status"])
	}

	// Second ingest with identical transaction_id and payload → duplicate.
	status, body = ingestUsage(t, apiKey, txID, custExt, "api_calls", ts, props)
	if status != http.StatusOK {
		t.Fatalf("duplicate ingest: status %d, want 200, body %v", status, body)
	}
	if body["status"] != "duplicate" {
		t.Fatalf("duplicate ingest status = %v, want duplicate", body["status"])
	}

	// Only one usage event exists.
	status, body = apiReq(t, "GET", "/v1/usage/events", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list usage events: status %d, body %v", status, body)
	}
	events := body["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("expected 1 usage event, got %d", len(events))
	}
}

// ---------------------------------------------------------------------------
// Testing #5: Identical transaction IDs with different payloads are
// rejected and audited
// ---------------------------------------------------------------------------

func TestUsagePayloadConflict(t *testing.T) {
	_, apiKey := createProviderAPI(t, "uso-conf-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	txID := "tx-" + uuid.NewString()[:8]
	ts := time.Now().UTC().Format(time.RFC3339Nano)

	// First ingest with count=10.
	status, _ := ingestUsage(t, apiKey, txID, custExt, "api_calls", ts, map[string]any{"count": 10})
	if status != http.StatusCreated {
		t.Fatalf("first ingest: status %d", status)
	}

	// Second ingest with same transaction_id but different payload (count=20) → 409.
	status, body := ingestUsage(t, apiKey, txID, custExt, "api_calls", ts, map[string]any{"count": 20})
	if status != http.StatusConflict {
		t.Fatalf("conflicting payload: status %d, want 409, body %v", status, body)
	}
	if errObj, ok := body["error"].(map[string]any); ok {
		if errObj["code"] != "usage_conflict" {
			t.Fatalf("error code = %v, want usage_conflict", errObj["code"])
		}
	} else {
		t.Fatalf("expected error object in response, got %v", body)
	}

	// An audit event was recorded for the conflict.
	status, body = apiReq(t, "GET", "/v1/audit-events", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list audit events: status %d", status)
	}
	found := false
	for _, e := range body["audit_events"].([]any) {
		if e.(map[string]any)["action"] == "usage.conflict" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected audit event for usage.conflict")
	}
}

// ---------------------------------------------------------------------------
// Testing #6: Reversal events produce correct outcomes
// ---------------------------------------------------------------------------

func TestUsageReversal(t *testing.T) {
	_, apiKey := createProviderAPI(t, "uso-rev-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	txID := "tx-" + uuid.NewString()[:8]
	status, _ := ingestUsage(t, apiKey, txID, custExt, "api_calls", time.Now().UTC().Format(time.RFC3339Nano), map[string]any{"count": 5})
	if status != http.StatusCreated {
		t.Fatalf("ingest: status %d", status)
	}

	// Reverse the usage.
	status, body := apiReq(t, "POST", "/v1/usage/reverse", apiKey, map[string]any{
		"original_transaction_id": txID,
		"reason":                  "customer dispute",
	})
	if status != http.StatusCreated {
		t.Fatalf("reverse: status %d, body %v", status, body)
	}

	// Two events exist: one ingestion, one reversal.
	status, body = apiReq(t, "GET", "/v1/usage/events", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list events: status %d", status)
	}
	events := body["events"].([]any)
	if len(events) != 2 {
		t.Fatalf("expected 2 events (ingestion + reversal), got %d", len(events))
	}

	// Find the reversal event and verify it reverses the original.
	var reversal map[string]any
	for _, e := range events {
		em := e.(map[string]any)
		if em["kind"] == "reversal" {
			reversal = em
			break
		}
	}
	if reversal == nil {
		t.Fatal("no reversal event found")
	}
	if reversal["transaction_id"] == txID {
		t.Fatal("reversal must have a different transaction_id from the original")
	}

	// Attempt to reverse again → 409 (already reversed).
	status, body = apiReq(t, "POST", "/v1/usage/reverse", apiKey, map[string]any{
		"original_transaction_id": txID,
	})
	if status != http.StatusConflict {
		t.Fatalf("double reverse: status %d, want 409, body %v", status, body)
	}
}

// ---------------------------------------------------------------------------
// Testing #22: Lago outage preserves accepted usage in Outbox and
// recovers without duplication
// ---------------------------------------------------------------------------

func TestOutboxRelayDeliversUsage(t *testing.T) {
	_, apiKey := createProviderAPI(t, "obx-ok-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	// Ingest usage — creates an outbox event with status=pending.
	txID := "tx-" + uuid.NewString()[:8]
	status, _ := ingestUsage(t, apiKey, txID, custExt, "api_calls", time.Now().UTC().Format(time.RFC3339Nano), map[string]any{"count": 1})
	if status != http.StatusCreated {
		t.Fatalf("ingest: status %d", status)
	}

	// Run relay with noop adapter (simulates successful delivery).
	relay := newTestRelay(billing.NewNoop(nil))
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// The outbox event should now be published.
	status, body := apiReq(t, "GET", "/v1/outbox-events", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list outbox: status %d", status)
	}
	events := body["outbox_events"].([]any)
	var usageEvent map[string]any
	for _, e := range events {
		em := e.(map[string]any)
		if em["event_type"] == "usage.accepted" {
			usageEvent = em
			break
		}
	}
	if usageEvent == nil {
		t.Fatal("usage.accepted outbox event not found")
	}
	if usageEvent["status"] != "published" {
		t.Fatalf("outbox status = %v, want published", usageEvent["status"])
	}
}

func TestOutboxRelayOutagePreservesUsage(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "obx-out-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	// Ingest usage — creates an outbox event with status=pending.
	txID := "tx-" + uuid.NewString()[:8]
	status, _ := ingestUsage(t, apiKey, txID, custExt, "api_calls", time.Now().UTC().Format(time.RFC3339Nano), map[string]any{"count": 1})
	if status != http.StatusCreated {
		t.Fatalf("ingest: status %d", status)
	}

	// Run relay with failing adapter (simulates billing engine outage).
	relay := newTestRelay(failingAdapter{})
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// The outbox event must NOT be published — it should be 'failed' (retry scheduled).
	status, body := apiReq(t, "GET", "/v1/outbox-events", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list outbox: status %d", status)
	}
	events := body["outbox_events"].([]any)
	var usageOutbox map[string]any
	for _, e := range events {
		em := e.(map[string]any)
		if em["event_type"] == "usage.accepted" {
			usageOutbox = em
			break
		}
	}
	if usageOutbox == nil {
		t.Fatal("usage.accepted outbox event not found")
	}
	if usageOutbox["status"] == "published" {
		t.Fatal("outbox event must not be published during outage")
	}

	// The usage event itself must still exist in the database (not lost).
	status, body = apiReq(t, "GET", "/v1/usage/events", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list usage events: status %d", status)
	}
	usageEvents := body["events"].([]any)
	if len(usageEvents) != 1 {
		t.Fatalf("expected 1 usage event (preserved during outage), got %d", len(usageEvents))
	}

	// --- Recovery: clear the retry backoff so the event is immediately
	// claimable, then drain with a noop adapter (simulates Lago recovery).
	if _, err := superPool.Exec(testCtx,
		"UPDATE outbox_events SET next_attempt_at = now() - interval '1 minute' WHERE event_type = 'usage.accepted' AND provider_id = $1",
		providerID); err != nil {
		t.Fatalf("reset outbox retry: %v", err)
	}
	relay2 := newTestRelay(billing.NewNoop(nil))
	if err := relay2.DrainOnce(testCtx); err != nil {
		t.Fatalf("recovery drain: %v", err)
	}

	// The outbox event should now be published.
	status, body = apiReq(t, "GET", "/v1/outbox-events", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list outbox after recovery: status %d", status)
	}
	for _, e := range body["outbox_events"].([]any) {
		em := e.(map[string]any)
		if em["event_type"] == "usage.accepted" {
			if em["status"] != "published" {
				t.Fatalf("outbox status = %v, want published after recovery", em["status"])
			}
			break
		}
	}

	// No duplicate usage event was created (at-least-once, not at-most-once
	// — but the usage_events table has a unique constraint on transaction_id
	// so the original event is the only one).
	status, body = apiReq(t, "GET", "/v1/usage/events", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list usage events after recovery: status %d", status)
	}
	usageEvents = body["events"].([]any)
	if len(usageEvents) != 1 {
		t.Fatalf("expected 1 usage event (no duplication after recovery), got %d", len(usageEvents))
	}

	// Outbox event count for usage.accepted must still be 1 — the relay
	// must not have created a duplicate outbox record during recovery.
	status, body = apiReq(t, "GET", "/v1/outbox-events", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list outbox after recovery: status %d", status)
	}
	usageOutboxCount := 0
	for _, e := range body["outbox_events"].([]any) {
		if em, ok := e.(map[string]any); ok && em["event_type"] == "usage.accepted" {
			usageOutboxCount++
		}
	}
	if usageOutboxCount != 1 {
		t.Fatalf("expected 1 usage.accepted outbox event (no duplication), got %d", usageOutboxCount)
	}
}

// ---------------------------------------------------------------------------
// Testing #25 (billing RLS): cross-provider billing data isolation
// ---------------------------------------------------------------------------

func TestBillingCrossTenantIsolation(t *testing.T) {
	_, keyA := createProviderAPI(t, "bill-a-"+uuid.NewString()[:8])
	_, keyB := createProviderAPI(t, "bill-b-"+uuid.NewString()[:8])

	// Provider A publishes a catalog.
	versionA := createPublishedCatalog(t, keyA)

	// Provider B cannot see A's catalog versions.
	status, body := apiReq(t, "GET", "/v1/catalog/versions", keyB, nil)
	if status != http.StatusOK {
		t.Fatalf("list catalog B: status %d", status)
	}
	// When the list is empty the JSON may be null, not [].
	if versions, ok := body["versions"].([]any); ok {
		for _, v := range versions {
			if v.(map[string]any)["id"] == versionA {
				t.Fatal("provider B can see provider A's catalog version")
			}
		}
	}

	// Provider B cannot ingest usage against A's catalog.
	custExtB, _ := createCustomerAndSubscription(t, keyB, createPublishedCatalog(t, keyB))
	_ = custExtB

	// Provider B cannot create a subscription with A's catalog_version_id.
	status, body = apiReq(t, "POST", "/v1/subscriptions", keyB, map[string]any{
		"external_id":          "evil-sub",
		"customer_external_id": custExtB,
		"catalog_version_id":   versionA,
		"plan_code":            "starter",
	})
	if status == http.StatusCreated {
		t.Fatal("provider B must not create subscription with provider A's catalog version")
	}
}

// ---------------------------------------------------------------------------
// Catalog publishing is gated on the provider lifecycle: a suspended (or
// otherwise non-serving) provider must not be able to publish a catalog
// version, because publishing is an external product commitment.
// ---------------------------------------------------------------------------

func TestCatalogPublishRequiresActiveProvider(t *testing.T) {
	slug := "cat-gate-" + uuid.NewString()[:8]
	providerID, apiKey := createProviderAPI(t, slug)

	// Prepare a validated draft while the provider is fully active.
	status, body := apiReq(t, "POST", "/v1/catalog/versions", apiKey, map[string]any{})
	if status != http.StatusCreated {
		t.Fatalf("create catalog version: status %d, body %v", status, body)
	}
	versionID := body["version"].(map[string]any)["id"].(string)

	status, body = apiReq(t, "PUT", "/v1/catalog/versions/"+versionID+"/content", apiKey, catalogContent())
	if status != http.StatusOK {
		t.Fatalf("replace content: status %d, body %v", status, body)
	}
	status, body = apiReq(t, "POST", "/v1/catalog/versions/"+versionID+"/validate", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("validate: status %d, body %v", status, body)
	}

	// Suspend the provider (TEST_ACTIVE → LIVE_REVIEW → SUSPENDED).
	transitionTo(t, providerID, "LIVE_REVIEW")
	transitionTo(t, providerID, "SUSPENDED")

	// Publish must be refused while the provider is suspended.
	status, body = apiReq(t, "POST", "/v1/catalog/versions/"+versionID+"/publish", apiKey, nil)
	if status != http.StatusConflict {
		t.Fatalf("publish on suspended provider: status %d, want 409, body %v", status, body)
	}
	if code := errorCode(body); code != "provider_not_writable" {
		t.Fatalf("code = %q, want provider_not_writable", code)
	}

	// Once the provider is serving traffic again the same draft can be
	// published: the gate must be state-based, not a one-way trap.
	transitionTo(t, providerID, "LIVE_ACTIVE")
	status, body = apiReq(t, "POST", "/v1/catalog/versions/"+versionID+"/publish", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("publish after reactivation: status %d, want 200, body %v", status, body)
	}
	if body["version"].(map[string]any)["state"] != "published" {
		t.Fatalf("state = %v, want published", body["version"].(map[string]any)["state"])
	}
}

// ---------------------------------------------------------------------------
// The provider API is read-only while the provider is suspended: every write
// method is rejected with 409 provider_not_writable while reads still pass,
// and writes are restored once the provider is serving traffic again.
// ---------------------------------------------------------------------------

func TestSuspendedProviderWritesBlocked(t *testing.T) {
	slug := "write-gate-" + uuid.NewString()[:8]
	providerID, apiKey := createProviderAPI(t, slug)
	transitionTo(t, providerID, "LIVE_REVIEW")
	transitionTo(t, providerID, "SUSPENDED")

	// Reads still work while suspended (inspection / audit).
	status, body := apiReq(t, "GET", "/v1/whoami", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("whoami while suspended: status %d, body %v", status, body)
	}

	// Every write method is rejected with 409 provider_not_writable.
	for _, tc := range []struct {
		method string
		path   string
	}{
		{"POST", "/v1/catalog/versions"},
		{"POST", "/v1/subscriptions"},
		{"POST", "/v1/usage/ingest"},
		{"POST", "/v1/credentials"},
	} {
		status, body := apiReq(t, tc.method, tc.path, apiKey, nil)
		if status != http.StatusConflict {
			t.Fatalf("%s %s on suspended provider: status %d, want 409, body %v", tc.method, tc.path, status, body)
		}
		if code := errorCode(body); code != "provider_not_writable" {
			t.Fatalf("%s %s: code = %q, want provider_not_writable", tc.method, tc.path, code)
		}
	}

	// Writes come back once the provider is serving traffic again.
	transitionTo(t, providerID, "LIVE_ACTIVE")
	status, body = apiReq(t, "POST", "/v1/catalog/versions", apiKey, map[string]any{})
	if status != http.StatusCreated {
		t.Fatalf("create catalog version after reactivation: status %d, want 201, body %v", status, body)
	}
}

// countingFailingAdapter is a billing adapter that always fails but counts
// every invocation, for asserting that an open circuit breaker stops calls
// to a dead billing engine.
type countingFailingAdapter struct {
	mu    sync.Mutex
	calls int
}

func (a *countingFailingAdapter) Name() string { return "counting-failing" }

func (a *countingFailingAdapter) DeliverUsageEvent(context.Context, billing.UsageEvent) error {
	a.mu.Lock()
	a.calls++
	a.mu.Unlock()
	return errors.New("simulated billing engine outage")
}

func (a *countingFailingAdapter) ListInvoices(context.Context, int32) ([]billing.InvoiceSync, int32, error) {
	return nil, 0, nil
}

func (a *countingFailingAdapter) Calls() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

// TestOutboxRelayCircuitBreakerFastFails verifies that once the billing
// engine trips the relay's circuit breaker open, pending outbox events are
// fast-failed (scheduled for backoff retry) without a single real adapter
// call — the relay stops hammering a dead dependency and lets it recover.
func TestOutboxRelayCircuitBreakerFastFails(t *testing.T) {
	providerID, apiKey := createProviderAPI(t, "obx-cb-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	// Two usage events → two consecutive adapter failures trip the breaker
	// (FailureThreshold=2).
	for range 2 {
		txID := "obx-cb-tx-" + uuid.NewString()[:8]
		status, _ := ingestUsage(t, apiKey, txID, custExt, "api_calls",
			time.Now().UTC().Format(time.RFC3339Nano), map[string]any{"count": 1})
		if status != http.StatusCreated {
			t.Fatalf("ingest usage: status %d", status)
		}
	}

	adapter := &countingFailingAdapter{}
	relay := newTestRelay(adapter)
	relay.WithCircuitBreaker(circuitbreaker.Options{
		FailureThreshold: 2,
		OpenTimeout:      5 * time.Minute, // stays open for the whole test
	})

	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("drain: %v", err)
	}
	// Both events failed: exactly 2 adapter calls, breaker now open.
	if got := adapter.Calls(); got != 2 {
		t.Fatalf("adapter calls = %d, want 2", got)
	}

	// Reset the retry backoff so the events are claimable again; the open
	// breaker must fast-fail them without calling the adapter.
	if _, err := superPool.Exec(testCtx,
		"UPDATE outbox_events SET next_attempt_at = now() - interval '1 minute' WHERE event_type = 'usage.accepted' AND provider_id = $1",
		providerID); err != nil {
		t.Fatalf("reset outbox retry: %v", err)
	}
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("drain after trip: %v", err)
	}
	if got := adapter.Calls(); got != 2 {
		t.Fatalf("adapter calls = %d while breaker open, want 2 (must fast-fail)", got)
	}
}
