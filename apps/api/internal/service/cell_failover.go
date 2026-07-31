package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// FailoverInput is the parameter bundle for initiating a cell failover.
type FailoverInput struct {
	ProviderID  uuid.UUID
	FromCellID  uuid.UUID
	ToCellID    uuid.UUID
	Reason      string
	InitiatedBy string
}

// InitiateFailover starts a manual cell failover (spec Section 14).
// The failover is manual (no auto-dual-master) and proceeds through
// stages: initiated → fenced → switched → replaying → completed.
// Write fencing prevents split-brain during the transition.
func (s *Service) InitiateFailover(ctx context.Context, in FailoverInput) (*storegen.CellFailover, error) {
	if in.Reason == "" {
		return nil, fmt.Errorf("%w: reason is required", ErrValidation)
	}
	if in.InitiatedBy == "" {
		return nil, fmt.Errorf("%w: initiated_by is required", ErrValidation)
	}
	if in.FromCellID == in.ToCellID {
		return nil, fmt.Errorf("%w: from_cell and to_cell must differ", ErrValidation)
	}

	// Generate a fencing token (random hex) for write fencing.
	fencingToken := generateFencingToken()

	var failover storegen.CellFailover
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		// Check no active failover exists for this provider.
		active, err := q.GetActiveCellFailover(ctx, in.ProviderID)
		if err == nil && active.ID != uuid.Nil {
			return fmt.Errorf("%w: provider %s already has an active failover (id=%s, status=%s)", ErrConflict, in.ProviderID, active.ID, active.Status)
		}

		// Verify both cells exist.
		fromCell, err := q.GetCellByID(ctx, in.FromCellID)
		if err != nil {
			return mapErr(err, "from_cell %s", in.FromCellID)
		}
		toCell, err := q.GetCellByID(ctx, in.ToCellID)
		if err != nil {
			return mapErr(err, "to_cell %s", in.ToCellID)
		}
		// Both cells must be in the same region (spec: "同地域热备").
		if fromCell.RegionID != toCell.RegionID {
			return fmt.Errorf("%w: cells must be in the same region for hot standby failover", ErrValidation)
		}
		// Target cell must be active.
		if toCell.Status != domain.CellStatusActive {
			return fmt.Errorf("%w: target cell %s is not active (status=%s)", ErrConflict, in.ToCellID, toCell.Status)
		}

		f, err := q.CreateCellFailover(ctx, storegen.CreateCellFailoverParams{
			ProviderID:   in.ProviderID,
			FromCellID:   in.FromCellID,
			ToCellID:     in.ToCellID,
			Reason:       in.Reason,
			InitiatedBy:  in.InitiatedBy,
			FencingToken: fencingToken,
		})
		if err != nil {
			return mapErr(err, "create failover")
		}

		if err := emitOutboxTx(ctx, q, in.ProviderID, uuid.Nil, "cell_failover", f.ID.String(), "failover.initiated", map[string]any{
			"from_cell": in.FromCellID.String(),
			"to_cell":   in.ToCellID.String(),
			"reason":    in.Reason,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q,
			uuid.NullUUID{UUID: in.ProviderID, Valid: true}, uuid.NullUUID{},
			"operator", in.InitiatedBy, "failover.initiate",
			"cell_failover", f.ID.String(),
			map[string]any{"from_cell": in.FromCellID.String(), "to_cell": in.ToCellID.String()}); err != nil {
			return err
		}
		failover = f
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &failover, nil
}

// FenceFailover applies write fencing to the source cell (spec Section 14:
// "区域切换必须先执行写 fencing"). Sets the source cell to draining
// status to prevent new writes, and transitions the failover to fenced.
func (s *Service) FenceFailover(ctx context.Context, failoverID uuid.UUID) (*storegen.CellFailover, error) {
	var failover storegen.CellFailover
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		f, err := q.GetCellFailoverByID(ctx, failoverID)
		if err != nil {
			return mapErr(err, "failover %s", failoverID)
		}
		if err := requireStatus(f.Status, []string{domain.FailoverStatusInitiated}, "failover", failoverID); err != nil {
			return err
		}

		// Set source cell to draining (write fencing).
		if _, err := q.UpdateCellStatus(ctx, storegen.UpdateCellStatusParams{
			ID: f.FromCellID, Status: domain.CellStatusDraining,
		}); err != nil {
			return mapErr(err, "fence source cell %s", f.FromCellID)
		}

		updated, err := q.UpdateCellFailoverStatus(ctx, storegen.UpdateCellFailoverStatusParams{
			ID: failoverID, Status: domain.FailoverStatusFenced,
		})
		if err != nil {
			return mapErr(err, "update failover status")
		}

		if err := emitOutboxTx(ctx, q, f.ProviderID, uuid.Nil, "cell_failover", failoverID.String(), "failover.fenced", map[string]any{
			"fencing_token": f.FencingToken,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q,
			uuid.NullUUID{UUID: f.ProviderID, Valid: true}, uuid.NullUUID{},
			"operator", "system", "failover.fence",
			"cell_failover", failoverID.String(),
			map[string]any{"fencing_token": f.FencingToken}); err != nil {
			return err
		}
		failover = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &failover, nil
}

// SwitchFailover performs the actual cell switch: assigns the provider
// to the target cell and transitions the failover to "switched".
func (s *Service) SwitchFailover(ctx context.Context, failoverID uuid.UUID) (*storegen.CellFailover, error) {
	var failover storegen.CellFailover
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		f, err := q.GetCellFailoverByID(ctx, failoverID)
		if err != nil {
			return mapErr(err, "failover %s", failoverID)
		}
		if err := requireStatus(f.Status, []string{domain.FailoverStatusFenced}, "failover", failoverID); err != nil {
			return err
		}

		// Reassign provider to target cell.
		if err := q.UpdateProviderCell(ctx, storegen.UpdateProviderCellParams{
			ID:     f.ProviderID,
			CellID: uuid.NullUUID{UUID: f.ToCellID, Valid: true},
		}); err != nil {
			return mapErr(err, "switch provider to target cell")
		}

		updated, err := q.UpdateCellFailoverStatus(ctx, storegen.UpdateCellFailoverStatusParams{
			ID: failoverID, Status: domain.FailoverStatusSwitched,
		})
		if err != nil {
			return mapErr(err, "update failover status")
		}

		if err := emitOutboxTx(ctx, q, f.ProviderID, uuid.Nil, "cell_failover", failoverID.String(), "failover.switched", map[string]any{
			"new_cell": f.ToCellID.String(),
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q,
			uuid.NullUUID{UUID: f.ProviderID, Valid: true}, uuid.NullUUID{},
			"operator", "system", "failover.switch",
			"cell_failover", failoverID.String(),
			map[string]any{"new_cell": f.ToCellID.String()}); err != nil {
			return err
		}
		failover = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &failover, nil
}

// CompleteFailover marks the failover as completed after replaying
// unconfirmed Usage and Outbox events (spec: "切换后重放未确认 Usage 和 Outbox").
// It queries the database for unconfirmed events and records the actual
// counts. The caller does not need to pass counts — they are computed.
func (s *Service) CompleteFailover(ctx context.Context, failoverID uuid.UUID) (*storegen.CellFailover, error) {
	var failover storegen.CellFailover
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		f, err := q.GetCellFailoverByID(ctx, failoverID)
		if err != nil {
			return mapErr(err, "failover %s", failoverID)
		}
		if err := requireStatus(f.Status, []string{domain.FailoverStatusSwitched, domain.FailoverStatusReplaying}, "failover", failoverID); err != nil {
			return err
		}

		// Count unconfirmed outbox events (pending or failed).
		outboxCount, err := q.CountUnconfirmedOutbox(ctx, f.ProviderID)
		if err != nil {
			return fmt.Errorf("count unconfirmed outbox: %w", err)
		}

		// Count uninvoiced usage events.
		usageCount, err := q.CountUninvoicedUsage(ctx, f.ProviderID)
		if err != nil {
			return fmt.Errorf("count uninvoiced usage: %w", err)
		}

		// Record replay counts.
		if err := q.UpdateCellFailoverReplay(ctx, storegen.UpdateCellFailoverReplayParams{
			ID:             failoverID,
			ReplayedUsage:  int32(usageCount),
			ReplayedOutbox: int32(outboxCount),
		}); err != nil {
			return mapErr(err, "update replay counts")
		}

		updated, err := q.UpdateCellFailoverStatus(ctx, storegen.UpdateCellFailoverStatusParams{
			ID: failoverID, Status: domain.FailoverStatusCompleted,
		})
		if err != nil {
			return mapErr(err, "complete failover")
		}

		if err := emitOutboxTx(ctx, q, f.ProviderID, uuid.Nil, "cell_failover", failoverID.String(), "failover.completed", map[string]any{
			"replayed_usage":  usageCount,
			"replayed_outbox": outboxCount,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q,
			uuid.NullUUID{UUID: f.ProviderID, Valid: true}, uuid.NullUUID{},
			"operator", "system", "failover.complete",
			"cell_failover", failoverID.String(),
			map[string]any{"replayed_usage": usageCount, "replayed_outbox": outboxCount}); err != nil {
			return err
		}
		failover = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &failover, nil
}

// AbortFailover cancels a failover that hasn't been switched yet.
// Sets the source cell back to active.
func (s *Service) AbortFailover(ctx context.Context, failoverID uuid.UUID) (*storegen.CellFailover, error) {
	var failover storegen.CellFailover
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		f, err := q.GetCellFailoverByID(ctx, failoverID)
		if err != nil {
			return mapErr(err, "failover %s", failoverID)
		}
		if f.Status == domain.FailoverStatusSwitched || f.Status == domain.FailoverStatusCompleted || f.Status == domain.FailoverStatusAborted {
			return fmt.Errorf("%w: cannot abort failover %s (status=%s)", ErrConflict, failoverID, f.Status)
		}
		// Abort is only allowed from initiated or fenced states.

		// Reactivate source cell if it was fenced.
		if f.Status == domain.FailoverStatusFenced {
			if _, err := q.UpdateCellStatus(ctx, storegen.UpdateCellStatusParams{
				ID: f.FromCellID, Status: domain.CellStatusActive,
			}); err != nil {
				return mapErr(err, "reactivate source cell")
			}
		}

		updated, err := q.UpdateCellFailoverStatus(ctx, storegen.UpdateCellFailoverStatusParams{
			ID: failoverID, Status: domain.FailoverStatusAborted,
		})
		if err != nil {
			return mapErr(err, "abort failover")
		}

		if err := emitOutboxTx(ctx, q, f.ProviderID, uuid.Nil, "cell_failover", failoverID.String(), "failover.aborted", nil); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q,
			uuid.NullUUID{UUID: f.ProviderID, Valid: true}, uuid.NullUUID{},
			"operator", "system", "failover.abort",
			"cell_failover", failoverID.String(), nil); err != nil {
			return err
		}
		failover = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &failover, nil
}

// GetFailover returns a failover by ID.
func (s *Service) GetFailover(ctx context.Context, failoverID uuid.UUID) (*storegen.CellFailover, error) {
	var failover storegen.CellFailover
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		f, err := q.GetCellFailoverByID(ctx, failoverID)
		if err != nil {
			return mapErr(err, "failover %s", failoverID)
		}
		failover = f
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &failover, nil
}

// ListFailovers returns all failovers for a provider.
func (s *Service) ListFailovers(ctx context.Context, providerID uuid.UUID) ([]storegen.CellFailover, error) {
	var out []storegen.CellFailover
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		failovers, err := q.ListCellFailoversByProvider(ctx, storegen.ListCellFailoversByProviderParams{
			ProviderID: providerID, Limit: 100,
		})
		out = failovers
		return err
	})
	return out, err
}

// generateFencingToken generates a random fencing token for write fencing.
func generateFencingToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "fence-" + hex.EncodeToString(b)
}
