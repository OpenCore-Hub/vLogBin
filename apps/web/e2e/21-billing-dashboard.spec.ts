import { test, expect, api } from "./helpers";

/**
 * E2E-21: Billing Dashboard — revenue/subscription/customer metrics and the
 * recent-invoice panel render from real operator data.
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

async function seedSubscription(apiKey: string): Promise<void> {
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

  const customerExt = `dash-cust-${Date.now()}`;
  const cust = await api(
    "POST",
    "/v1/customers",
    {
      external_id: customerExt,
      account_type: "business",
      display_name: "Dashboard Customer",
    },
    apiKey,
  );
  expect(cust.status, `create customer: ${JSON.stringify(cust.body)}`).toBe(201);

  const sub = await api(
    "POST",
    "/v1/subscriptions",
    {
      external_id: `dash-sub-${Date.now()}`,
      customer_external_id: customerExt,
      catalog_version_id: versionId,
      plan_code: "starter",
    },
    apiKey,
  );
  expect(sub.status, `create subscription: ${JSON.stringify(sub.body)}`).toBe(201);
}

test.describe("Billing Dashboard", () => {
  test("renders real metrics and recent invoices from the workspace", async ({
    page,
    freshProvider,
  }) => {
    await seedSubscription(freshProvider.apiKey);
    await page.goto("/console/billing/dashboard?env=test");

    await expect(
      page.getByRole("heading", { name: "Billing Dashboard" }),
    ).toBeVisible();
    await expect(page.getByText("活跃订阅", { exact: true })).toBeVisible();
    await expect(
      page.getByRole("main").getByText("客户", { exact: true }),
    ).toBeVisible();
    await expect(page.getByText("1 个套餐", { exact: true })).toBeVisible();
    await expect(page.getByText("暂无账单", { exact: true })).toBeVisible();
  });
});
