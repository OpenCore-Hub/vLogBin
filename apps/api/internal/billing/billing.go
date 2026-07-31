// Package billing defines the outbound billing adapter port. The platform
// owns the stable usage contract; Lago (or any other engine) is an internal
// execution engine reached only through this adapter (spec decision #18).
package billing

import (
	"context"
	"fmt"
	"time"
)

// UsageEvent is the stable platform usage contract delivered to the billing
// engine. The field names mirror the outbox payload of usage.accepted /
// usage.reversed events, so the relay can decode payloads directly.
type UsageEvent struct {
	TransactionID      string         `json:"transaction_id"`
	Code               string         `json:"code"`
	ExternalCustomerID string         `json:"external_customer_id"`
	Timestamp          time.Time      `json:"timestamp"`
	Properties         map[string]any `json:"properties,omitempty"`
}

// FeeSync is a single invoice line synced from the billing engine.
type FeeSync struct {
	LagoFeeID          string
	ItemCode           string // metric code or plan code
	ItemType           string // charge, subscription, add_on, credit
	ItemName           string
	Units              string
	PreciseUnitAmount  string
	AmountCents        int64
	TaxesAmountCents   int64
	TotalAmountCents   int64
	Currency           string
	EventTransactionID string
	FromDate           *time.Time
	ToDate             *time.Time
}

// InvoiceSync is a complete invoice synced from the billing engine.
type InvoiceSync struct {
	LagoID                 string
	Number                 string
	IssuingDate            time.Time
	InvoiceType            string
	Status                 string
	PaymentStatus          string
	Currency               string
	FeesAmountCents        int64
	CouponsAmountCents     int64
	CreditNotesAmountCents int64
	SubTotalExclTaxCents   int64
	TaxesAmountCents       int64
	SubTotalInclTaxCents   int64
	TotalAmountCents       int64
	FileURL                string
	WebURL                 string
	LagoCreatedAt          *time.Time
	ExternalCustomerID     string
	ExternalSubscriptionID string
	Fees                   []FeeSync
}

// Adapter delivers accepted usage to the billing engine and pulls invoices
// back for the invoice sync pipeline. Implementations must be safe to call
// concurrently.
type Adapter interface {
	Name() string
	DeliverUsageEvent(ctx context.Context, ev UsageEvent) error
	// ListInvoices returns one page of invoices (max 50) from the billing
	// engine along with the next page number (0 when there are no more
	// pages). page starts at 1.
	ListInvoices(ctx context.Context, page int32) ([]InvoiceSync, int32, error)
}

// Adapter kinds (config BILLING_ADAPTER).
const (
	KindNoop = "noop"
	KindLago = "lago"
)

// New builds the configured adapter. kind defaults to noop; lago requires
// the API URL and key.
func New(kind, lagoAPIURL, lagoAPIKey string) (Adapter, error) {
	switch kind {
	case "", KindNoop:
		return NewNoop(nil), nil
	case KindLago:
		if lagoAPIURL == "" || lagoAPIKey == "" {
			return nil, fmt.Errorf("BILLING_ADAPTER=lago requires LAGO_API_URL and LAGO_API_KEY")
		}
		return NewLago(lagoAPIURL, lagoAPIKey), nil
	default:
		return nil, fmt.Errorf("unknown BILLING_ADAPTER %q", kind)
	}
}
