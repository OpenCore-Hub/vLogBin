import { test, expect, api } from "./helpers";

/**
 * E2E-09: Customers page — a human seeds a customer + subscription + usage
 * via API, then verifies the list, creates a customer through the UI, and
 * opens the detail page with subscriptions / usage / invoices tabs.
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

async function seedBillingData(apiKey: string): Promise<{
  versionId: string;
  customerExt: string;
  subscriptionExt: string;
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
      display_name: "Seeded Customer",
    },
    apiKey,
  );
  expect(cust.status, `create customer: ${JSON.stringify(cust.body)}`).toBe(201);

  const subscriptionExt = `sub-${Date.now()}`;
  const sub = await api(
    "POST",
    "/v1/subscriptions",
    {
      external_id: subscriptionExt,
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

  return { versionId, customerExt, subscriptionExt, txId };
}

test.describe("Customers page", () => {
  test("list, create and inspect customer detail with related tabs", async ({
    page,
    freshProvider,
  }) => {
    const seeded = await seedBillingData(freshProvider.apiKey);

    await page.goto("/console/billing/customers");
    await expect(page.getByRole("heading", { name: "客户" })).toBeVisible();
    await expect(page.getByText("Seeded Customer", { exact: true })).toBeVisible();
    await expect(page.getByText(seeded.customerExt, { exact: true })).toBeVisible();

    // Create a customer through the Console UI.
    await page.getByRole("button", { name: "创建客户" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("客户名称").fill("Acme Corp");
    await dialog.getByLabel("客户外部 ID").fill("acme-corp");
    await dialog.getByLabel("客户类型").selectOption("business");
    await dialog.getByRole("button", { name: "创建客户" }).click();

    await expect(page.getByText("客户创建成功", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "返回客户列表" }).click();
    await expect(page.getByText("Acme Corp", { exact: true })).toBeVisible();
    await expect(page.getByText("acme-corp", { exact: true })).toBeVisible();

    // Open the seeded customer's detail page.
    await page.getByText("Seeded Customer", { exact: true }).click();
    await expect(page).toHaveURL(
      new RegExp(`/console/billing/customers/${seeded.customerExt}`),
    );
    await expect(
      page.getByRole("heading", { name: "Seeded Customer" }),
    ).toBeVisible();

    // Related navigation: subscriptions tab shows the seeded subscription.
    await expect(
      page.getByRole("tab", { name: /订阅/ }),
    ).toBeVisible();
    await expect(
      page.getByText(seeded.subscriptionExt, { exact: true }),
    ).toBeVisible();

    // Usage tab shows the ingested event.
    await page.getByRole("tab", { name: /用量/ }).click();
    await expect(page.getByText(seeded.txId, { exact: true })).toBeVisible();

    // Invoices tab renders its empty state.
    await page.getByRole("tab", { name: /账单/ }).click();
    await expect(page.getByText("暂无账单", { exact: true })).toBeVisible();
  });
});
