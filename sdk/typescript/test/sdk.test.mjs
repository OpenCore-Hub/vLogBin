import assert from "node:assert/strict";
import http from "node:http";
import test from "node:test";
import { createHmac } from "node:crypto";
import {
  ApiError,
  VLogBinClient,
  ingestUsage,
  listSubscriptions,
  verifyWebhookSignature,
} from "../dist/index.js";

test("client sends auth and idempotency headers", async () => {
  const server = http.createServer((req, res) => {
    assert.equal(req.headers.authorization, "Bearer key-1");
    assert.equal(req.headers["idempotency-key"], "tx-1");
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end('{"status":"accepted"}');
  });
  await new Promise((resolve) => server.listen(0, resolve));
  try {
    const address = server.address();
    const client = new VLogBinClient(
      `http://127.0.0.1:${address.port}`,
      "key-1",
    );
    const result = await ingestUsage(
      client,
      {
        transaction_id: "tx-1",
        customer_external_id: "cust-1",
        metric_code: "api_calls",
        timestamp: "2026-01-01T00:00:00Z",
        properties: { count: 1 },
      },
      "tx-1",
    );
    assert.equal(result.status, "accepted");
  } finally {
    server.closeAllConnections?.();
    server.close();
  }
});

test("client decodes the error envelope", async () => {
  const server = http.createServer((req, res) => {
    res.writeHead(429, { "Content-Type": "application/json" });
    res.end(
      JSON.stringify({
        error: {
          code: "rate_limited",
          message: "slow down",
          request_id: "req-1",
          retry_after: "5",
        },
      }),
    );
  });
  await new Promise((resolve) => server.listen(0, resolve));
  try {
    const address = server.address();
    const client = new VLogBinClient(
      `http://127.0.0.1:${address.port}`,
      "key",
    );
    await assert.rejects(
      client.request("GET", "/customers"),
      (err) =>
        err instanceof ApiError &&
        err.code === "rate_limited" &&
        err.requestId === "req-1" &&
        err.retryAfter === "5",
    );
  } finally {
    server.closeAllConnections?.();
    server.close();
  }
});

test("client lists subscriptions", async () => {
  const server = http.createServer((req, res) => {
    assert.equal(req.url, "/subscriptions");
    assert.equal(req.method, "GET");
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(
      JSON.stringify({
        subscriptions: [
          {
            id: "sub-1",
            external_id: "sub-1",
            customer_account_id: "cust-1",
            catalog_version_id: "cv-1",
            plan_id: "plan-1",
            status: "active",
            started_at: "2026-01-01T00:00:00Z",
            terminated_at: null,
          },
        ],
      }),
    );
  });
  await new Promise((resolve) => server.listen(0, resolve));
  try {
    const address = server.address();
    const client = new VLogBinClient(
      `http://127.0.0.1:${address.port}`,
      "key",
    );
    const subscriptions = await listSubscriptions(client);
    assert.equal(subscriptions.length, 1);
    assert.equal(subscriptions[0].status, "active");
  } finally {
    server.closeAllConnections?.();
    server.close();
  }
});

test("webhook signature verifies", () => {
  const secret = "secret";
  const timestamp = String(Math.floor(Date.now() / 1000));
  const payload = '{"event_type":"usage.accepted"}';
  const signature = createHmac("sha256", secret)
    .update(timestamp + payload)
    .digest("hex");
  assert.equal(verifyWebhookSignature(secret, timestamp, payload, signature), true);
});
