package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SLATierInput is the parameter bundle for creating/updating an SLA tier.
type SLATierInput struct {
	Code             string         `json:"code"`
	Name             string         `json:"name"`
	UptimeSLA        float64        `json:"uptime_sla"`
	PriorityLevel    int32          `json:"priority_level"`
	ReservedCapacity map[string]any `json:"reserved_capacity"`
}

// CreateSLATier defines a new SLA tier for the provider's environment.
func (s *Service) CreateSLATier(ctx context.Context, tc tenant.Ctx, in SLATierInput) (*storegen.SlaTier, error) {
	if in.Code == "" {
		return nil, fmt.Errorf("%w: code is required", ErrValidation)
	}
	if in.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}
	if in.UptimeSLA < 0 || in.UptimeSLA > 100 {
		return nil, fmt.Errorf("%w: uptime_sla must be between 0 and 100", ErrValidation)
	}
	if in.PriorityLevel < 1 || in.PriorityLevel > 5 {
		return nil, fmt.Errorf("%w: priority_level must be between 1 and 5", ErrValidation)
	}

	capBytes, err := json.Marshal(in.ReservedCapacity)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid reserved_capacity: %v", ErrValidation, err)
	}

	var tier storegen.SlaTier
	err = s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		t, err := q.CreateSLATier(ctx, storegen.CreateSLATierParams{
			ProviderID:       tc.ProviderID,
			EnvironmentID:    tc.EnvironmentID,
			Code:             in.Code,
			Name:             in.Name,
			UptimeSla:        in.UptimeSLA,
			PriorityLevel:    in.PriorityLevel,
			ReservedCapacity: capBytes,
		})
		if err != nil {
			return mapErr(err, "sla tier %q", in.Code)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "sla_tier", t.ID.String(), "sla.tier_created", map[string]any{
			"code": in.Code, "uptime_sla": in.UptimeSLA,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "sla.tier_create",
			"sla_tier", t.ID.String(),
			map[string]any{"code": in.Code, "uptime_sla": in.UptimeSLA}); err != nil {
			return err
		}
		tier = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &tier, nil
}

// GetSLATier returns an SLA tier by ID.
func (s *Service) GetSLATier(ctx context.Context, tc tenant.Ctx, tierID uuid.UUID) (*storegen.SlaTier, error) {
	var tier storegen.SlaTier
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		t, err := q.GetSLATierByID(ctx, tierID)
		if err != nil {
			return mapErr(err, "sla tier %s", tierID)
		}
		if err := checkTenantOwnership(t.ProviderID, t.EnvironmentID, tc, "sla tier", tierID); err != nil {
			return err
		}
		tier = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &tier, nil
}

// ListSLATiers returns all SLA tiers for the caller's tenant.
func (s *Service) ListSLATiers(ctx context.Context, tc tenant.Ctx) ([]storegen.SlaTier, error) {
	var out []storegen.SlaTier
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		tiers, err := q.ListSLATiers(ctx, storegen.ListSLATiersParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		out = tiers
		return err
	})
	return out, err
}

// UpdateSLATier updates an existing SLA tier.
func (s *Service) UpdateSLATier(ctx context.Context, tc tenant.Ctx, tierID uuid.UUID, in SLATierInput) (*storegen.SlaTier, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrValidation)
	}
	if in.UptimeSLA < 0 || in.UptimeSLA > 100 {
		return nil, fmt.Errorf("%w: uptime_sla must be between 0 and 100", ErrValidation)
	}
	if in.PriorityLevel < 1 || in.PriorityLevel > 5 {
		return nil, fmt.Errorf("%w: priority_level must be between 1 and 5", ErrValidation)
	}

	capBytes, err := json.Marshal(in.ReservedCapacity)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid reserved_capacity: %v", ErrValidation, err)
	}

	var tier storegen.SlaTier
	err = s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		existing, err := q.GetSLATierByID(ctx, tierID)
		if err != nil {
			return mapErr(err, "sla tier %s", tierID)
		}
		if err := checkTenantOwnership(existing.ProviderID, existing.EnvironmentID, tc, "sla tier", tierID); err != nil {
			return err
		}
		t, err := q.UpdateSLATier(ctx, storegen.UpdateSLATierParams{
			ID:               tierID,
			Name:             in.Name,
			UptimeSla:        in.UptimeSLA,
			PriorityLevel:    in.PriorityLevel,
			ReservedCapacity: capBytes,
		})
		if err != nil {
			return mapErr(err, "update sla tier %s", tierID)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "sla_tier", tierID.String(), "sla.tier_updated", map[string]any{
			"name": in.Name, "uptime_sla": in.UptimeSLA,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "sla.tier_update",
			"sla_tier", tierID.String(),
			map[string]any{"name": in.Name}); err != nil {
			return err
		}
		tier = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &tier, nil
}

// DeleteSLATier removes an SLA tier.
func (s *Service) DeleteSLATier(ctx context.Context, tc tenant.Ctx, tierID uuid.UUID) error {
	return s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		rows, err := q.DeleteSLATier(ctx, storegen.DeleteSLATierParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ID: tierID,
		})
		if err != nil {
			return mapErr(err, "delete sla tier %s", tierID)
		}
		if rows == 0 {
			return fmt.Errorf("%w: sla tier %s", ErrNotFound, tierID)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "sla_tier", tierID.String(), "sla.tier_deleted", nil); err != nil {
			return err
		}
		return insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "sla.tier_delete",
			"sla_tier", tierID.String(), nil)
	})
}
