// Package outbox implements the relay worker: it polls the outbox_events
// table, delivers usage events to the billing adapter, and marks each
// event's outcome (published / retry / dead-letter). Non-usage events
// (catalog.*, subscription.*, customer.*, credential.*, provider.*) are
// marked published without external delivery — they are durable business
// facts whose consumers read the outbox table directly in Phase 1. The
// worker uses the operator context so it can drain events of every tenant
// (RLS applies to worker-originated queries too).
package outbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/billing"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	// defaultMaxAttempts is the delivery attempt ceiling before an event
	// is dead-lettered (status='failed', next_attempt_at=NULL).
	defaultMaxAttempts = 5
	// retryBase is the base delay for exponential backoff.
	retryBase = 2 * time.Second
	// retryCap is the maximum backoff delay.
	retryCap = 5 * time.Minute
)

type Relay struct {
	store       *store.Store
	adapter     billing.Adapter
	interval    time.Duration
	batch       int32
	maxAttempts int
	log         *slog.Logger
}

// NewRelay builds a relay that delivers usage events to the billing
// adapter and marks all other events published. interval defaults to 1s
// when non-positive.
func NewRelay(st *store.Store, adapter billing.Adapter, interval time.Duration, log *slog.Logger) *Relay {
	if interval <= 0 {
		interval = time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	if adapter == nil {
		adapter = billing.NewNoop(log)
	}
	return &Relay{
		store:       st,
		adapter:     adapter,
		interval:    interval,
		batch:       100,
		maxAttempts: defaultMaxAttempts,
		log:         log,
	}
}

// Run polls until ctx is cancelled, then returns nil (graceful shutdown).
func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			r.log.Info("outbox relay shutting down")
			return nil
		case <-ticker.C:
			if err := r.drain(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				r.log.Error("outbox relay drain failed", "error", err)
			}
		}
	}
}

// DrainOnce processes one batch synchronously. Exported for tests and for
// running the relay in a manually-triggered mode.
func (r *Relay) DrainOnce(ctx context.Context) error {
	return r.drain(ctx)
}

// drain claims a batch of due events, delivers each outside the claim
// transaction, and marks the outcome in separate short transactions.
//
// Claim is an atomic UPDATE...RETURNING that sets next_attempt_at 120s
// ahead (a lease): concurrent relay instances skip leased rows via SKIP
// LOCKED + the next_attempt_at filter. Delivery happens with NO database
// locks held, so slow HTTP calls to the billing engine cannot starve
// other workers. If the relay crashes mid-delivery, the 120s lease
// expires and another instance re-claims the event — at-least-once.
func (r *Relay) drain(ctx context.Context) error {
	events, err := r.claim(ctx)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	for _, ev := range events {
		if err := r.processEvent(ctx, ev); err != nil {
			r.log.Error("outbox event processing error",
				"event_id", ev.ID.String(),
				"event_type", ev.EventType,
				"error", err,
			)
		}
	}
	return nil
}

// claim atomically claims and leases a batch of due events.
func (r *Relay) claim(ctx context.Context) ([]storegen.OutboxEvent, error) {
	var events []storegen.OutboxEvent
	err := r.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		var err error
		events, err = q.ClaimDueOutboxEvents(ctx, r.batch)
		return err
	})
	return events, err
}

// processEvent delivers one event and marks its outcome. Usage events go
// to the billing adapter; all other event types are marked published
// directly (Phase 1 does not relay them externally).
func (r *Relay) processEvent(ctx context.Context, ev storegen.OutboxEvent) error {
	switch ev.EventType {
	case "usage.accepted", "usage.reversed":
		return r.deliverUsage(ctx, ev)
	default:
		// catalog.*, subscription.*, customer.*, credential.*,
		// provider.*, entitlement_*: not externally delivered in Phase 1.
		if err := r.markPublished(ctx, ev.ID); err != nil {
			return err
		}
		r.log.Info("outbox event published (internal)",
			"event_id", ev.ID.String(),
			"event_type", ev.EventType,
			"provider_id", ev.ProviderID.String(),
		)
		return nil
	}
}

// deliverUsage decodes the outbox payload into a billing.UsageEvent,
// delivers it to the adapter, and marks published / retry / dead-letter.
func (r *Relay) deliverUsage(ctx context.Context, ev storegen.OutboxEvent) error {
	var ue billing.UsageEvent
	if err := json.Unmarshal(ev.Payload, &ue); err != nil {
		// Unparseable payload: dead-letter immediately — retrying won't help.
		r.log.Error("outbox event payload unmarshal failed; dead-lettering",
			"event_id", ev.ID.String(),
			"event_type", ev.EventType,
			"error", err,
		)
		return r.markDeadLetter(ctx, ev.ID)
	}

	if err := r.adapter.DeliverUsageEvent(ctx, ue); err != nil {
		attempts := int(ev.Attempts) + 1
		if attempts >= r.maxAttempts {
			r.log.Error("outbox event dead-lettered (max attempts reached)",
				"event_id", ev.ID.String(),
				"event_type", ev.EventType,
				"attempts", attempts,
				"error", err,
			)
			return r.markDeadLetter(ctx, ev.ID)
		}
		next := nextRetryAt(time.Now(), attempts)
		r.log.Warn("outbox event delivery failed; scheduling retry",
			"event_id", ev.ID.String(),
			"event_type", ev.EventType,
			"attempts", attempts,
			"next_attempt_at", next.Format(time.RFC3339),
			"error", err,
		)
		return r.markRetry(ctx, ev.ID, next)
	}

	if err := r.markPublished(ctx, ev.ID); err != nil {
		return err
	}
	r.log.Info("outbox usage event delivered and published",
		"event_id", ev.ID.String(),
		"event_type", ev.EventType,
		"transaction_id", ue.TransactionID,
		"provider_id", ev.ProviderID.String(),
	)
	return nil
}

// markPublished marks an event as delivered in a short transaction.
func (r *Relay) markPublished(ctx context.Context, id uuid.UUID) error {
	return r.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		return q.MarkOutboxEventPublished(ctx, id)
	})
}

// markRetry schedules a retry with exponential backoff.
func (r *Relay) markRetry(ctx context.Context, id uuid.UUID, next time.Time) error {
	return r.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		return q.MarkOutboxEventRetry(ctx, storegen.MarkOutboxEventRetryParams{
			ID:            id,
			NextAttemptAt: &next,
		})
	})
}

// markDeadLetter permanently fails an event (max attempts reached or
// unparseable payload).
func (r *Relay) markDeadLetter(ctx context.Context, id uuid.UUID) error {
	return r.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		return q.MarkOutboxEventDeadLetter(ctx, id)
	})
}

// nextRetryAt computes an exponential backoff delay capped at retryCap:
// base * 2^(attempts-1), where attempts is the count after increment.
func nextRetryAt(now time.Time, attempts int) time.Time {
	delay := retryBase << uint(attempts-1) // base * 2^(attempts-1)
	if delay > retryCap || delay <= 0 {
		delay = retryCap
	}
	return now.Add(delay)
}
