package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SetQuotaLimitInput configures a hard quota limit for a subscription.
type SetQuotaLimitInput struct {
	SubscriptionID uuid.UUID
	QuotaKey       string
	LimitValue     int64
	PeriodType     string
}

// QuotaUsageResult is the current committed and reserved usage for a
// quota key, plus the configured limit.
type QuotaUsageResult struct {
	Committed  int64               `json:"committed"`
	Reserved   int64               `json:"reserved"`
	Limit      int64               `json:"limit"`
	PeriodType string              `json:"period_type"`
	LimitRow   storegen.QuotaLimit `json:"limit_row"`
}

// QuotaLimitUsage is a quota limit plus its live committed/reserved usage.
// Operator quota console renders these as one table without N+1.
type QuotaLimitUsage struct {
	ID             uuid.UUID `json:"id"`
	SubscriptionID uuid.UUID `json:"subscription_id"`
	QuotaKey       string    `json:"quota_key"`
	LimitValue     int64     `json:"limit_value"`
	PeriodType     string    `json:"period_type"`
	Committed      int64     `json:"committed"`
	Reserved       int64     `json:"reserved"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SetQuotaLimit creates or updates a hard quota limit for a subscription.
// The limit is scoped to the caller's tenant (RLS-enforced).
func (s *Service) SetQuotaLimit(ctx context.Context, tc tenant.Ctx, in SetQuotaLimitInput) (*storegen.QuotaLimit, error) {
	if in.QuotaKey == "" {
		return nil, fmt.Errorf("%w: quota_key is required", ErrValidation)
	}
	if in.LimitValue < 0 {
		return nil, fmt.Errorf("%w: limit_value must be non-negative", ErrValidation)
	}
	if !domain.ValidQuotaPeriod(in.PeriodType) {
		return nil, fmt.Errorf("%w: period_type must be daily, monthly or total", ErrValidation)
	}
	var limit storegen.QuotaLimit
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Verify the subscription belongs to this tenant (RLS-enforced).
		if _, err := q.GetSubscriptionByID(ctx, in.SubscriptionID); err != nil {
			return mapErr(err, "subscription %s", in.SubscriptionID)
		}
		l, err := q.UpsertQuotaLimit(ctx, storegen.UpsertQuotaLimitParams{
			ProviderID:     tc.ProviderID,
			EnvironmentID:  tc.EnvironmentID,
			SubscriptionID: in.SubscriptionID,
			QuotaKey:       in.QuotaKey,
			LimitValue:     in.LimitValue,
			PeriodType:     in.PeriodType,
		})
		if err != nil {
			return mapErr(err, "quota limit %q", in.QuotaKey)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "quota_limit", l.ID.String(), "quota.limit_set", map[string]any{
			"subscription_id": in.SubscriptionID.String(),
			"quota_key":       in.QuotaKey,
			"limit_value":     in.LimitValue,
			"period_type":     in.PeriodType,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "quota.limit_set",
			"quota_limit", l.ID.String(),
			map[string]any{"subscription_id": in.SubscriptionID.String(), "quota_key": in.QuotaKey, "limit_value": in.LimitValue}); err != nil {
			return err
		}
		limit = l
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &limit, nil
}

// GetQuotaLimit returns the configured limit for a subscription + key.
func (s *Service) GetQuotaLimit(ctx context.Context, tc tenant.Ctx, subscriptionID uuid.UUID, quotaKey string) (*storegen.QuotaLimit, error) {
	var limit storegen.QuotaLimit
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		l, err := q.GetQuotaLimit(ctx, storegen.GetQuotaLimitParams{
			SubscriptionID: subscriptionID, QuotaKey: quotaKey,
		})
		if err != nil {
			return mapErr(err, "quota limit %q", quotaKey)
		}
		limit = l
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &limit, nil
}

// ListQuotaLimits returns all quota limits for a subscription.
func (s *Service) ListQuotaLimits(ctx context.Context, tc tenant.Ctx, subscriptionID uuid.UUID) ([]storegen.QuotaLimit, error) {
	var out []storegen.QuotaLimit
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Verify ownership (RLS-enforced).
		if _, err := q.GetSubscriptionByID(ctx, subscriptionID); err != nil {
			return mapErr(err, "subscription %s", subscriptionID)
		}
		limits, err := q.ListQuotaLimitsBySubscription(ctx, storegen.ListQuotaLimitsBySubscriptionParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, SubscriptionID: subscriptionID,
		})
		out = limits
		return err
	})
	return out, err
}

// ListQuotaLimitsWithUsage returns each quota limit with current
// committed/reserved usage in a single tenant transaction.
func (s *Service) ListQuotaLimitsWithUsage(ctx context.Context, tc tenant.Ctx, subscriptionID uuid.UUID) ([]QuotaLimitUsage, error) {
	out := []QuotaLimitUsage{}
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetSubscriptionByID(ctx, subscriptionID); err != nil {
			return mapErr(err, "subscription %s", subscriptionID)
		}
		limits, err := q.ListQuotaLimitsBySubscription(ctx, storegen.ListQuotaLimitsBySubscriptionParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, SubscriptionID: subscriptionID,
		})
		if err != nil {
			return err
		}
		out = make([]QuotaLimitUsage, 0, len(limits))
		for _, limit := range limits {
			usage, err := q.GetQuotaUsage(ctx, storegen.GetQuotaUsageParams{
				SubscriptionID: subscriptionID, QuotaKey: limit.QuotaKey,
			})
			if err != nil {
				return mapErr(err, "quota usage %q", limit.QuotaKey)
			}
			out = append(out, QuotaLimitUsage{
				ID:             limit.ID,
				SubscriptionID: limit.SubscriptionID,
				QuotaKey:       limit.QuotaKey,
				LimitValue:     limit.LimitValue,
				PeriodType:     limit.PeriodType,
				Committed:      usage.Committed,
				Reserved:       usage.Reserved,
				CreatedAt:      limit.CreatedAt,
				UpdatedAt:      limit.UpdatedAt,
			})
		}
		return nil
	})
	return out, err
}

// DeleteQuotaLimit removes a quota limit. Existing committed reservations
// are unaffected; new reservations will be rejected (no limit = no quota).
func (s *Service) DeleteQuotaLimit(ctx context.Context, tc tenant.Ctx, subscriptionID uuid.UUID, quotaKey string) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := q.DeleteQuotaLimit(ctx, storegen.DeleteQuotaLimitParams{
			ProviderID:     tc.ProviderID,
			EnvironmentID:  tc.EnvironmentID,
			SubscriptionID: subscriptionID,
			QuotaKey:       quotaKey,
		})
		if err != nil {
			return mapErr(err, "quota limit %q", quotaKey)
		}
		if rows == 0 {
			return fmt.Errorf("%w: quota limit %q", ErrNotFound, quotaKey)
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "quota.limit_deleted",
			"quota_limit", subscriptionID.String(),
			map[string]any{"quota_key": quotaKey}); err != nil {
			return err
		}
		return nil
	})
}

// ReserveQuotaInput is the parameter bundle for reserving quota.
type ReserveQuotaInput struct {
	SubscriptionID uuid.UUID
	QuotaKey       string
	Amount         int64
	ReservationID  string // caller-supplied idempotency key
	ExpiresAt      *time.Time
}

// ReserveQuota atomically checks the current usage against the limit and
// creates a reservation if within bounds. The reservation_id provides
// idempotency: a retry with the same ID returns the original reservation.
// Returns ErrQuotaExceeded when the reservation would exceed the limit.
func (s *Service) ReserveQuota(ctx context.Context, tc tenant.Ctx, in ReserveQuotaInput) (*storegen.QuotaReservation, error) {
	if in.QuotaKey == "" {
		return nil, fmt.Errorf("%w: quota_key is required", ErrValidation)
	}
	if in.Amount <= 0 {
		return nil, fmt.Errorf("%w: amount must be positive", ErrValidation)
	}
	if in.ReservationID == "" {
		return nil, fmt.Errorf("%w: reservation_id is required", ErrValidation)
	}
	var reservation storegen.QuotaReservation
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Verify the subscription belongs to this tenant (RLS-enforced).
		if _, err := q.GetSubscriptionByID(ctx, in.SubscriptionID); err != nil {
			return mapErr(err, "subscription %s", in.SubscriptionID)
		}

		// Idempotency: if a reservation with the same ID exists, return it.
		existing, err := q.GetQuotaReservationByTxID(ctx, storegen.GetQuotaReservationByTxIDParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ReservationID: in.ReservationID,
		})
		if err == nil {
			reservation = existing
			return nil
		}
		if !isNoRows(err) {
			return mapErr(err, "quota reservation %q", in.ReservationID)
		}

		// Atomic check-and-insert. The CTE computes current usage and only
		// inserts if usage + amount <= limit.
		r, err := q.ReserveQuota(ctx, storegen.ReserveQuotaParams{
			SubscriptionID: in.SubscriptionID,
			QuotaKey:       in.QuotaKey,
			ProviderID:     tc.ProviderID,
			EnvironmentID:  tc.EnvironmentID,
			Amount:         in.Amount,
			ReservationID:  in.ReservationID,
			ExpiresAt:      in.ExpiresAt,
		})
		if err != nil {
			if isNoRows(err) {
				return fmt.Errorf("%w: quota %q would exceed limit (amount=%d)", ErrQuotaExceeded, in.QuotaKey, in.Amount)
			}
			return mapErr(err, "reserve quota %q", in.QuotaKey)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "quota_reservation", r.ID.String(), "quota.reserved", map[string]any{
			"reservation_id":  in.ReservationID,
			"subscription_id": in.SubscriptionID.String(),
			"quota_key":       in.QuotaKey,
			"amount":          in.Amount,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "quota.reserve",
			"quota_reservation", r.ID.String(),
			map[string]any{"quota_key": in.QuotaKey, "amount": in.Amount, "reservation_id": in.ReservationID}); err != nil {
			return err
		}
		reservation = r
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &reservation, nil
}

// CommitQuota transitions a reservation from "reserved" to "committed",
// making the usage permanent.
func (s *Service) CommitQuota(ctx context.Context, tc tenant.Ctx, reservationID uuid.UUID) (*storegen.QuotaReservation, error) {
	var reservation storegen.QuotaReservation
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		r, err := q.GetQuotaReservationByID(ctx, reservationID)
		if err != nil {
			return mapErr(err, "quota reservation %s", reservationID)
		}
		if err := checkTenantOwnership(r.ProviderID, r.EnvironmentID, tc, "quota reservation", reservationID); err != nil {
			return err
		}
		if r.Status != domain.QuotaReserved {
			return fmt.Errorf("%w: quota reservation %s is not reserved (status=%s)", ErrConflict, reservationID, r.Status)
		}
		updated, err := q.CommitQuotaReservation(ctx, reservationID)
		if err != nil {
			return mapErr(err, "commit quota reservation %s", reservationID)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "quota_reservation", reservationID.String(), "quota.committed", map[string]any{
			"reservation_id": reservationID.String(),
			"amount":         r.Amount,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "quota.commit",
			"quota_reservation", reservationID.String(),
			map[string]any{"quota_key": r.QuotaKey, "amount": r.Amount}); err != nil {
			return err
		}
		reservation = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &reservation, nil
}

// ReleaseQuota transitions a reservation from "reserved" to "released",
// freeing the reserved amount back to the pool.
func (s *Service) ReleaseQuota(ctx context.Context, tc tenant.Ctx, reservationID uuid.UUID) (*storegen.QuotaReservation, error) {
	var reservation storegen.QuotaReservation
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		r, err := q.GetQuotaReservationByID(ctx, reservationID)
		if err != nil {
			return mapErr(err, "quota reservation %s", reservationID)
		}
		if err := checkTenantOwnership(r.ProviderID, r.EnvironmentID, tc, "quota reservation", reservationID); err != nil {
			return err
		}
		if r.Status != domain.QuotaReserved {
			return fmt.Errorf("%w: quota reservation %s is not reserved (status=%s)", ErrConflict, reservationID, r.Status)
		}
		updated, err := q.ReleaseQuotaReservation(ctx, reservationID)
		if err != nil {
			return mapErr(err, "release quota reservation %s", reservationID)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "quota_reservation", reservationID.String(), "quota.released", map[string]any{
			"reservation_id": reservationID.String(),
			"amount":         r.Amount,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "quota.release",
			"quota_reservation", reservationID.String(),
			map[string]any{"quota_key": r.QuotaKey, "amount": r.Amount}); err != nil {
			return err
		}
		reservation = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &reservation, nil
}

// GetQuotaUsage returns the current committed and reserved usage for a
// quota key, along with the configured limit.
func (s *Service) GetQuotaUsage(ctx context.Context, tc tenant.Ctx, subscriptionID uuid.UUID, quotaKey string) (*QuotaUsageResult, error) {
	var out QuotaUsageResult
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		usage, err := q.GetQuotaUsage(ctx, storegen.GetQuotaUsageParams{
			SubscriptionID: subscriptionID, QuotaKey: quotaKey,
		})
		if err != nil {
			return mapErr(err, "quota usage %q", quotaKey)
		}
		limit, err := q.GetQuotaLimit(ctx, storegen.GetQuotaLimitParams{
			SubscriptionID: subscriptionID, QuotaKey: quotaKey,
		})
		if err != nil {
			return mapErr(err, "quota limit %q", quotaKey)
		}
		out = QuotaUsageResult{
			Committed:  usage.Committed,
			Reserved:   usage.Reserved,
			Limit:      limit.LimitValue,
			PeriodType: limit.PeriodType,
			LimitRow:   limit,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListQuotaReservations returns active (reserved + committed) reservations
// for a subscription.
func (s *Service) ListQuotaReservations(ctx context.Context, tc tenant.Ctx, subscriptionID uuid.UUID) ([]storegen.QuotaReservation, error) {
	var out []storegen.QuotaReservation
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Verify ownership (RLS-enforced).
		if _, err := q.GetSubscriptionByID(ctx, subscriptionID); err != nil {
			return mapErr(err, "subscription %s", subscriptionID)
		}
		reservations, err := q.ListQuotaReservationsBySubscription(ctx, storegen.ListQuotaReservationsBySubscriptionParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, SubscriptionID: subscriptionID, Limit: 100,
		})
		out = reservations
		return err
	})
	return out, err
}

// RecoverExpiredReservations batch-expires all past-due reservations.
// Called by the background sweeper goroutine.
func (s *Service) RecoverExpiredReservations(ctx context.Context) (int64, error) {
	var n int64
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		count, err := q.ExpireQuotaReservations(ctx)
		n = count
		return err
	})
	return n, err
}

// NewQuotaExpirySweeper creates a background sweeper that expires
// past-due quota reservations at the given interval.
func NewQuotaExpirySweeper(svc *Service, interval time.Duration, log *slog.Logger) *ExpirySweeper {
	return NewExpirySweeper("quota_reservation", svc.RecoverExpiredReservations, interval, log)
}

// isNoRows reports whether err is pgx.ErrNoRows.
func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
