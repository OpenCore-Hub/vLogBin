package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Lago delivers usage events to a Lago instance over HTTP
// (POST {apiURL}/api/v1/events). Non-2xx responses are errors so the relay
// can schedule a retry; the caller owns idempotency via transaction_id.
type Lago struct {
	apiURL string
	apiKey string
	client *http.Client
}

// NewLago returns the Lago adapter with a 10s request timeout.
func NewLago(apiURL, apiKey string) *Lago {
	return &Lago{
		apiURL: strings.TrimRight(apiURL, "/"),
		apiKey: apiKey,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (l *Lago) Name() string { return KindLago }

type lagoEventPayload struct {
	Event lagoEvent `json:"event"`
}

type lagoEvent struct {
	TransactionID      string         `json:"transaction_id"`
	Code               string         `json:"code"`
	ExternalCustomerID string         `json:"external_customer_id"`
	Timestamp          string         `json:"timestamp"`
	Properties         map[string]any `json:"properties,omitempty"`
}

func (l *Lago) DeliverUsageEvent(ctx context.Context, ev UsageEvent) error {
	body, err := json.Marshal(lagoEventPayload{Event: lagoEvent{
		TransactionID:      ev.TransactionID,
		Code:               ev.Code,
		ExternalCustomerID: ev.ExternalCustomerID,
		Timestamp:          ev.Timestamp.UTC().Format(time.RFC3339),
		Properties:         ev.Properties,
	}})
	if err != nil {
		return fmt.Errorf("marshal lago event: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.apiURL+"/api/v1/events", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build lago request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.apiKey)
	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("deliver to lago: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("lago returned status %d", resp.StatusCode)
	}
	return nil
}

// ---- invoice list (sync pull) ----

type lagoInvoiceListResponse struct {
	Invoices []lagoInvoice `json:"invoices"`
	Meta     lagoMeta      `json:"meta"`
}

type lagoMeta struct {
	CurrentPage int32 `json:"current_page"`
	NextPage    int32 `json:"next_page"`
	TotalPages  int32 `json:"total_pages"`
}

type lagoInvoice struct {
	LagoID         string     `json:"lago_id"`
	Number         string     `json:"number"`
	IssuingDate    string     `json:"issuing_date"`
	InvoiceType    string     `json:"invoice_type"`
	Status         string     `json:"status"`
	PaymentStatus  string     `json:"payment_status"`
	Currency       string     `json:"currency"`
	FeesAmountCents           json.Number `json:"fees_amount_cents"`
	CouponsAmountCents        json.Number `json:"coupons_amount_cents"`
	CreditNotesAmountCents    json.Number `json:"credit_notes_amount_cents"`
	SubTotalExclTaxCents      json.Number `json:"sub_total_excluding_taxes_amount_cents"`
	TaxesAmountCents          json.Number `json:"taxes_amount_cents"`
	SubTotalInclTaxCents      json.Number `json:"sub_total_including_taxes_amount_cents"`
	TotalAmountCents          json.Number `json:"total_amount_cents"`
	FileURL        string     `json:"file_url"`
	WebURL         string     `json:"web_url"`
	LagoCreatedAt  string     `json:"created_at"`
	Fees           []lagoFee  `json:"fees"`
	Customer       lagoCustomer      `json:"customer"`
	Subscriptions  []lagoSubscription `json:"subscriptions"`
}

type lagoCustomer struct {
	ExternalID string `json:"external_id"`
}

type lagoSubscription struct {
	ExternalID string `json:"external_id"`
}

type lagoFee struct {
	LagoFeeID          string      `json:"lago_id"`
	Item               lagoFeeItem `json:"item"`
	Units              string      `json:"units"`
	PreciseUnitAmount  string      `json:"precise_unit_amount"`
	AmountCents        json.Number `json:"amount_cents"`
	TaxesAmountCents   json.Number `json:"taxes_amount_cents"`
	TotalAmountCents   json.Number `json:"total_amount_cents"`
	Currency           string      `json:"currency"`
	EventTransactionID string      `json:"event_transaction_id"`
	FromDate           string      `json:"from_date"`
	ToDate             string      `json:"to_date"`
}

type lagoFeeItem struct {
	Code string `json:"code"`
	Type string `json:"type"`
	Name string `json:"name"`
}

// ListInvoices pulls one page of invoices (per_page=50) from Lago and maps
// each to an InvoiceSync with its fees. next_page is 0 when there are no
// more pages (Lago returns null).
func (l *Lago) ListInvoices(ctx context.Context, page int32) ([]InvoiceSync, int32, error) {
	url := fmt.Sprintf("%s/api/v1/invoices?page=%d&per_page=50", l.apiURL, page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build lago invoice list request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+l.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("lago invoice list request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, 0, fmt.Errorf("lago invoice list returned status %d", resp.StatusCode)
	}
	var body lagoInvoiceListResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, 0, fmt.Errorf("decode lago invoice list response: %w", err)
	}
	out := make([]InvoiceSync, 0, len(body.Invoices))
	for _, li := range body.Invoices {
		out = append(out, mapLagoInvoice(li))
	}
	return out, body.Meta.NextPage, nil
}

func mapLagoInvoice(li lagoInvoice) InvoiceSync {
	inv := InvoiceSync{
		LagoID:                 li.LagoID,
		Number:                 li.Number,
		InvoiceType:            li.InvoiceType,
		Status:                 li.Status,
		PaymentStatus:          li.PaymentStatus,
		Currency:               li.Currency,
		FeesAmountCents:        numToInt64(li.FeesAmountCents),
		CouponsAmountCents:     numToInt64(li.CouponsAmountCents),
		CreditNotesAmountCents: numToInt64(li.CreditNotesAmountCents),
		SubTotalExclTaxCents:   numToInt64(li.SubTotalExclTaxCents),
		TaxesAmountCents:       numToInt64(li.TaxesAmountCents),
		SubTotalInclTaxCents:   numToInt64(li.SubTotalInclTaxCents),
		TotalAmountCents:       numToInt64(li.TotalAmountCents),
		FileURL:                li.FileURL,
		WebURL:                 li.WebURL,
		ExternalCustomerID:     li.Customer.ExternalID,
	}
	if len(li.Subscriptions) > 0 {
		inv.ExternalSubscriptionID = li.Subscriptions[0].ExternalID
	}
	if li.IssuingDate != "" {
		if d, err := time.Parse("2006-01-02", li.IssuingDate); err == nil {
			inv.IssuingDate = d
		}
	}
	if li.LagoCreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, li.LagoCreatedAt); err == nil {
			inv.LagoCreatedAt = &t
		}
	}
	inv.Fees = make([]FeeSync, 0, len(li.Fees))
	for _, lf := range li.Fees {
		fee := FeeSync{
			LagoFeeID:          lf.LagoFeeID,
			ItemCode:           lf.Item.Code,
			ItemType:           lf.Item.Type,
			ItemName:           lf.Item.Name,
			Units:              lf.Units,
			PreciseUnitAmount:  lf.PreciseUnitAmount,
			AmountCents:        numToInt64(lf.AmountCents),
			TaxesAmountCents:   numToInt64(lf.TaxesAmountCents),
			TotalAmountCents:   numToInt64(lf.TotalAmountCents),
			Currency:           lf.Currency,
			EventTransactionID: lf.EventTransactionID,
		}
		if lf.FromDate != "" {
			if t, err := time.Parse(time.RFC3339, lf.FromDate); err == nil {
				fee.FromDate = &t
			}
		}
		if lf.ToDate != "" {
			if t, err := time.Parse(time.RFC3339, lf.ToDate); err == nil {
				fee.ToDate = &t
			}
		}
		inv.Fees = append(inv.Fees, fee)
	}
	return inv
}

// numToInt64 safely converts a json.Number (which Lago uses for cent amounts)
// to int64. A missing/invalid value becomes 0.
func numToInt64(n json.Number) int64 {
	if n == "" {
		return 0
	}
	i, err := n.Int64()
	if err != nil {
		return 0
	}
	return i
}
