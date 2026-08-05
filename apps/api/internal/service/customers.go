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

// CustomerDetail is the Console-facing customer detail payload: the customer
// plus its subscriptions, usage events and invoices in the same environment.
// One request carries all data the detail page needs, with DB-side filtering
// by customer id instead of shipping the provider's entire billing dataset.
type CustomerDetail struct {
	Customer      storegen.CustomerAccount                  `json:"customer"`
	Subscriptions []storegen.ListSubscriptionsByCustomerRow `json:"subscriptions"`
	UsageEvents   []storegen.ListUsageEventsByCustomerRow   `json:"usage_events"`
	Invoices      []storegen.ListInvoicesByCustomerRow      `json:"invoices"`
}

const customerDetailLimit = 100

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

// GetCustomerDetail returns one customer with its subscriptions, usage events
// and invoices, all scoped to the tenant (provider + environment). Unknown
// customers yield ErrNotFound so the Console can surface a clear 404.
func (s *Service) GetCustomerDetail(ctx context.Context, tc tenant.Ctx, externalID string) (*CustomerDetail, error) {
	var out CustomerDetail
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		customer, err := q.GetCustomerByExternalID(ctx, storegen.GetCustomerByExternalIDParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ExternalID: externalID,
		})
		if err != nil {
			return mapErr(err, "customer %q", externalID)
		}
		subs, err := q.ListSubscriptionsByCustomer(ctx, storegen.ListSubscriptionsByCustomerParams{
			CustomerAccountID: customer.ID, Limit: customerDetailLimit,
		})
		if err != nil {
			return err
		}
		events, err := q.ListUsageEventsByCustomer(ctx, storegen.ListUsageEventsByCustomerParams{
			CustomerAccountID: customer.ID, Limit: customerDetailLimit,
		})
		if err != nil {
			return err
		}
		invs, err := q.ListInvoicesByCustomer(ctx, storegen.ListInvoicesByCustomerParams{
			CustomerAccountID: customer.ID, Limit: customerDetailLimit,
		})
		if err != nil {
			return err
		}
		out = CustomerDetail{
			Customer:      customer,
			Subscriptions: emptyIfNil(subs),
			UsageEvents:   emptyIfNil(events),
			Invoices:      emptyIfNil(invs),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
