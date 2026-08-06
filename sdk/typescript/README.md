# vLogBin TypeScript SDK

Official TypeScript/Node client for the vLogBin Platform API. Requires Node 18
or newer and ships as an ESM package with generated type declarations.

## Install

```bash
npm install ./sdk/typescript
```

## Usage

```ts
import {
  VLogBinClient,
  createCustomer,
  ingestUsage,
  streamEvents,
} from "@vlogbin/platform-sdk";

const client = new VLogBinClient("https://api.vlogbin.com/v1", "vlb_test_...");

await createCustomer(client, {
  external_id: "acct-123",
  account_type: "business",
  display_name: "Example Corp",
});

await ingestUsage(
  client,
  {
    transaction_id: "tx-123",
    customer_external_id: "acct-123",
    metric_code: "api_calls",
    timestamp: new Date().toISOString(),
    properties: { count: 1 },
  },
  "tx-123",
);

let cursor: string | undefined;
do {
  const page = await streamEvents(client, { cursor, limit: 100 });
  cursor = page.next_cursor ?? undefined;
} while (cursor);
```

Pass the same `Idempotency-Key` when retrying a mutating request. Completed
requests replay the original response instead of applying the mutation twice.

## Error contract

All non-2xx responses reject with `ApiError`, which carries `status`, `code`,
`message`, `requestId`, `retryAfter`, and `details`. Include `requestId` when
contacting support.

## Webhook verification

```ts
import { verifyWebhookSignatureWithin } from "@vlogbin/platform-sdk";

const ok = verifyWebhookSignatureWithin(
  secret,
  request.headers["x-webhook-timestamp"],
  rawBody,
  request.headers["x-webhook-signature"],
);
```

## Compatibility

The SDK tracks the platform `v1` API and follows the 12-month parallel
compatibility policy. Breaking SDK changes require a new major version.

