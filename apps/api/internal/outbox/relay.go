// Package outbox implements the Phase 0 relay worker: it polls the
// outbox_events table and marks pending events as published. There is no
// Kafka in Phase 0 — "publishing" is a status transition plus a structured
// log line. The worker uses the operator context so it can drain events of
// every tenant (RLS applies to worker-originated queries too).
package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/jackc/pgx/v5"
)

type Relay struct {
	store    *store.Store
	interval time.Duration
	batch    int32
	log      *slog.Logger
}

func NewRelay(st *store.Store, interval time.Duration, log *slog.Logger) *Relay {
	if interval <= 0 {
		interval = time.Second
	}
	return &Relay{store: st, interval: interval, batch: 100, log: log}
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

// drain claims a batch of pending events inside one transaction
// (FOR UPDATE SKIP LOCKED) and marks them published.
func (r *Relay) drain(ctx context.Context) error {
	return r.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		events, err := q.ClaimPendingOutboxEvents(ctx, r.batch)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if err := q.MarkOutboxEventPublished(ctx, ev.ID); err != nil {
				return err
			}
			r.log.Info("outbox event published",
				"event_id", ev.ID.String(),
				"event_type", ev.EventType,
				"provider_id", ev.ProviderID.String(),
				"environment_id", ev.EnvironmentID.String(),
				"payload_hash", ev.PayloadHash,
			)
		}
		return nil
	})
}
