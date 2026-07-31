package billing

import (
	"context"
	"log/slog"
)

// Noop is the default adapter: it logs the delivery and succeeds. Used when
// no billing engine is configured (development, tests).
type Noop struct {
	log *slog.Logger
}

// NewNoop returns the logging no-op adapter. A nil logger uses the default.
func NewNoop(log *slog.Logger) *Noop {
	if log == nil {
		log = slog.Default()
	}
	return &Noop{log: log}
}

func (n *Noop) Name() string { return KindNoop }

func (n *Noop) DeliverUsageEvent(_ context.Context, ev UsageEvent) error {
	n.log.Info("noop billing adapter: usage event delivered",
		"transaction_id", ev.TransactionID,
		"code", ev.Code,
		"external_customer_id", ev.ExternalCustomerID,
	)
	return nil
}

// ListInvoices returns no invoices for the noop adapter (no billing engine
// configured means no invoices to sync).
func (n *Noop) ListInvoices(_ context.Context, _ int32) ([]InvoiceSync, int32, error) {
	return nil, 0, nil
}
