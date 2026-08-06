import { test, expect, api } from "./helpers";

/**
 * E2E-33: Queue capacity / dead-letter board — recent outbox events render
 * from real business writes.
 */
const CATALOG_CONTENT = {
  metrics: [
    {
      code: "api_calls",
      name: "API Calls",
      aggregation_type: "count",
      billable: true,
    },
  ],
  plans: [
    {
      code: "starter",
      name: "Starter",
      interval: "monthly",
      currency: "USD",
      prices: [
        {
          charge_model: "fixed",
          properties: { amount_cents: 1000, currency: "USD" },
        },
      ],
    },
  ],
};

async function seedUsage(apiKey: string): Promise<string> {
  const v = await api("POST", "/v1/catalog/versions", {}, apiKey);
  expect(v.status, `create catalog version: ${JSON.stringify(v.body)}`).toBe(201);
  const versionId = v.body.version.id;

  const c = await api(
    "PUT",
    `/v1/catalog/versions/${versionId}/content`,
    CATALOG_CONTENT,
    apiKey,
  );
  expect(c.status, `replace content: ${JSON.stringify(c.body)}`).toBe(200);
  await api("POST", `/v1/catalog/versions/${versionId}/validate`, undefined, apiKey);
  const pub = await api(
    "POST",
    `/v1/catalog/versions/${versionId}/publish`,
    undefined,
    apiKey,
  );
  expect(pub.status, `publish: ${JSON.stringify(pub.body)}`).toBe(200);

  const customerExt = `queue-cust-${Date.now()}`;
  const cust = await api(
    "POST",
    "/v1/customers",
    {
      external_id: customerExt,
      account_type: "business",
      display_name: "Queue Customer",
    },
    apiKey,
  );
  expect(cust.status, `create customer: ${JSON.stringify(cust.body)}`).toBe(201);

  const sub = await api(
    "POST",
    "/v1/subscriptions",
    {
      external_id: `queue-sub-${Date.now()}`,
      customer_external_id: customerExt,
      catalog_version_id: versionId,
      plan_code: "starter",
    },
    apiKey,
  );
  expect(sub.status, `create subscription: ${JSON.stringify(sub.body)}`).toBe(201);

  const txId = `queue-tx-${Date.now()}`;
  const u = await api(
    "POST",
    "/v1/usage/ingest",
    {
      transaction_id: txId,
      customer_external_id: customerExt,
      metric_code: "api_calls",
      timestamp: new Date().toISOString(),
      properties: { count: 1 },
    },
    apiKey,
  );
  expect(u.status, `ingest usage: ${JSON.stringify(u.body)}`).toBe(201);
  return txId;
}

test.describe("Queue capacity board", () => {
  test("renders outbox summary and recent usage event", async ({
    page,
    freshProvider,
  }) => {
    await seedUsage(freshProvider.apiKey);
    await page.goto("/console/queues");

    await expect(page.getByRole("heading", { name: "队列" })).toBeVisible();
    await expect(page.getByText("Outbox 待处理", { exact: true })).toBeVisible();
    await expect(page.getByText("Webhook 死信", { exact: true })).toBeVisible();
    await expect(page.getByText("usage.accepted", { exact: true })).toBeVisible();
  });
});
