import { test, expect, api } from "./helpers";

/**
 * E2E-27: Usage control plane — usage event summary and table render real
 * ingested usage data.
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

  const val = await api(
    "POST",
    `/v1/catalog/versions/${versionId}/validate`,
    undefined,
    apiKey,
  );
  expect(val.status, `validate: ${JSON.stringify(val.body)}`).toBe(200);

  const pub = await api(
    "POST",
    `/v1/catalog/versions/${versionId}/publish`,
    undefined,
    apiKey,
  );
  expect(pub.status, `publish: ${JSON.stringify(pub.body)}`).toBe(200);

  const customerExt = `usage-cust-${Date.now()}`;
  const cust = await api(
    "POST",
    "/v1/customers",
    {
      external_id: customerExt,
      account_type: "business",
      display_name: "Usage Customer",
    },
    apiKey,
  );
  expect(cust.status, `create customer: ${JSON.stringify(cust.body)}`).toBe(201);

  const sub = await api(
    "POST",
    "/v1/subscriptions",
    {
      external_id: `usage-sub-${Date.now()}`,
      customer_external_id: customerExt,
      catalog_version_id: versionId,
      plan_code: "starter",
    },
    apiKey,
  );
  expect(sub.status, `create subscription: ${JSON.stringify(sub.body)}`).toBe(201);

  const txId = `usage-tx-${Date.now()}`;
  const u = await api(
    "POST",
    "/v1/usage/ingest",
    {
      transaction_id: txId,
      customer_external_id: customerExt,
      metric_code: "api_calls",
      timestamp: new Date().toISOString(),
      properties: { count: 3 },
    },
    apiKey,
  );
  expect(u.status, `ingest usage: ${JSON.stringify(u.body)}`).toBe(201);
  return txId;
}

test.describe("Usage control plane", () => {
  test("renders usage summary and ingested events", async ({
    page,
    freshProvider,
  }) => {
    const txId = await seedUsage(freshProvider.apiKey);
    await page.goto("/console/billing/usage?env=test");

    await expect(page.getByRole("heading", { name: "用量" })).toBeVisible();
    await expect(page.getByText("事件数", { exact: true })).toBeVisible();
    await expect(page.getByText(txId, { exact: true })).toBeVisible();
    await expect(page.getByText("api_calls", { exact: true })).toBeVisible();
  });
});
