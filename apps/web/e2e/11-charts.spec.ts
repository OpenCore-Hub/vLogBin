import { test, expect, api } from "./helpers";

/**
 * E2E-11: Self-drawn SVG chart variants — verifies the Overview donut and
 * sparkline render for a fresh provider, and the customer usage tab renders
 * the line chart after a usage event is ingested.
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

async function seedCustomerUsage(apiKey: string): Promise<{
  customerExt: string;
  txId: string;
}> {
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

  const customerExt = `cust-${Date.now()}`;
  const cust = await api(
    "POST",
    "/v1/customers",
    {
      external_id: customerExt,
      account_type: "business",
      display_name: "Chart Customer",
    },
    apiKey,
  );
  expect(cust.status, `create customer: ${JSON.stringify(cust.body)}`).toBe(201);

  const sub = await api(
    "POST",
    "/v1/subscriptions",
    {
      external_id: `sub-${Date.now()}`,
      customer_external_id: customerExt,
      catalog_version_id: versionId,
      plan_code: "starter",
    },
    apiKey,
  );
  expect(sub.status, `create subscription: ${JSON.stringify(sub.body)}`).toBe(201);

  const txId = `tx-${Date.now()}`;
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

  return { customerExt, txId };
}

test.describe("Chart variants", () => {
  test("overview donut and sparkline render for a fresh provider", async ({
    page,
    freshProvider,
  }) => {
    expect(freshProvider.id).toBeTruthy();
    await page.goto("/console");

    await expect(page.getByText("Provider 状态分布", { exact: true })).toBeVisible();
    await expect(
      page.getByRole("img", { name: /环形图（\d+ 项）/ }),
    ).toBeVisible();
    await expect(
      page
        .getByRole("img", { name: /环形图（\d+ 项）/ })
        .getByText("TEST_ACTIVE", { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("img", { name: /迷你趋势图（\d+ 天）/ }),
    ).toBeVisible();
  });

  test("customer usage tab renders the line chart", async ({
    page,
    freshProvider,
  }) => {
    const seeded = await seedCustomerUsage(freshProvider.apiKey);

    await page.goto(
      `/console/billing/customers/${encodeURIComponent(seeded.customerExt)}`,
    );
    await page.getByRole("tab", { name: /用量/ }).click();

    await expect(
      page.getByText("用量趋势（按天）", { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("img", { name: "折线图（1 天）" }),
    ).toBeVisible();
    await expect(page.getByText(seeded.txId, { exact: true })).toBeVisible();
  });
});
