package vlogbin

import (
	"context"
	"net/url"
	"strconv"
)

// Customer mirrors the public provider-domain customer resource.
type Customer struct {
	ID          string `json:"id"`
	ExternalID  string `json:"external_id"`
	AccountType string `json:"account_type"`
	DisplayName string `json:"display_name"`
}

// CreateCustomerInput is the public customer creation payload.
type CreateCustomerInput struct {
	ExternalID  string `json:"external_id"`
	AccountType string `json:"account_type"`
	DisplayName string `json:"display_name"`
}

// CreateCustomer creates a customer under the authenticated environment.
func (c *Client) CreateCustomer(ctx context.Context, input CreateCustomerInput) (*Customer, error) {
	var out struct {
		Customer Customer `json:"customer"`
	}
	if err := c.Do(ctx, "POST", "/customers", RequestOptions{}, input, &out); err != nil {
		return nil, err
	}
	return &out.Customer, nil
}

// Subscription mirrors the provider-domain subscription resource.
type Subscription struct {
	ID                string `json:"id"`
	ExternalID        string `json:"external_id"`
	CatalogVersionID  string `json:"catalog_version_id"`
	PlanID            string `json:"plan_id"`
	Status            string `json:"status"`
	CustomerAccountID string `json:"customer_account_id"`
}

// ListSubscriptions lists subscriptions in the authenticated environment.
func (c *Client) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	var out struct {
		Subscriptions []Subscription `json:"subscriptions"`
	}
	if err := c.Do(ctx, "GET", "/subscriptions", RequestOptions{}, nil, &out); err != nil {
		return nil, err
	}
	return out.Subscriptions, nil
}

// IngestUsageInput is the public usage ingestion payload.
type IngestUsageInput struct {
	TransactionID      string         `json:"transaction_id"`
	CustomerExternalID string         `json:"customer_external_id"`
	MetricCode         string         `json:"metric_code"`
	Timestamp          string         `json:"timestamp"`
	Properties         map[string]any `json:"properties"`
}

// UsageResult is the public ingestion result.
type UsageResult struct {
	Status string `json:"status"`
}

// IngestUsage sends one usage event. Use the same transaction ID for retries.
func (c *Client) IngestUsage(ctx context.Context, input IngestUsageInput) (*UsageResult, error) {
	var out UsageResult
	if err := c.Do(ctx, "POST", "/usage/ingest", RequestOptions{}, input, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Event mirrors the public event stream record.
type Event struct {
	ID            string `json:"id"`
	EventType     string `json:"event_type"`
	AggregateID   string `json:"aggregate_id"`
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
}

// StreamEventsInput controls event stream pagination.
type StreamEventsInput struct {
	Cursor        string
	Limit         int
	Type          string
	AggregateType string
}

// StreamResult is one page of the event stream.
type StreamResult struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
}

// StreamEvents pulls one page of events from the cursor-based stream.
func (c *Client) StreamEvents(ctx context.Context, input StreamEventsInput) (*StreamResult, error) {
	query := map[string]string{}
	if input.Cursor != "" {
		query["cursor"] = input.Cursor
	}
	if input.Limit > 0 {
		query["limit"] = strconv.Itoa(input.Limit)
	}
	if input.Type != "" {
		query["type"] = input.Type
	}
	if input.AggregateType != "" {
		query["aggregate_type"] = input.AggregateType
	}
	opts := RequestOptions{Query: url.Values{}}
	for k, v := range query {
		opts.Query.Set(k, v)
	}
	var out StreamResult
	if err := c.Do(ctx, "GET", "/events", opts, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
