# vLogBin Go SDK

Official Go client for the vLogBin Platform API.

## Install

```bash
go get github.com/OpenCore-Hub/vLogBin/sdk/go
```

## Usage

```go
client := vlogbin.NewClient("https://api.vlogbin.com/v1", "vlb_test_...")

customer, err := client.CreateCustomer(ctx, vlogbin.CreateCustomerInput{
    ExternalID:  "acct-123",
    AccountType: "business",
    DisplayName: "Example Corp",
})

result, err := client.IngestUsage(ctx, vlogbin.IngestUsageInput{
    TransactionID:      "tx-123",
    CustomerExternalID: "acct-123",
    MetricCode:         "api_calls",
    Timestamp:          time.Now().UTC().Format(time.RFC3339Nano),
    Properties:         map[string]any{"count": 1},
}, vlogbin.RequestOptions{IdempotencyKey: "tx-123"})

page, err := client.StreamEvents(ctx, vlogbin.StreamEventsInput{Limit: 100})
for page.HasMore {
    page, err = client.StreamEvents(ctx, vlogbin.StreamEventsInput{
        Cursor: page.NextCursor,
        Limit:  100,
    })
}
```

## Error contract

All failures decode into `*vlogbin.APIError` with `Code`, `Message`, `RequestID`,
`RetryAfter`, and `Details`. Use `RequestID` when contacting support.

## Webhook verification

```go
ok := vlogbin.VerifyWebhookSignatureWithin(
    secret,
    r.Header.Get("X-Webhook-Timestamp"),
    payload,
    r.Header.Get("X-Webhook-Signature"),
    5*time.Minute,
)
```

## Compatibility

The SDK tracks the platform `v1` API and follows the 12-month parallel
compatibility policy. Breaking SDK changes require a new major version.
