package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// intToPgInt4 converts a Go int to a pgtype.Int4. Zero is treated as NULL
// (no HTTP response, e.g. network error).
func intToPgInt4(v int) pgtype.Int4 {
	if v == 0 {
		return pgtype.Int4{Valid: false}
	}
	return pgtype.Int4{Int32: int32(v), Valid: true}
}

// stringToPgText converts a Go string to a pgtype.Text. Empty string → NULL.
func stringToPgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: s, Valid: true}
}

const (
	// defaultMaxAttempts is the delivery attempt ceiling before a delivery
	// is dead-lettered.
	defaultMaxAttempts = 3
	// retryBase is the base delay for exponential backoff.
	retryBase = 2 * time.Second
	// retryCap is the maximum backoff delay.
	retryCap = 5 * time.Minute
	// httpTimeout is the per-request timeout for webhook HTTP delivery.
	httpTimeout = 10 * time.Second
	// undeliveredBatch is how many published outbox events to scan per cycle.
	undeliveredBatch = 500
	// deliveryBatch is how many pending deliveries to claim per cycle.
	deliveryBatch = 50
)

// Worker polls published outbox events, creates delivery records for each
// matching webhook endpoint, then claims pending deliveries and performs
// the HTTP POST with HMAC-SHA256 signing. Delivery (the HTTP call) happens
// OUTSIDE the claim transaction so slow endpoints cannot starve the worker.
type Worker struct {
	store        *store.Store
	log          *slog.Logger
	interval     time.Duration
	maxAttempts  int
	httpClient   *http.Client
	urlValidator func(string) error // SSRF check at delivery time (TOCTOU prevention)
}

// NewWorker builds a webhook delivery worker. interval defaults to 5s when
// non-positive.
func NewWorker(st *store.Store, log *slog.Logger, interval time.Duration) *Worker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	return &Worker{
		store:        st,
		log:          log,
		interval:     interval,
		maxAttempts:  defaultMaxAttempts,
		httpClient:   &http.Client{Timeout: httpTimeout},
		urlValidator: ValidateURL, // default: strict SSRF; tests override with AllowLoopback
	}
}

// SetURLValidator configures the SSRF check function used at delivery
// time. Tests should set ValidateURLAllowLoopback. This prevents
// DNS-rebinding (TOCTOU) by re-validating the URL before each HTTP POST.
func (w *Worker) SetURLValidator(fn func(string) error) {
	if fn != nil {
		w.urlValidator = fn
	}
}

// Run polls until ctx is cancelled, then returns nil (graceful shutdown).
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.log.Info("webhook worker shutting down")
			return nil
		case <-ticker.C:
			if err := w.DrainOnce(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				w.log.Error("webhook worker drain failed", "error", err)
			}
		}
	}
}

// DrainOnce processes one full cycle: create delivery records for newly
// published outbox events, then claim and deliver pending deliveries.
// Exported for tests.
func (w *Worker) DrainOnce(ctx context.Context) error {
	if err := w.createDeliveries(ctx); err != nil {
		return fmt.Errorf("create deliveries: %w", err)
	}
	if err := w.deliverPending(ctx); err != nil {
		return fmt.Errorf("deliver pending: %w", err)
	}
	return nil
}

// createDeliveries finds published outbox events that have no
// webhook_deliveries yet and creates one delivery record per matching
// enabled endpoint. Dedup is enforced by the UNIQUE(endpoint_id,
// outbox_event_id) constraint (ON CONFLICT DO NOTHING).
func (w *Worker) createDeliveries(ctx context.Context) error {
	var events []storegen.OutboxEvent
	err := w.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		var err error
		events, err = q.FindUndeliveredOutboxEvents(ctx, undeliveredBatch)
		return err
	})
	if err != nil {
		return err
	}

	for _, ev := range events {
		var endpoints []storegen.WebhookEndpoint
		err := w.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
			var err error
			endpoints, err = q.ListEnabledWebhooksByTenant(ctx, storegen.ListEnabledWebhooksByTenantParams{
				ProviderID:    ev.ProviderID,
				EnvironmentID: ev.EnvironmentID,
			})
			return err
		})
		if err != nil {
			w.log.Error("list enabled webhooks failed",
				"event_id", ev.ID.String(),
				"provider_id", ev.ProviderID.String(),
				"error", err,
			)
			continue
		}
		for _, ep := range endpoints {
			if !eventMatchesFilter(ev.EventType, ep.Events) {
				continue
			}
			if err := w.createDelivery(ctx, ep.ID, ev.ID, ev.ProviderID, ev.EnvironmentID); err != nil {
				w.log.Error("create webhook delivery failed",
					"endpoint_id", ep.ID.String(),
					"event_id", ev.ID.String(),
					"error", err,
				)
			}
		}
	}
	return nil
}

// createDelivery inserts a webhook_deliveries row (idempotent via ON
// CONFLICT DO NOTHING).
func (w *Worker) createDelivery(ctx context.Context, endpointID, eventID, providerID, environmentID uuid.UUID) error {
	return w.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		return q.CreateWebhookDelivery(ctx, storegen.CreateWebhookDeliveryParams{
			EndpointID:    endpointID,
			OutboxEventID: eventID,
			ProviderID:    providerID,
			EnvironmentID: environmentID,
		})
	})
}

// deliverPending claims a batch of due deliveries (atomic UPDATE with
// FOR UPDATE SKIP LOCKED + a 60s lease), then delivers each outside the
// transaction and marks the outcome in a separate short transaction.
func (w *Worker) deliverPending(ctx context.Context) error {
	var deliveries []storegen.WebhookDelivery
	err := w.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		var err error
		deliveries, err = q.ClaimPendingWebhookDeliveries(ctx, deliveryBatch)
		return err
	})
	if err != nil {
		return err
	}
	for _, d := range deliveries {
		if err := w.processDelivery(ctx, d); err != nil {
			w.log.Error("webhook delivery processing error",
				"delivery_id", d.ID.String(),
				"endpoint_id", d.EndpointID.String(),
				"error", err,
			)
		}
	}
	return nil
}

// processDelivery resolves the endpoint + outbox event, signs and sends
// the HTTP request, and marks the delivery outcome.
func (w *Worker) processDelivery(ctx context.Context, d storegen.WebhookDelivery) error {
	endpoint, outboxEvent, err := w.loadDeliveryContext(ctx, d)
	if err != nil {
		return err
	}
	if endpoint == nil {
		// Endpoint was deleted; dead-letter to avoid retrying forever.
		return w.markDeadLetter(ctx, d.ID, 0, "endpoint not found")
	}

	status, body, err := w.deliver(ctx, endpoint, outboxEvent)
	if err != nil {
		// Network error / timeout → retry with backoff.
		return w.handleFailure(ctx, d, 0, err.Error())
	}
	if status >= 200 && status < 300 {
		return w.markDelivered(ctx, d.ID, status, body)
	}
	return w.handleFailure(ctx, d, status, body)
}

// loadDeliveryContext fetches the endpoint and outbox event for a delivery.
// A nil endpoint (with nil error) means the endpoint was deleted.
// loadDeliveryContext fetches the endpoint and outbox event for a delivery
// using direct ID lookups (O(1) per delivery, not O(n) list scans).
func (w *Worker) loadDeliveryContext(ctx context.Context, d storegen.WebhookDelivery) (*storegen.WebhookEndpoint, *storegen.OutboxEvent, error) {
	var endpoint *storegen.WebhookEndpoint
	var outboxEvent *storegen.OutboxEvent
	err := w.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		ep, err := q.GetWebhookEndpointByID(ctx, storegen.GetWebhookEndpointByIDParams{
			ID:            d.EndpointID,
			ProviderID:    d.ProviderID,
			EnvironmentID: d.EnvironmentID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // endpoint deleted; skip delivery
			}
			return err
		}
		endpoint = &ep

		ev, err := q.GetOutboxEventByIDForWebhook(ctx, storegen.GetOutboxEventByIDForWebhookParams{
			ID:            d.OutboxEventID,
			ProviderID:    d.ProviderID,
			EnvironmentID: d.EnvironmentID,
		})
		if err != nil {
			return fmt.Errorf("outbox event %s not found: %w", d.OutboxEventID, err)
		}
		outboxEvent = &ev
		return nil
	})
	return endpoint, outboxEvent, err
}

// deliver sends the signed HTTP POST and returns (statusCode, responseBody, error).
// A non-nil error means the request could not be completed (network/timeout).
func (w *Worker) deliver(ctx context.Context, ep *storegen.WebhookEndpoint, ev *storegen.OutboxEvent) (int, string, error) {
	// SSRF re-validation at delivery time (TOCTOU prevention): the DNS
	// may have changed since the endpoint was registered. If the URL is
	// now unsafe, dead-letter the delivery immediately.
	if w.urlValidator != nil {
		if err := w.urlValidator(ep.Url); err != nil {
			return 0, "", fmt.Errorf("SSRF blocked at delivery: %w", err)
		}
	}

	payload := ev.Payload
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	sig := signPayload(ep.Secret, timestamp, payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.Url, bytes.NewReader(payload))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", sig)
	req.Header.Set("X-Webhook-Timestamp", timestamp)
	req.Header.Set("X-Webhook-Event-Type", ev.EventType)
	req.Header.Set("X-Webhook-Schema-Version", domain.WebhookSchemaVersion)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MB cap
	return resp.StatusCode, string(body), nil
}

// handleFailure records a failed delivery. If attempts have reached
// maxAttempts, the delivery is dead-lettered; otherwise a retry is
// scheduled with exponential backoff.
func (w *Worker) handleFailure(ctx context.Context, d storegen.WebhookDelivery, status int, body string) error {
	attempts := int(d.Attempts)
	if attempts >= w.maxAttempts {
		w.log.Error("webhook delivery dead-lettered (max attempts reached)",
			"delivery_id", d.ID.String(),
			"endpoint_id", d.EndpointID.String(),
			"attempts", attempts,
			"response_status", status,
		)
		return w.markDeadLetter(ctx, d.ID, status, body)
	}
	next := nextRetryAt(time.Now(), attempts+1)
	w.log.Warn("webhook delivery failed; scheduling retry",
		"delivery_id", d.ID.String(),
		"endpoint_id", d.EndpointID.String(),
		"attempts", attempts+1,
		"response_status", status,
		"next_attempt_at", next.Format(time.RFC3339),
	)
	return w.markRetry(ctx, d.ID, status, body, next)
}

// signPayload computes the HMAC-SHA256 signature of timestamp.payload
// using the endpoint secret, returned as a lowercase hex string.
func signPayload(secret, timestamp string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// eventMatchesFilter reports whether eventType should be delivered to an
// endpoint with the given events filter. An empty filter matches all.
func eventMatchesFilter(eventType string, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	return slices.Contains(filter, eventType)
}

// nextRetryAt computes exponential backoff: base * 2^(attempts-1),
// capped at retryCap.
func nextRetryAt(now time.Time, attempts int) time.Time {
	delay := retryBase << uint(attempts-1)
	if delay > retryCap || delay <= 0 {
		delay = retryCap
	}
	return now.Add(delay)
}

func (w *Worker) markDelivered(ctx context.Context, id uuid.UUID, status int, body string) error {
	return w.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		return q.MarkWebhookDelivered(ctx, storegen.MarkWebhookDeliveredParams{
			ID:             id,
			ResponseStatus: intToPgInt4(status),
			ResponseBody:   stringToPgText(body),
		})
	})
}

func (w *Worker) markRetry(ctx context.Context, id uuid.UUID, status int, body string, next time.Time) error {
	return w.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		return q.MarkWebhookRetry(ctx, storegen.MarkWebhookRetryParams{
			ID:             id,
			ResponseStatus: intToPgInt4(status),
			ResponseBody:   stringToPgText(body),
			NextAttemptAt:  &next,
		})
	})
}

func (w *Worker) markDeadLetter(ctx context.Context, id uuid.UUID, status int, body string) error {
	return w.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		return q.MarkWebhookDeadLetter(ctx, storegen.MarkWebhookDeadLetterParams{
			ID:             id,
			ResponseStatus: intToPgInt4(status),
			ResponseBody:   stringToPgText(body),
		})
	})
}
