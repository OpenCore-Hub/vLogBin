package service

import (
	"context"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/google/uuid"
)

// GrantCapability grants a Live capability to a provider. Only operators
// can call this (WithOperator). If the capability is already granted, the
// reason and granted_by are updated (idempotent).
func (s *Service) GrantCapability(ctx context.Context, providerID uuid.UUID, capability, grantedBy, reason string) (*storegen.ProviderCapability, error) {
	if grantedBy == "" {
		return nil, fmt.Errorf("%w: granted_by is required", ErrValidation)
	}
	return s.upsertCapability(ctx, providerID, capability, "granted", grantedBy, reason)
}

// RevokeCapability revokes a previously granted capability. Only operators
// can call this. The original granted_by is preserved by the database
// (UpsertProviderCapability CASE expression keeps it for non-grant status).
func (s *Service) RevokeCapability(ctx context.Context, providerID uuid.UUID, capability, reason string) (*storegen.ProviderCapability, error) {
	return s.upsertCapability(ctx, providerID, capability, "revoked", "", reason)
}

// upsertCapability is the shared implementation for grant and revoke.
func (s *Service) upsertCapability(ctx context.Context, providerID uuid.UUID, capability, status, grantedBy, reason string) (*storegen.ProviderCapability, error) {
	if !domain.ValidCapability(capability) {
		return nil, fmt.Errorf("%w: unknown capability %q", ErrValidation, capability)
	}
	var cap storegen.ProviderCapability
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		if _, err := q.GetProviderByID(ctx, providerID); err != nil {
			return mapErr(err, "provider %s", providerID)
		}
		c, err := q.UpsertProviderCapability(ctx, storegen.UpsertProviderCapabilityParams{
			ProviderID: providerID,
			Capability: capability,
			Status:     status,
			GrantedBy:  grantedBy,
			Reason:     reason,
		})
		if err != nil {
			return mapErr(err, "capability %s", capability)
		}
		cap = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &cap, nil
}

// ListCapabilitiesByProvider returns all capability grants for a provider.
// Operator-scoped (WithOperator).
func (s *Service) ListCapabilitiesByProvider(ctx context.Context, providerID uuid.UUID) ([]storegen.ProviderCapability, error) {
	var caps []storegen.ProviderCapability
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		c, err := q.ListProviderCapabilities(ctx, providerID)
		if err != nil {
			return mapErr(err, "capabilities for provider %s", providerID)
		}
		caps = c
		return nil
	})
	return caps, err
}

// ListMyCapabilities returns the calling provider's own capabilities.
// Tenant-scoped (WithTenant).
func (s *Service) ListMyCapabilities(ctx context.Context, tc tenant.Ctx) ([]storegen.ProviderCapability, error) {
	var caps []storegen.ProviderCapability
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		c, err := q.ListProviderCapabilities(ctx, tc.ProviderID)
		if err != nil {
			return mapErr(err, "capabilities")
		}
		caps = c
		return nil
	})
	return caps, err
}
