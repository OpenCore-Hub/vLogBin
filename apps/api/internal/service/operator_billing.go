package service

import (
	"context"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// GetCatalogVersionOperator returns a catalog version with its full
// content, viewed from the operator context (cross-environment). Unlike
// the tenant-scoped GetCatalogVersion, this only needs the version ID.
func (s *Service) GetCatalogVersionOperator(ctx context.Context, id uuid.UUID) (*CatalogVersionDetail, error) {
	var out CatalogVersionDetail
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		detail, err := loadCatalogDetailTx(ctx, q, id)
		if err != nil {
			return err
		}
		out = *detail
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListCustomersByProvider lists customer accounts across all environments
// of a provider (operator path, for the console).
func (s *Service) ListCustomersByProvider(ctx context.Context, providerID uuid.UUID) ([]storegen.ListCustomersByProviderRow, error) {
	var out []storegen.ListCustomersByProviderRow
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		cs, err := q.ListCustomersByProvider(ctx, providerID)
		out = cs
		return err
	})
	return out, err
}

// ListUsageEventsByProvider lists recent usage events across all
// environments of a provider (operator path, for the console).
func (s *Service) ListUsageEventsByProvider(ctx context.Context, providerID uuid.UUID) ([]storegen.ListUsageEventsByProviderRow, error) {
	var out []storegen.ListUsageEventsByProviderRow
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		evs, err := q.ListUsageEventsByProvider(ctx, providerID)
		out = evs
		return err
	})
	return out, err
}
