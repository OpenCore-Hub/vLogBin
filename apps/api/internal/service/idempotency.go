package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
)

// PurgeExpiredIdempotencyKeys deletes idempotency records past their TTL.
// Called by the background sweeper (NewIdempotencyKeySweeper) so the
// idempotency_keys table does not grow without bound. Runs in the operator
// context so RLS does not scope the sweep.
func (s *Service) PurgeExpiredIdempotencyKeys(ctx context.Context, cutoff time.Time) (int64, error) {
	var n int64
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		var err error
		n, err = q.DeleteExpiredIdempotencyKeys(ctx, cutoff)
		return err
	})
	return n, err
}

// NewIdempotencyKeySweeper creates a background sweeper that purges
// idempotency records past their TTL at the given interval. The cutoff is
// recomputed on every sweep so a long shutdown gap does not age the window.
func NewIdempotencyKeySweeper(svc *Service, ttl, interval time.Duration, log *slog.Logger) *ExpirySweeper {
	return NewExpirySweeper("idempotency_keys", func(ctx context.Context) (int64, error) {
		return svc.PurgeExpiredIdempotencyKeys(ctx, time.Now().UTC().Add(-ttl))
	}, interval, log)
}
