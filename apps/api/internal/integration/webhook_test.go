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
	"testing"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/billing"
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
