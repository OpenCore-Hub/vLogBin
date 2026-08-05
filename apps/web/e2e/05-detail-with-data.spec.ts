import { test, expect, api } from "./helpers";

/**
 * E2E-05: Provider detail page WITH data — human creates a provider,
 * adds catalog/subscriptions/customers/usage via API (using the
 * provider's own API key), then verifies the detail page displays
 * them in the tables.
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

async function createPublishedCatalog(
  apiKey: string,
): Promise<{ versionId: string; planCode: string; metricCode: string }> {
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

  return { versionId, planCode: "starter", metricCode: "api_calls" };
}

async function createCustomerAndSubscription(
  apiKey: string,
  versionId: string,
  planCode: string,
): Promise<string> {
  const custExt = `cust-${Date.now()}`;
  const c = await api(
    "POST",
    "/v1/customers",
    {
      external_id: custExt,
      account_type: "business",
      display_name: "Test Customer",
    },
    apiKey,
  );
  expect(c.status, `create customer: ${JSON.stringify(c.body)}`).toBe(201);

  const s = await api(
    "POST",
    "/v1/subscriptions",
    {
      external_id: `sub-${Date.now()}`,
      customer_external_id: custExt,
      catalog_version_id: versionId,
      plan_code: planCode,
    },
    apiKey,
  );
  expect(s.status, `create subscription: ${JSON.stringify(s.body)}`).toBe(201);
  return custExt;
}

test.describe("Detail page with data", () => {
  test("catalog version appears in the 目录 tab", async ({
    page,
    freshProvider,
  }) => {
    await createPublishedCatalog(freshProvider.apiKey);

    await page.goto(`/ops/${freshProvider.id}`);
    await page.getByRole("tab", { name: /目录/ }).click();

    await expect(page.getByText("v1", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("api_calls")).toBeVisible();
    // plans 表同时有 code 列 "starter" 和名称列 "Starter"，exact 匹配避免歧义。
    await expect(page.getByText("starter", { exact: true })).toBeVisible();
  });

  test("customer and subscription appear in their tables", async ({
    page,
    freshProvider,
  }) => {
    const { versionId, planCode } = await createPublishedCatalog(
      freshProvider.apiKey,
    );
    const custExt = await createCustomerAndSubscription(
      freshProvider.apiKey,
      versionId,
      planCode,
    );

    await page.goto(`/ops/${freshProvider.id}`);
    await page.getByRole("tab", { name: /订阅/ }).click();
    // tab 面板都保留在 DOM 中（未选中时 hidden），必须过滤可见元素。
    await expect(
      page.getByText(custExt, { exact: true }).filter({ visible: true }).first(),
    ).toBeVisible();

    await page.getByRole("tab", { name: /客户/ }).click();
    await expect(
      page.getByText(custExt, { exact: true }).filter({ visible: true }).first(),
    ).toBeVisible();
  });

  test("usage event appears in the 用量 tab", async ({
    page,
    freshProvider,
  }) => {
    const { versionId, planCode, metricCode } = await createPublishedCatalog(
      freshProvider.apiKey,
    );
    const custExt = await createCustomerAndSubscription(
      freshProvider.apiKey,
      versionId,
      planCode,
    );

    const txId = `tx-${Date.now()}`;
    const u = await api(
      "POST",
      "/v1/usage/ingest",
      {
        transaction_id: txId,
        customer_external_id: custExt,
        metric_code: metricCode,
        timestamp: new Date().toISOString(),
        properties: { count: 1 },
      },
      freshProvider.apiKey,
    );
    expect(u.status, `ingest usage: ${JSON.stringify(u.body)}`).toBe(201);

    await page.goto(`/ops/${freshProvider.id}`);
    await page.getByRole("tab", { name: /用量/ }).click();

    await expect(page.getByText(txId, { exact: true }).first()).toBeVisible();
  });
});
