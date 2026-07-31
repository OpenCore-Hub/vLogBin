package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// CreateCellInput is the parameter bundle for creating a cell.
type CreateCellInput struct {
	RegionID       uuid.UUID
	Code           string
	CellType       string
	Status         string
	CapacityLimits map[string]any
}

// CreateCell creates a new regional cell (operator-only). Cells are
// deployment units that host provider data in a specific region.
// Shared cells serve multiple providers; dedicated cells serve one.
func (s *Service) CreateCell(ctx context.Context, in CreateCellInput) (*storegen.Cell, error) {
	if in.Code == "" {
		return nil, fmt.Errorf("%w: code is required", ErrValidation)
	}
	if !domain.ValidCellType(in.CellType) {
		return nil, fmt.Errorf("%w: cell_type must be shared or dedicated", ErrValidation)
	}
	if in.Status == "" {
		in.Status = domain.CellStatusActive
	}
	if !domain.ValidCellStatus(in.Status) {
		return nil, fmt.Errorf("%w: status must be active, draining or inactive", ErrValidation)
	}

	capBytes, err := json.Marshal(in.CapacityLimits)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid capacity_limits: %v", ErrValidation, err)
	}

	var cell storegen.Cell
	err = s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		c, err := q.CreateCell(ctx, storegen.CreateCellParams{
			RegionID:       in.RegionID,
			Code:           in.Code,
			CellType:       in.CellType,
			Status:         in.Status,
			CapacityLimits: capBytes,
		})
		if err != nil {
			return mapErr(err, "cell %q", in.Code)
		}
		if err := emitOutboxTx(ctx, q, uuid.Nil, uuid.Nil, "cell", c.ID.String(), "cell.created", map[string]any{
			"code": in.Code, "cell_type": in.CellType, "region_id": in.RegionID,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{}, uuid.NullUUID{},
			"operator", "system", "cell.create",
			"cell", c.ID.String(),
			map[string]any{"code": in.Code, "cell_type": in.CellType}); err != nil {
			return err
		}
		cell = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cell, nil
}

// GetCell returns a cell by ID (operator-only).
func (s *Service) GetCell(ctx context.Context, cellID uuid.UUID) (*storegen.Cell, error) {
	var cell storegen.Cell
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		c, err := q.GetCellByID(ctx, cellID)
		if err != nil {
			return mapErr(err, "cell %s", cellID)
		}
		cell = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cell, nil
}

// ListCellsByRegion returns active cells in a specific region.
func (s *Service) ListCellsByRegion(ctx context.Context, regionID uuid.UUID) ([]storegen.Cell, error) {
	var out []storegen.Cell
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		cells, err := q.ListCellsByRegion(ctx, regionID)
		out = cells
		return err
	})
	return out, err
}

// UpdateCellStatus updates a cell's status (active/draining/inactive).
// Draining cells stop accepting new providers; inactive cells stop all traffic.
func (s *Service) UpdateCellStatus(ctx context.Context, cellID uuid.UUID, status string) (*storegen.Cell, error) {
	if !domain.ValidCellStatus(status) {
		return nil, fmt.Errorf("%w: status must be active, draining or inactive", ErrValidation)
	}

	var cell storegen.Cell
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		c, err := q.UpdateCellStatus(ctx, storegen.UpdateCellStatusParams{
			ID:     cellID,
			Status: status,
		})
		if err != nil {
			return mapErr(err, "update cell %s", cellID)
		}
		if err := emitOutboxTx(ctx, q, uuid.Nil, uuid.Nil, "cell", cellID.String(), "cell.status_changed", map[string]any{
			"status": status,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, uuid.NullUUID{}, uuid.NullUUID{},
			"operator", "system", "cell.status_change",
			"cell", cellID.String(),
			map[string]any{"status": status}); err != nil {
			return err
		}
		cell = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cell, nil
}

// AssignProviderCell assigns a provider to a cell. This is used during
// provider creation (assigning to the shared cell in their Home Region)
// or during cell migration (moving to a dedicated cell).
func (s *Service) AssignProviderCell(ctx context.Context, providerID, cellID uuid.UUID) error {
	return s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		// Verify the cell exists and is active.
		cell, err := q.GetCellByID(ctx, cellID)
		if err != nil {
			return mapErr(err, "cell %s", cellID)
		}
		if cell.Status != domain.CellStatusActive {
			return fmt.Errorf("%w: cell %s is not active (status=%s)", ErrConflict, cellID, cell.Status)
		}

		if err := q.UpdateProviderCell(ctx, storegen.UpdateProviderCellParams{
			ID:     providerID,
			CellID: uuid.NullUUID{UUID: cellID, Valid: true},
		}); err != nil {
			return mapErr(err, "assign provider %s to cell %s", providerID, cellID)
		}

		if err := emitOutboxTx(ctx, q, providerID, uuid.Nil, "provider", providerID.String(), "provider.cell_assigned", map[string]any{
			"cell_id": cellID.String(), "cell_code": cell.Code,
		}); err != nil {
			return err
		}
		return insertAuditTx(ctx, q,
			uuid.NullUUID{UUID: providerID, Valid: true}, uuid.NullUUID{},
			"operator", "system", "provider.cell_assign",
			"cell", cellID.String(),
			map[string]any{"provider_id": providerID.String()})
	})
}

// GetProviderCell returns the cell assigned to a provider. Providers
// can call this to learn their cell and region information.
func (s *Service) GetProviderCell(ctx context.Context, tc tenant.Ctx) (*storegen.Cell, error) {
	var cell storegen.Cell
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		c, err := q.GetCellByProviderID(ctx, tc.ProviderID)
		if err != nil {
			return mapErr(err, "cell for provider %s", tc.ProviderID)
		}
		cell = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cell, nil
}

// CheckCellDraining returns ErrCellDraining if the provider's cell is
// in 'draining' status. This enforces write fencing during failover
// and cell migration (spec Section 14: "写 fencing"). Called at the
// beginning of billing operations alongside CheckCutoverLock.
func (s *Service) CheckCellDraining(ctx context.Context, tc tenant.Ctx) error {
	cell, err := s.GetProviderCell(ctx, tc)
	if err != nil {
		// If no cell is assigned, skip the check (backward compatibility).
		return nil
	}
	if cell.Status == domain.CellStatusDraining {
		return ErrCellDraining
	}
	return nil
}
