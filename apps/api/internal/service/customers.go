package service

import (
	"context"
	"fmt"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/domain"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/jackc/pgx/v5"
)

// CreateCustomer registers a B2B (business) or B2C (individual) customer
// account. external_id is unique per provider environment.
func (s *Service) CreateCustomer(ctx context.Context, tc tenant.Ctx, externalID, accountType, displayName string) (*storegen.CustomerAccount, error) {
	if externalID == "" {
		return nil, fmt.Errorf("%w: external_id is required", ErrValidation)
	}
	if !domain.ValidAccountType(accountType) {
		return nil, fmt.Errorf("%w: account_type must be individual or business", ErrValidation)
	}
	var out storegen.CustomerAccount
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		customer, err := q.InsertCustomerAccount(ctx, storegen.InsertCustomerAccountParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			ExternalID: externalID, AccountType: accountType, DisplayName: displayName,
		})
		if err != nil {
			return mapErr(err, "customer %q", externalID)
		}
		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "customer_account", customer.ID.String(), "customer.created", map[string]any{
			"customer_account_id": customer.ID.String(), "external_id": externalID, "account_type": accountType,
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "customer.create", "customer_account", customer.ID.String(),
			map[string]any{"external_id": externalID, "account_type": accountType}); err != nil {
			return err
		}
		out = customer
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) ListCustomers(ctx context.Context, tc tenant.Ctx, limit int32) ([]storegen.CustomerAccount, error) {
	var out []storegen.CustomerAccount
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		cs, err := q.ListCustomerAccounts(ctx, storegen.ListCustomerAccountsParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Limit: limit,
		})
		out = cs
		return err
	})
	return out, err
}
