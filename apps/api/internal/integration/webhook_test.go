package integration

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/billing"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/circuitbreaker"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/outbox"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/webhook"
	"github.com/google/uuid"
)

// newTestWebhookWorker builds a webhook worker using the shared test store.
func newTestWebhookWorker() *webhook.Worker {
	wk := webhook.NewWorker(appStore,
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		50*time.Millisecond)
	wk.SetURLValidator(webhook.ValidateURLAllowLoopback) // allow localhost for httptest
	return wk
}

// drainWebhookUntilDelivered runs the worker DrainOnce in a loop until at
// least one delivery record exists for the given API key, or the deadline
// expires. This handles the case where published outbox events from earlier
// tests are processed first (FindUndeliveredOutboxEvents is batched and
// ordered by created_at, so older events are scanned before newer ones).
func drainWebhookUntilDelivered(t *testing.T, apiKey string) {
	t.Helper()
	worker := newTestWebhookWorker()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if err := worker.DrainOnce(testCtx); err != nil {
			t.Fatalf("webhook drain: %v", err)
		}
		_, body := apiReq(t, "GET", "/v1/webhook-deliveries", apiKey, nil)
		if ds, ok := body["deliveries"].([]any); ok && len(ds) > 0 {
			return
		}
	}
	t.Fatal("timed out waiting for webhook delivery")
}

// cleanupWebhookData deletes all outbox events and webhook deliveries
// from prior tests using the superuser connection (bypasses RLS). This
// prevents stale events from interfering with webhook test assertions.
func cleanupWebhookData(t *testing.T) {
	t.Helper()
	_, err := superPool.Exec(testCtx, "DELETE FROM webhook_deliveries")
	if err != nil {
		t.Logf("cleanup webhook_deliveries: %v", err)
	}
	_, err = superPool.Exec(testCtx, "DELETE FROM outbox_events")
	if err != nil {
		t.Logf("cleanup outbox_events: %v", err)
	}
}

// drainAllPendingOutbox processes all accumulated outbox events from prior
// tests so the webhook worker queue is empty before a new webhook test
// starts. Without this, old events (pointing to dead httptest servers)
// dominate DrainOnce and cause timeouts.
func drainAllPendingOutbox(t *testing.T) {
	t.Helper()
	relay := outbox.NewRelay(appStore, billing.NewNoop(nil), 50*time.Millisecond,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	worker := newTestWebhookWorker()
	// Run enough iterations to dead-letter all stale deliveries from
	// prior tests. Each DrainOnce processes a batch; dead endpoints
	// fail fast (connection refused) and get dead-lettered after 3
	// attempts with short backoff.
	for i := 0; i < 20; i++ {
		_ = relay.DrainOnce(testCtx)
		_ = worker.DrainOnce(testCtx)
		time.Sleep(200 * time.Millisecond)
	}
}

// capturedRequest holds the headers and body of a webhook received by a
// test HTTP server, for assertion.
type capturedRequest struct {
	body      []byte
	signature string
	timestamp string
	eventType string
}

// createWebhookViaAPI creates a webhook endpoint pointing to the given URL
// via the HTTP API. Returns the endpoint's secret.
func createWebhookViaAPI(t *testing.T, apiKey, targetURL string) (endpointID, secret string) {
	t.Helper()
	status, body := apiReq(t, "POST", "/v1/webhooks", apiKey, map[string]any{
		"url":    targetURL,
		"events": []string{},
	})
	if status != http.StatusCreated {
		t.Fatalf("create webhook: status %d, body %v", status, body)
	}
	ep := body["endpoint"].(map[string]any)
	return ep["id"].(string), ep["secret"].(string)
}

// ---------------------------------------------------------------------------
// Testing: Webhook delivery end-to-end
// ---------------------------------------------------------------------------

func TestWebhookDelivery(t *testing.T) {
	cleanupWebhookData(t)

	_, apiKey := createProviderAPI(t, "wh-deliver-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	// Start a test HTTP server to receive webhooks.
	var captured []capturedRequest
	var capMu sync.Mutex
	wc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capMu.Lock()
		captured = append(captured, capturedRequest{
			body:      body,
			signature: r.Header.Get("X-Webhook-Signature"),
			timestamp: r.Header.Get("X-Webhook-Timestamp"),
			eventType: r.Header.Get("X-Webhook-Event-Type"),
		})
		capMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer wc.Close()

	endpointID, secret := createWebhookViaAPI(t, apiKey, wc.URL)

	// Ingest usage → triggers outbox event.
	txID := "wh-tx-" + uuid.NewString()[:8]
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	status, body := ingestUsage(t, apiKey, txID, custExt, "api_calls", ts, map[string]any{"count": 5})
	if status != http.StatusCreated {
		t.Fatalf("ingest usage: status %d, body %v", status, body)
	}

	// Run the outbox relay to mark events published.
	relay := outbox.NewRelay(appStore, billing.NewNoop(nil), 50*time.Millisecond,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("relay drain: %v", err)
	}

	// Run the webhook worker to create deliveries + deliver.
	drainWebhookUntilDelivered(t, apiKey)

	// The test server should have received the webhook.
	capMu.Lock()
	if len(captured) == 0 {
		t.Fatal("expected at least 1 webhook received, got 0")
	}
	capMu.Unlock()

	// Verify HMAC signature is correct.
	capMu.Lock()
	cr := captured[0]
	capMu.Unlock()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(cr.timestamp))
	mac.Write(cr.body)
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(cr.signature), []byte(expectedSig)) {
		t.Fatalf("HMAC signature mismatch: got %s, want %s", cr.signature, expectedSig)
	}

	// Verify delivery status = 'delivered'.
	status, body = apiReq(t, "GET", "/v1/webhook-deliveries", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list deliveries: status %d, body %v", status, body)
	}
	deliveries, ok := body["deliveries"].([]any)
	if !ok || len(deliveries) == 0 {
		t.Fatal("expected at least 1 delivery record")
	}
	d := deliveries[0].(map[string]any)
	if d["status"] != "delivered" {
		t.Fatalf("delivery status = %v, want delivered", d["status"])
	}
	if d["endpoint_id"] != endpointID {
		t.Fatalf("endpoint_id = %v, want %s", d["endpoint_id"], endpointID)
	}
}

// TestWebhookDeliveryLifecycleAware verifies that outbound webhook delivery is
// lifecycle-aware (design baseline §7.4):
//
//  1. While a provider is SUSPENDED, worker runs create delivery rows but never
//     claim/send them — the backlog stays 'pending' and no HTTP call is made.
//  2. After reactivation (SUSPENDED -> LIVE_ACTIVE) the backlog is delivered
//     automatically without any manual replay.
//  3. RESTRICTED keeps delivering: it is a limited but operational state, so
//     deliveries must still be claimed and sent.
func TestWebhookDeliveryLifecycleAware(t *testing.T) {
	cleanupWebhookData(t)

	providerID, apiKey := createProviderAPI(t, "wh-lifecycle-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	var capMu sync.Mutex
	received := 0
	wc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capMu.Lock()
		received++
		capMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer wc.Close()

	createWebhookViaAPI(t, apiKey, wc.URL)

	// Ingest usage while TEST_ACTIVE (writable) so the outbox event exists
	// before the provider is suspended.
	txID := "wh-lc-tx-" + uuid.NewString()[:8]
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	status, body := ingestUsage(t, apiKey, txID, custExt, "api_calls", ts, map[string]any{"count": 1})
	if status != http.StatusCreated {
		t.Fatalf("ingest usage: status %d, body %v", status, body)
	}

	// Suspend the provider: TEST_ACTIVE -> LIVE_REVIEW -> SUSPENDED.
	for _, to := range []string{"LIVE_REVIEW", "SUSPENDED"} {
		status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle", operatorToken,
			map[string]any{"to": to, "reason": "abuse investigation", "actor": "op-lifecycle-test"})
		if status != http.StatusOK {
			t.Fatalf("transition to %s: status %d, body %v", to, status, body)
		}
	}

	// Publish the outbox event while suspended (the relay is lifecycle-agnostic).
	relay := outbox.NewRelay(appStore, billing.NewNoop(nil), 50*time.Millisecond,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("relay drain: %v", err)
	}

	// Run the worker repeatedly while suspended: a delivery row is created but
	// must never be claimed/sent.
	worker := newTestWebhookWorker()
	for range 3 {
		if err := worker.DrainOnce(testCtx); err != nil {
			t.Fatalf("drain while suspended: %v", err)
		}
	}
	capMu.Lock()
	if received != 0 {
		t.Fatalf("webhook delivered while suspended: %d requests", received)
	}
	capMu.Unlock()

	// The delivery row exists and is parked as 'pending' (backlog).
	status, body = apiReq(t, "GET", "/v1/webhook-deliveries", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list deliveries (suspended): status %d, body %v", status, body)
	}
	deliveries, ok := body["deliveries"].([]any)
	if !ok || len(deliveries) == 0 {
		t.Fatal("expected a delivery record while suspended")
	}
	if d := deliveries[0].(map[string]any); d["status"] != "pending" {
		t.Fatalf("delivery status = %v, want pending while suspended", d["status"])
	}

	// Reactivate: SUSPENDED -> LIVE_ACTIVE.
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle", operatorToken,
		map[string]any{"to": "LIVE_ACTIVE", "reason": "investigation cleared", "actor": "op-lifecycle-test"})
	if status != http.StatusOK {
		t.Fatalf("reactivate: status %d, body %v", status, body)
	}

	// After reactivation the backlog is delivered automatically.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := worker.DrainOnce(testCtx); err != nil {
			t.Fatalf("drain after reactivation: %v", err)
		}
		capMu.Lock()
		n := received
		capMu.Unlock()
		if n > 0 {
			break
		}
	}
	capMu.Lock()
	if received == 0 {
		t.Fatal("expected delivery after reactivation, got 0")
	}
	capMu.Unlock()

	status, body = apiReq(t, "GET", "/v1/webhook-deliveries", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list deliveries (reactivated): status %d, body %v", status, body)
	}
	deliveries, ok = body["deliveries"].([]any)
	if !ok || len(deliveries) == 0 {
		t.Fatal("expected a delivery record after reactivation")
	}
	if d := deliveries[0].(map[string]any); d["status"] != "delivered" {
		t.Fatalf("delivery status = %v, want delivered after reactivation", d["status"])
	}

	// RESTRICTED must keep delivering: ingest a second event, move to
	// RESTRICTED, and assert the webhook still goes out.
	txID2 := "wh-lc-tx-" + uuid.NewString()[:8]
	ts2 := time.Now().UTC().Format(time.RFC3339Nano)
	status, body = ingestUsage(t, apiKey, txID2, custExt, "api_calls", ts2, map[string]any{"count": 2})
	if status != http.StatusCreated {
		t.Fatalf("ingest usage (2): status %d, body %v", status, body)
	}
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/lifecycle", operatorToken,
		map[string]any{"to": "RESTRICTED", "reason": "billing review", "actor": "op-lifecycle-test"})
	if status != http.StatusOK {
		t.Fatalf("transition to RESTRICTED: status %d, body %v", status, body)
	}
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("relay drain (restricted): %v", err)
	}
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := worker.DrainOnce(testCtx); err != nil {
			t.Fatalf("drain while restricted: %v", err)
		}
		capMu.Lock()
		n := received
		capMu.Unlock()
		if n >= 2 {
			break
		}
	}
	capMu.Lock()
	if received < 2 {
		t.Fatalf("expected delivery while RESTRICTED, received = %d, want >= 2", received)
	}
	capMu.Unlock()
}

// ---------------------------------------------------------------------------
// Testing: SSRF prevention
// ---------------------------------------------------------------------------

func TestWebhookSSRFPrevention(t *testing.T) {
	// Test the production-grade SSRF validator directly (the HTTP API path
	// uses the AllowLoopback variant so httptest servers work; here we
	// verify the strict validator blocks all private ranges).
	privateURLs := []string{
		"http://127.0.0.1:8080/webhook",
		"http://10.0.0.1/webhook",
		"http://192.168.1.1/webhook",
		"http://169.254.169.254/latest/meta-data/",
		"http://172.16.0.1/webhook",
	}
	for _, u := range privateURLs {
		if err := webhook.ValidateURL(u); err == nil {
			t.Fatalf("SSRF URL %q: expected error, got nil", u)
		}
	}

	// The AllowLoopback variant (used in tests) must still block non-loopback
	// private IPs (cloud metadata, RFC 1918).
	nonLoopbackPrivate := []string{
		"http://10.0.0.1/webhook",
		"http://192.168.1.1/webhook",
		"http://169.254.169.254/latest/meta-data/",
		"http://172.16.0.1/webhook",
	}
	for _, u := range nonLoopbackPrivate {
		if err := webhook.ValidateURLAllowLoopback(u); err == nil {
			t.Fatalf("AllowLoopback should still block %q, got nil", u)
		}
	}
}

// ---------------------------------------------------------------------------
// Testing: Webhook retry on failure
// ---------------------------------------------------------------------------

func TestWebhookRetryOnFailure(t *testing.T) {
	_, apiKey := createProviderAPI(t, "wh-retry-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	// Server that always returns 500.
	wc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer wc.Close()

	createWebhookViaAPI(t, apiKey, wc.URL)

	// Ingest usage → outbox event.
	txID := "wh-retry-tx-" + uuid.NewString()[:8]
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	status, _ := ingestUsage(t, apiKey, txID, custExt, "api_calls", ts, map[string]any{"count": 1})
	if status != http.StatusCreated {
		t.Fatalf("ingest usage: status %d", status)
	}

	// Mark published.
	relay := outbox.NewRelay(appStore, billing.NewNoop(nil), 50*time.Millisecond,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("relay drain: %v", err)
	}

	worker := newTestWebhookWorker()

	// First drain: creates deliveries + first attempt → failed (retry).
	drainWebhookUntilDelivered(t, apiKey)
	// Run once more to ensure all pending deliveries are claimed.
	_ = worker.DrainOnce(testCtx)
	if d := latestDelivery(t, apiKey); d["status"] != "failed" {
		t.Fatalf("after first attempt: status = %v, want failed", d["status"])
	}

	// Manually reset next_attempt_at so the next drain can re-claim without
	// waiting for the backoff. We use the operator (super) pool directly.
	_, err := superPool.Exec(context.Background(),
		`UPDATE webhook_deliveries SET next_attempt_at = now() WHERE status = 'failed'`)
	if err != nil {
		t.Fatalf("reset next_attempt_at: %v", err)
	}

	// Second drain: attempt 2 → failed (retry).
	if err := worker.DrainOnce(testCtx); err != nil {
		t.Fatalf("second drain: %v", err)
	}

	// Reset again for the third attempt.
	_, err = superPool.Exec(context.Background(),
		`UPDATE webhook_deliveries SET next_attempt_at = now() WHERE status = 'failed'`)
	if err != nil {
		t.Fatalf("reset next_attempt_at (2): %v", err)
	}

	// Third drain: attempt 3 → dead_letter (max attempts).
	if err := worker.DrainOnce(testCtx); err != nil {
		t.Fatalf("third drain: %v", err)
	}
	if d := latestDelivery(t, apiKey); d["status"] != "dead_letter" {
		t.Fatalf("after third attempt: status = %v, want dead_letter", d["status"])
	}
}

// latestDelivery returns the most recent webhook delivery for the caller's
// tenant.
func latestDelivery(t *testing.T, apiKey string) map[string]any {
	t.Helper()
	status, body := apiReq(t, "GET", "/v1/webhook-deliveries", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list deliveries: status %d, body %v", status, body)
	}
	deliveries, ok := body["deliveries"].([]any)
	if !ok || len(deliveries) == 0 {
		t.Fatal("expected at least 1 delivery record")
	}
	return deliveries[0].(map[string]any)
}

// ---------------------------------------------------------------------------
// Testing: Webhook dedup
// ---------------------------------------------------------------------------

func TestWebhookDedup(t *testing.T) {
	cleanupWebhookData(t)
	_, apiKey := createProviderAPI(t, "wh-dedup-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	wc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer wc.Close()

	createWebhookViaAPI(t, apiKey, wc.URL)

	txID := "wh-dedup-tx-" + uuid.NewString()[:8]
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	status, _ := ingestUsage(t, apiKey, txID, custExt, "api_calls", ts, map[string]any{"count": 1})
	if status != http.StatusCreated {
		t.Fatalf("ingest usage: status %d", status)
	}

	relay := outbox.NewRelay(appStore, billing.NewNoop(nil), 50*time.Millisecond,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("relay drain: %v", err)
	}

	// First worker drain: creates deliveries + delivers them.
	drainWebhookUntilDelivered(t, apiKey)

	// Count deliveries after first drain.
	_, body := apiReq(t, "GET", "/v1/webhook-deliveries", apiKey, nil)
	before := 0
	if ds, ok := body["deliveries"].([]any); ok {
		before = len(ds)
	}
	if before == 0 {
		t.Fatal("expected deliveries after first drain, got 0")
	}

	// Second worker drain: should NOT create any new deliveries (dedup
	// via UNIQUE(endpoint_id, outbox_event_id) + ON CONFLICT DO NOTHING).
	worker := newTestWebhookWorker()
	if err := worker.DrainOnce(testCtx); err != nil {
		t.Fatalf("second drain: %v", err)
	}

	_, body = apiReq(t, "GET", "/v1/webhook-deliveries", apiKey, nil)
	after := 0
	if ds, ok := body["deliveries"].([]any); ok {
		after = len(ds)
	}
	if after != before {
		t.Fatalf("dedup failed: deliveries before=%d, after=%d", before, after)
	}
}

// ---------------------------------------------------------------------------
// Testing: Webhook CRUD
// ---------------------------------------------------------------------------

func TestWebhookCRUD(t *testing.T) {
	_, apiKey := createProviderAPI(t, "wh-crud-"+uuid.NewString()[:8])

	wc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer wc.Close()

	// Create.
	endpointID, secret := createWebhookViaAPI(t, apiKey, wc.URL)
	if secret == "" {
		t.Fatal("expected non-empty secret to be generated")
	}

	// List.
	status, body := apiReq(t, "GET", "/v1/webhooks", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list webhooks: status %d, body %v", status, body)
	}
	endpoints, ok := body["endpoints"].([]any)
	if !ok || len(endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(endpoints))
	}
	if endpoints[0].(map[string]any)["id"] != endpointID {
		t.Fatal("endpoint id mismatch")
	}

	// Delete.
	status, _ = apiReq(t, "DELETE", "/v1/webhooks/"+endpointID, apiKey, nil)
	if status != http.StatusNoContent {
		t.Fatalf("delete webhook: status %d, want 204", status)
	}

	// List again → empty.
	status, body = apiReq(t, "GET", "/v1/webhooks", apiKey, nil)
	if status != http.StatusOK {
		t.Fatalf("list webhooks after delete: status %d", status)
	}
	endpoints, ok = body["endpoints"].([]any)
	if ok && len(endpoints) != 0 {
		t.Fatalf("expected 0 endpoints after delete, got %d", len(endpoints))
	}
}

// ---------------------------------------------------------------------------
// Testing: Webhook event filter
// ---------------------------------------------------------------------------

func TestWebhookEventFilter(t *testing.T) {
	cleanupWebhookData(t)
	_, apiKey := createProviderAPI(t, "wh-filter-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	var capMu sync.Mutex
	var received []capturedRequest
	wc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capMu.Lock()
		received = append(received, capturedRequest{
			body:      body,
			eventType: r.Header.Get("X-Webhook-Event-Type"),
		})
		capMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer wc.Close()

	// Create endpoint filtered to only "usage.accepted" events.
	status, body := apiReq(t, "POST", "/v1/webhooks", apiKey, map[string]any{
		"url":    wc.URL,
		"events": []string{"usage.accepted"},
	})
	if status != http.StatusCreated {
		t.Fatalf("create filtered webhook: status %d, body %v", status, body)
	}

	// Ingest usage (event type = usage.accepted).
	txID := "wh-filter-tx-" + uuid.NewString()[:8]
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	status, _ = ingestUsage(t, apiKey, txID, custExt, "api_calls", ts, map[string]any{"count": 1})
	if status != http.StatusCreated {
		t.Fatalf("ingest usage: status %d", status)
	}

	relay := outbox.NewRelay(appStore, billing.NewNoop(nil), 50*time.Millisecond,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("relay drain: %v", err)
	}

	// Run the webhook worker until the filtered delivery arrives.
	worker := newTestWebhookWorker()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := worker.DrainOnce(testCtx); err != nil {
			t.Fatalf("webhook drain: %v", err)
		}
		capMu.Lock()
		got := len(received)
		capMu.Unlock()
		if got > 0 {
			break
		}
	}

	capMu.Lock()
	if len(received) == 0 {
		t.Fatal("expected webhook delivery for usage.accepted (matching filter)")
	}
	capMu.Unlock()
}

// ---------------------------------------------------------------------------
// Testing: Webhook dead-letter replay (operator loop)
// ---------------------------------------------------------------------------

// TestWebhookDeliveryReplay verifies the dead-letter replay loop: a delivery
// that exhausts its retry budget lands in dead_letter, the operator can
// requeue it via POST /v1/operator/providers/{id}/webhook-deliveries/{id}/replay,
// and the worker redelivers it with a fresh attempt budget. Non-terminal
// deliveries are rejected with replay_invalid_state, and the replay is
// written to the provider's audit trail.
func TestWebhookDeliveryReplay(t *testing.T) {
	cleanupWebhookData(t)

	providerID, apiKey := createProviderAPI(t, "wh-replay-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	// Endpoint that fails until flipped healthy, forcing the delivery into
	// dead_letter first.
	var healthy atomic.Bool
	wc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy.Load() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer wc.Close()

	createWebhookViaAPI(t, apiKey, wc.URL)

	txID := "wh-replay-tx-" + uuid.NewString()[:8]
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	status, body := ingestUsage(t, apiKey, txID, custExt, "api_calls", ts, map[string]any{"count": 1})
	if status != http.StatusCreated {
		t.Fatalf("ingest usage: status %d, body %v", status, body)
	}

	relay := outbox.NewRelay(appStore, billing.NewNoop(nil), 50*time.Millisecond,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("relay drain: %v", err)
	}

	// Exhaust the retry budget (3 attempts): drain, then reset the backoff so
	// the next drain can re-claim immediately (same acceleration as
	// TestWebhookRetryBackoff).
	// The setup helpers above also enqueue non-usage outbox events (customer,
	// subscription, catalog), so the worker will create a delivery for each.
	// The breaker is left effectively inert (a very high threshold) because
	// this test exercises the replay loop, not circuit-breaking.
	worker := newTestWebhookWorker()
	worker.SetBreakerOptions(circuitbreaker.Options{FailureThreshold: 1000})
	for range 3 {
		if err := worker.DrainOnce(testCtx); err != nil {
			t.Fatalf("drain: %v", err)
		}
		if _, err := superPool.Exec(context.Background(),
			`UPDATE webhook_deliveries SET next_attempt_at = now() WHERE status = 'failed'`); err != nil {
			t.Fatalf("reset next_attempt_at: %v", err)
		}
	}
	if err := worker.DrainOnce(testCtx); err != nil {
		t.Fatalf("final drain: %v", err)
	}
	d := operatorLatestDelivery(t, providerID)
	if d["status"] != "dead_letter" {
		t.Fatalf("status = %v, want dead_letter", d["status"])
	}
	deliveryID := d["id"].(string)

	// Replay via the operator API → reset to pending with a fresh budget.
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/webhook-deliveries/"+deliveryID+"/replay",
		operatorToken, map[string]any{"actor": "op-replay-test"})
	if status != http.StatusOK {
		t.Fatalf("replay: status %d, body %v", status, body)
	}
	rd := body["delivery"].(map[string]any)
	if rd["status"] != "pending" {
		t.Fatalf("replayed status = %v, want pending", rd["status"])
	}
	if n := rd["attempts"].(float64); n != 0 {
		t.Fatalf("replayed attempts = %v, want 0", n)
	}

	// Replaying a non-terminal (now pending) delivery must be rejected.
	status, body = apiReq(t, "POST", "/v1/operator/providers/"+providerID+"/webhook-deliveries/"+deliveryID+"/replay",
		operatorToken, nil)
	if status != http.StatusConflict {
		t.Fatalf("replay while pending: status %d, want 409, body %v", status, body)
	}
	if ec := body["error"].(map[string]any)["code"]; ec != "replay_invalid_state" {
		t.Fatalf("replay while pending error code = %v, want replay_invalid_state", ec)
	}

	// Flip the endpoint healthy and drain: the replayed delivery delivers.
	healthy.Store(true)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if err := worker.DrainOnce(testCtx); err != nil {
			t.Fatalf("drain after replay: %v", err)
		}
		if operatorDeliveryStatus(t, providerID, deliveryID) == "delivered" {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if s := operatorDeliveryStatus(t, providerID, deliveryID); s != "delivered" {
		t.Fatalf("delivery status after replay = %v, want delivered", s)
	}

	// The replay is recorded in the provider's audit trail.
	status, body = apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/audit", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("audit list: status %d, body %v", status, body)
	}
	found := false
	for _, e := range body["audit_events"].([]any) {
		em := e.(map[string]any)
		if em["action"] == "webhook_delivery_replay" && em["target_id"] == deliveryID {
			found = true
		}
	}
	if !found {
		t.Fatal("replay must produce a webhook_delivery_replay audit record")
	}
}

// operatorLatestDelivery returns the most recent webhook delivery for a
// provider via the operator (cross-environment) view.
func operatorLatestDelivery(t *testing.T, providerID string) map[string]any {
	t.Helper()
	status, body := apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/webhook-deliveries", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list deliveries: status %d, body %v", status, body)
	}
	deliveries, ok := body["deliveries"].([]any)
	if !ok || len(deliveries) == 0 {
		t.Fatal("expected at least 1 delivery record")
	}
	return deliveries[0].(map[string]any)
}

func operatorDeliveryStatus(t *testing.T, providerID, deliveryID string) string {
	t.Helper()
	status, body := apiReq(t, "GET", "/v1/operator/providers/"+providerID+"/webhook-deliveries", operatorToken, nil)
	if status != http.StatusOK {
		t.Fatalf("list deliveries: status %d, body %v", status, body)
	}
	for _, e := range body["deliveries"].([]any) {
		d := e.(map[string]any)
		if d["id"] == deliveryID {
			return d["status"].(string)
		}
	}
	t.Fatalf("delivery %s not found", deliveryID)
	return ""
}

// ---------------------------------------------------------------------------
// Testing: Webhook retention sweep
// ---------------------------------------------------------------------------

// TestWebhookRetentionPurge verifies the retention sweeper deletes only
// terminal rows older than the cutoff: delivered/dead_letter deliveries,
// failed deliveries with retries exhausted (next_attempt_at NULL), and
// published or dead-lettered outbox events. Pending rows, failed rows still
// inside their retry window, and fresh rows are never purged.
func TestWebhookRetentionPurge(t *testing.T) {
	cleanupWebhookData(t)

	providerID, apiKey := createProviderAPI(t, "wh-ret-"+uuid.NewString()[:8])
	envID := getTestEnvID(t, providerID)
	endpointID, _ := createWebhookViaAPI(t, apiKey, "http://127.0.0.1:9/never")

	old := "now() - interval '40 days'"
	future := "now() + interval '1 hour'"

	// Terminal deliveries older than the cutoff — all must be purged.
	insertDelivery := func(status string, createdAt string, nextAttemptAt string) string {
		id := uuid.NewString()
		_, err := superPool.Exec(testCtx, `
			INSERT INTO webhook_deliveries
				(id, endpoint_id, outbox_event_id, provider_id, environment_id, status, created_at, next_attempt_at)
			VALUES ($1, $2, $3, $4, $5, $6, `+createdAt+`, `+nextAttemptAt+`)`,
			id, endpointID, uuid.NewString(), providerID, envID, status)
		if err != nil {
			t.Fatalf("insert delivery %s: %v", status, err)
		}
		return id
	}
	insertDelivery("delivered", old, "NULL")
	insertDelivery("dead_letter", old, "NULL")
	insertDelivery("failed", old, "NULL") // retries exhausted → purge

	// Must survive: failed but still inside its retry window.
	insertDelivery("failed", old, future)
	// Must survive: fresh terminal row.
	insertDelivery("delivered", "now()", "NULL")
	// Must survive: pending row (never purged regardless of age).
	insertDelivery("pending", old, "NULL")

	// Outbox events. transaction_id must be unique per provider+environment.
	insertOutbox := func(status string, createdAt string, nextAttemptAt string) {
		_, err := superPool.Exec(testCtx, `
			INSERT INTO outbox_events
				(provider_id, environment_id, aggregate_type, aggregate_id, event_type,
				 payload, payload_hash, transaction_id, status, created_at, next_attempt_at)
			VALUES ($1, $2, 'usage', 'agg-1', 'usage.recorded', '{}', 'h', $3, $4, `+createdAt+`, `+nextAttemptAt+`)`,
			providerID, envID, uuid.NewString(), status)
		if err != nil {
			t.Fatalf("insert outbox %s: %v", status, err)
		}
	}
	insertOutbox("published", old, "NULL")     // terminal → purge
	insertOutbox("failed", old, "NULL")        // retries exhausted → purge
	insertOutbox("failed", old, future)        // still retrying → survive
	insertOutbox("published", "now()", "NULL") // fresh → survive

	// Run the sweep with a 30-day retention window.
	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	n, err := svc.PurgeExpiredWebhookDeliveries(testCtx, cutoff)
	if err != nil {
		t.Fatalf("PurgeExpiredWebhookDeliveries: %v", err)
	}
	if n != 5 { // 3 deliveries + 2 outbox events
		t.Fatalf("purged %d rows, want 5", n)
	}

	var deliveries, oldOutbox, freshPublished int
	if err := superPool.QueryRow(testCtx,
		`SELECT count(*) FROM webhook_deliveries WHERE provider_id = $1`, providerID).Scan(&deliveries); err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	if deliveries != 3 { // failed-retry + fresh delivered + pending
		t.Errorf("remaining deliveries = %d, want 3", deliveries)
	}
	// Old outbox rows: only the failed-but-still-retrying row survives.
	if err := superPool.QueryRow(testCtx,
		`SELECT count(*) FROM outbox_events WHERE provider_id = $1 AND created_at < $2`,
		providerID, cutoff).Scan(&oldOutbox); err != nil {
		t.Fatalf("count old outbox: %v", err)
	}
	if oldOutbox != 1 {
		t.Errorf("remaining old outbox events = %d, want 1 (failed, still retrying)", oldOutbox)
	}
	// Fresh published rows are never purged.
	if err := superPool.QueryRow(testCtx,
		`SELECT count(*) FROM outbox_events WHERE provider_id = $1 AND status = 'published' AND created_at >= $2`,
		providerID, cutoff).Scan(&freshPublished); err != nil {
		t.Fatalf("count fresh published outbox: %v", err)
	}
	if freshPublished < 1 {
		t.Errorf("fresh published outbox events = %d, want >= 1 (never purged)", freshPublished)
	}

	// A second sweep is a no-op (idempotent).
	n2, err := svc.PurgeExpiredWebhookDeliveries(testCtx, cutoff)
	if err != nil {
		t.Fatalf("second PurgeExpiredWebhookDeliveries: %v", err)
	}
	if n2 != 0 {
		t.Errorf("second sweep purged %d rows, want 0 (idempotent)", n2)
	}
}

// TestWebhookCircuitBreakerTripsAndFastFails verifies the per-endpoint
// circuit breaker: a webhook endpoint that keeps returning 500s is hit
// exactly FailureThreshold times, then every subsequent delivery is
// fast-failed (scheduled for backoff retry) without any real HTTP call —
// the worker stops hammering a dead endpoint.
func TestWebhookCircuitBreakerTripsAndFastFails(t *testing.T) {
	cleanupWebhookData(t)

	_, apiKey := createProviderAPI(t, "wh-cb-"+uuid.NewString()[:8])
	versionID := createPublishedCatalog(t, apiKey)
	custExt, _ := createCustomerAndSubscription(t, apiKey, versionID)

	// Endpoint that always fails with 500.
	var hits atomic.Int64
	wc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer wc.Close()

	createWebhookViaAPI(t, apiKey, wc.URL)

	// Ingest 4 usage events → 4 outbox events → 4 deliveries, enough to
	// trip a breaker with FailureThreshold=4.
	for range 4 {
		txID := "wh-cb-tx-" + uuid.NewString()[:8]
		status, _ := ingestUsage(t, apiKey, txID, custExt, "api_calls",
			time.Now().UTC().Format(time.RFC3339Nano), map[string]any{"count": 1})
		if status != http.StatusCreated {
			t.Fatalf("ingest usage: status %d", status)
		}
	}
	relay := outbox.NewRelay(appStore, billing.NewNoop(nil), 50*time.Millisecond,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := relay.DrainOnce(testCtx); err != nil {
		t.Fatalf("relay drain: %v", err)
	}

	worker := newTestWebhookWorker()
	worker.SetBreakerOptions(circuitbreaker.Options{
		FailureThreshold: 4,
		OpenTimeout:      5 * time.Minute, // stays open for the whole test
	})

	// Drain until the endpoint has been hit at least 4 times (breaker open).
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && hits.Load() < 4 {
		if err := worker.DrainOnce(testCtx); err != nil {
			t.Fatalf("drain: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := hits.Load(); got != 4 {
		t.Fatalf("endpoint hit %d times, want exactly 4 (threshold)", got)
	}

	// More drains must not produce any real HTTP call: deliveries are
	// fast-failed while the breaker is open (and retried later).
	for range 4 {
		if err := worker.DrainOnce(testCtx); err != nil {
			t.Fatalf("drain after trip: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := hits.Load(); got != 4 {
		t.Fatalf("endpoint hit %d times while breaker open, want 4 (must fast-fail)", got)
	}
}
