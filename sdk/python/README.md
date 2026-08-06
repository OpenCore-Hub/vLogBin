# vLogBin Python SDK

Official Python client for the vLogBin Platform API. The package uses only the
Python standard library and supports Python 3.9+.

## Install

```bash
pip install ./sdk/python
```

## Usage

```python
from vlogbin import VLogBinClient, create_customer, ingest_usage, stream_events

client = VLogBinClient("https://api.vlogbin.com/v1", "vlb_test_...")

customer = create_customer(client, {
    "external_id": "acct-123",
    "account_type": "business",
    "display_name": "Example Corp",
})

result = ingest_usage(client, {
    "transaction_id": "tx-123",
    "customer_external_id": "acct-123",
    "metric_code": "api_calls",
    "timestamp": "2026-01-01T00:00:00Z",
    "properties": {"count": 1},
}, idempotency_key="tx-123")
```

Use the same `Idempotency-Key` when retrying a mutating request. Retries with a
completed key replay the original response instead of applying the request twice.

## Error contract

All non-2xx responses raise `ApiError` with `status`, `code`, `message`,
`request_id`, `retry_after`, and `details`. Include `request_id` when contacting
support.

## Webhook verification

```python
from vlogbin import verify_webhook_signature_within

ok = verify_webhook_signature_within(
    secret,
    request.headers["X-Webhook-Timestamp"],
    request.body,
    request.headers["X-Webhook-Signature"],
)
```

## Compatibility

The SDK tracks the platform `v1` API and follows the 12-month parallel
compatibility policy. Breaking SDK changes require a new major version.
