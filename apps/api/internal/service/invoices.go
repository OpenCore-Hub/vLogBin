package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/billing"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/store/storegen"
	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// InvoiceStatusFinalized and InvoiceStatusVoided are the terminal invoice
// states: once an invoice reaches either, its financial fields and lines
// are immutable (enforced by DB triggers).
const (
	InvoiceStatusFinalized = "finalized"
	InvoiceStatusVoided    = "voided"
)

// InvoiceDetail is an invoice with its lines.
type InvoiceDetail struct {
	Invoice storegen.Invoice       `json:"invoice"`
	Lines   []storegen.InvoiceLine `json:"lines"`
}

// SyncInvoices pulls invoices from the billing adapter and upserts them
// with catalog version traceability. Only invoices whose external_customer_id
// resolves to a customer in the current tenant are stored (others are
// skipped — they belong to a different provider). Finalized/voided invoices
// already in the database are skipped (immutable). Returns the count of
// invoices stored or updated in this run.
//
// The tenant context tc determines which provider/environment invoices are
// synced into. The billing engine is provider-agnostic, so the customer
// external_id is resolved within tc via RLS.
func (s *Service) SyncInvoices(ctx context.Context, tc tenant.Ctx) (int, error) {
	if s.adapter == nil {
		return 0, nil
	}
	// Invoice freeze: block invoice sync during cell failover/migration
	// (spec Section 14: "灾难期间发票冻结"). Draining cells must not
	// accept new invoice data to prevent inconsistency during switchover.
	if err := s.CheckCellDraining(ctx, tc); err != nil {
		return 0, err
	}
	page := int32(1)
	synced := 0
	for {
		invoices, nextPage, err := s.adapter.ListInvoices(ctx, page)
		if err != nil {
			return synced, fmt.Errorf("list invoices page %d: %w", page, err)
		}
		for _, inv := range invoices {
			if inv.ExternalCustomerID == "" {
				continue
			}
			stored, err := s.syncOneInvoice(ctx, tc, inv)
			if err != nil {
				return synced, err
			}
			if stored {
				synced++
			}
		}
		if nextPage == 0 {
			break
		}
		page = nextPage
	}
	return synced, nil
}

// syncOneInvoice resolves the customer and (optionally) subscription for
// one InvoiceSync, then upserts the invoice and its lines inside a tenant
// transaction. Finalized/voided invoices already present are skipped.
// Returns true when the invoice was stored or updated.
func (s *Service) syncOneInvoice(ctx context.Context, tc tenant.Ctx, inv billing.InvoiceSync) (bool, error) {
	stored := false
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		// Resolve the customer within the tenant scope (RLS-enforced).
		// If the customer doesn't exist here, the invoice belongs to
		// another provider — skip silently.
		customer, err := q.GetCustomerByExternalID(ctx, storegen.GetCustomerByExternalIDParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ExternalID: inv.ExternalCustomerID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // not our customer
			}
			return err
		}

		// If the invoice already exists and is finalized/voided, it is
		// immutable — skip (financial fields and lines cannot change).
		existing, err := q.GetInvoiceStatusByLagoID(ctx, storegen.GetInvoiceStatusByLagoIDParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, LagoID: inv.LagoID,
		})
		if err == nil {
			if existing.Status == InvoiceStatusFinalized || existing.Status == InvoiceStatusVoided {
				return nil
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		// Resolve subscription + catalog version pin when the billing
		// engine provided an external subscription id.
		var subID uuid.NullUUID
		var catalogVersionID uuid.NullUUID
		var planID uuid.UUID
		if inv.ExternalSubscriptionID != "" {
			sub, err := q.GetSubscriptionByExternalID(ctx, storegen.GetSubscriptionByExternalIDParams{
				ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, ExternalID: inv.ExternalSubscriptionID,
			})
			if err == nil {
				subID = uuid.NullUUID{UUID: sub.ID, Valid: true}
				catalogVersionID = uuid.NullUUID{UUID: sub.CatalogVersionID, Valid: true}
				planID = sub.PlanID
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			// A missing subscription is not fatal: one-off invoices or
			// cross-engine drift still store the invoice without a pin.
		}

		// Build catalog lookup maps for per-line traceability (Testing #9):
		// each invoice line should link to the exact metric and price in
		// the catalog version that was active when the subscription was
		// created.
		var metricByCode map[string]uuid.UUID
		var priceByMetric map[uuid.UUID]uuid.UUID
		if catalogVersionID.Valid {
			metrics, err := q.ListMetricsByVersion(ctx, catalogVersionID.UUID)
			if err != nil {
				return mapErr(err, "metrics for catalog version %s", catalogVersionID.UUID)
			}
			metricByCode = make(map[string]uuid.UUID, len(metrics))
			for _, m := range metrics {
				metricByCode[m.Code] = m.ID
			}
			if planID != uuid.Nil {
				prices, err := q.ListPricesByVersion(ctx, catalogVersionID.UUID)
				if err != nil {
					return mapErr(err, "prices for catalog version %s", catalogVersionID.UUID)
				}
				priceByMetric = make(map[uuid.UUID]uuid.UUID)
				for _, p := range prices {
					if p.PlanID == planID && p.MetricID.Valid {
						// First price for this metric within the plan wins.
						if _, exists := priceByMetric[p.MetricID.UUID]; !exists {
							priceByMetric[p.MetricID.UUID] = p.ID
						}
					}
				}
			}
		}

		params := storegen.UpsertInvoiceParams{
			ProviderID:                        tc.ProviderID,
			EnvironmentID:                     tc.EnvironmentID,
			LagoID:                            inv.LagoID,
			Number:                            inv.Number,
			CustomerAccountID:                 customer.ID,
			SubscriptionID:                    subID,
			CatalogVersionID:                  catalogVersionID,
			IssuingDate:                       pgtype.Date{Time: inv.IssuingDate, Valid: !inv.IssuingDate.IsZero()},
			InvoiceType:                       inv.InvoiceType,
			Status:                            inv.Status,
			PaymentStatus:                     inv.PaymentStatus,
			Currency:                          inv.Currency,
			FeesAmountCents:                   inv.FeesAmountCents,
			CouponsAmountCents:                inv.CouponsAmountCents,
			CreditNotesAmountCents:            inv.CreditNotesAmountCents,
			SubTotalExcludingTaxesAmountCents: inv.SubTotalExclTaxCents,
			TaxesAmountCents:                  inv.TaxesAmountCents,
			SubTotalIncludingTaxesAmountCents: inv.SubTotalInclTaxCents,
			TotalAmountCents:                  inv.TotalAmountCents,
			FileUrl:                           pgtype.Text{String: inv.FileURL, Valid: inv.FileURL != ""},
			WebUrl:                            pgtype.Text{String: inv.WebURL, Valid: inv.WebURL != ""},
			LagoCreatedAt:                     inv.LagoCreatedAt,
			SyncedAt:                          time.Now(),
		}
		// Terminal-state timestamps: finalized_at is the issuing_date (the
		// date the billing engine finalized the invoice). voided_at falls
		// back to now() since Lago does not expose a separate void date.
		if inv.Status == InvoiceStatusFinalized {
			t := inv.IssuingDate
			if t.IsZero() {
				t = time.Now()
			}
			params.FinalizedAt = &t
		} else if inv.Status == InvoiceStatusVoided {
			t := time.Now()
			params.VoidedAt = &t
		}

		invoice, err := q.UpsertInvoice(ctx, params)
		if err != nil {
			return mapErr(err, "invoice lago_id %q", inv.LagoID)
		}

		// Lines are rebuilt only for non-finalized invoices: finalized
		// invoices are immutable (trigger guard rejects UPDATE/DELETE on
		// invoice_lines), so we skip the rebuild to avoid a trigger error.
		if invoice.Status != InvoiceStatusFinalized && invoice.Status != InvoiceStatusVoided {
			if err := q.DeleteInvoiceLinesByInvoice(ctx, storegen.DeleteInvoiceLinesByInvoiceParams{
				InvoiceID: invoice.ID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
			}); err != nil {
				return err
			}
			for _, fee := range inv.Fees {
				// Resolve per-line traceability: metric_id from the catalog
				// metric code, price_id from the plan's price for that metric.
				var metricID, priceID uuid.NullUUID
				if id, ok := metricByCode[fee.ItemCode]; ok {
					metricID = uuid.NullUUID{UUID: id, Valid: true}
					if pid, ok := priceByMetric[id]; ok {
						priceID = uuid.NullUUID{UUID: pid, Valid: true}
					}
				}
				if _, err := q.InsertInvoiceLine(ctx, storegen.InsertInvoiceLineParams{
					InvoiceID: invoice.ID, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
					LagoFeeID:          fee.LagoFeeID,
					MetricCode:         fee.ItemCode,
					ItemType:           fee.ItemType,
					ItemName:           fee.ItemName,
					Units:              fee.Units,
					PreciseUnitAmount:  fee.PreciseUnitAmount,
					AmountCents:        fee.AmountCents,
					TaxesAmountCents:   fee.TaxesAmountCents,
					TotalAmountCents:   fee.TotalAmountCents,
					Currency:           fee.Currency,
					EventTransactionID: pgtype.Text{String: fee.EventTransactionID, Valid: fee.EventTransactionID != ""},
					FromDate:           fee.FromDate,
					ToDate:             fee.ToDate,
					MetricID:           metricID,
					PriceID:            priceID,
				}); err != nil {
					return mapErr(err, "invoice line lago_fee_id %q", fee.LagoFeeID)
				}
			}
		}

		if err := emitOutboxTx(ctx, q, tc.ProviderID, tc.EnvironmentID, "invoice", invoice.ID.String(), "invoice.synced", map[string]any{
			"invoice_id":          invoice.ID.String(),
			"lago_id":             inv.LagoID,
			"status":              invoice.Status,
			"customer_account_id": customer.ID.String(),
		}); err != nil {
			return err
		}
		if err := insertAuditTx(ctx, q, tc.ProviderNullUUID(), tc.EnvironmentNullUUID(),
			"credential", tc.CredentialID.String(), "invoice.sync", "invoice", invoice.ID.String(),
			map[string]any{"lago_id": inv.LagoID, "status": invoice.Status}); err != nil {
			return err
		}
		stored = true
		return nil
	})
	return stored, err
}

// ListInvoices returns invoices for the current tenant, newest first.
func (s *Service) ListInvoices(ctx context.Context, tc tenant.Ctx, limit int32) ([]storegen.Invoice, error) {
	var out []storegen.Invoice
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		invs, err := q.ListInvoicesByTenant(ctx, storegen.ListInvoicesByTenantParams{
			ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID, Limit: limit,
		})
		out = invs
		return err
	})
	return out, err
}

// GetInvoice returns one invoice with its lines.
func (s *Service) GetInvoice(ctx context.Context, tc tenant.Ctx, id uuid.UUID) (*InvoiceDetail, error) {
	var out InvoiceDetail
	err := s.store.WithTenant(ctx, tc, func(tx pgx.Tx, q *store.Queries) error {
		invoice, err := q.GetInvoiceByID(ctx, storegen.GetInvoiceByIDParams{
			ID: id, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return mapErr(err, "invoice %s", id)
		}
		lines, err := q.ListInvoiceLinesByInvoice(ctx, storegen.ListInvoiceLinesByInvoiceParams{
			InvoiceID: id, ProviderID: tc.ProviderID, EnvironmentID: tc.EnvironmentID,
		})
		if err != nil {
			return err
		}
		out.Invoice = invoice
		out.Lines = emptyIfNil(lines)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ListInvoicesByProvider lists invoices across all environments of a
// provider (operator path, for the console).
func (s *Service) ListInvoicesByProvider(ctx context.Context, providerID uuid.UUID) ([]storegen.ListInvoicesByProviderRow, error) {
	var out []storegen.ListInvoicesByProviderRow
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		invs, err := q.ListInvoicesByProvider(ctx, providerID)
		out = invs
		return err
	})
	return out, err
}

// OperatorInvoiceDetail is the Console-facing invoice detail: the joined
// invoice view (customer_external_id + environment_kind resolved) plus its
// line items.
type OperatorInvoiceDetail struct {
	Invoice storegen.GetInvoiceByProviderEnvIDRow `json:"invoice"`
	Lines   []storegen.InvoiceLine                `json:"lines"`
}

// ListInvoicesByProviderEnv lists invoices of one provider environment
// (operator path, for the Console invoices page).
func (s *Service) ListInvoicesByProviderEnv(ctx context.Context, providerID, envID uuid.UUID) ([]storegen.ListInvoicesByProviderEnvRow, error) {
	var out []storegen.ListInvoicesByProviderEnvRow
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		invs, err := q.ListInvoicesByProviderEnv(ctx, storegen.ListInvoicesByProviderEnvParams{
			ProviderID: providerID, EnvironmentID: envID,
		})
		out = invs
		return err
	})
	return out, err
}

// GetInvoiceDetailByProvider returns one invoice with its lines for the
// operator Console. Unknown invoices yield ErrNotFound.
func (s *Service) GetInvoiceDetailByProvider(ctx context.Context, providerID, envID, invoiceID uuid.UUID) (*OperatorInvoiceDetail, error) {
	var out OperatorInvoiceDetail
	err := s.store.WithOperator(ctx, func(tx pgx.Tx, q *store.Queries) error {
		invoice, err := q.GetInvoiceByProviderEnvID(ctx, storegen.GetInvoiceByProviderEnvIDParams{
			ID: invoiceID, ProviderID: providerID, EnvironmentID: envID,
		})
		if err != nil {
			return mapErr(err, "invoice %s", invoiceID)
		}
		lines, err := q.ListInvoiceLinesByInvoice(ctx, storegen.ListInvoiceLinesByInvoiceParams{
			InvoiceID: invoiceID, ProviderID: providerID, EnvironmentID: envID,
		})
		if err != nil {
			return err
		}
		out.Invoice = invoice
		out.Lines = emptyIfNil(lines)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
