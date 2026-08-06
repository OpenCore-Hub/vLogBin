import { test, expect, api } from "./helpers";

/**
 * E2E-32: Quota control plane — subscription hard quota limits can be
 * created and deleted from the Console.
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

async function seedSubscription(
  apiKey: string,
): Promise<{ subId: string; subExt: string }> {
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

  const customerExt = `quota-cust-${Date.now()}`;
  const cust = await api(
    "POST",
    "/v1/customers",
    {
      external_id: customerExt,
      account_type: "business",
      display_name: "Quota Customer",
    },
    apiKey,
  );
  expect(cust.status, `create customer: ${JSON.stringify(cust.body)}`).toBe(201);

  const subExt = `quota-sub-${Date.now()}`;
  const sub = await api(
    "POST",
    "/v1/subscriptions",
    {
      external_id: subExt,
      customer_external_id: customerExt,
      catalog_version_id: versionId,
      plan_code: "starter",
    },
    apiKey,
  );
  expect(sub.status, `create subscription: ${JSON.stringify(sub.body)}`).toBe(201);
  return { subId: sub.body.id as string, subExt };
}

test.describe("Quota control plane", () => {
  test("creates and deletes a hard quota limit", async ({
    page,
    freshProvider,
  }) => {
    const { subExt } = await seedSubscription(freshProvider.apiKey);
    await page.goto("/console/billing/quota?env=test");

    await expect(page.getByRole("heading", { name: "额度" })).toBeVisible();
    await expect(page.getByLabel("订阅")).toContainText(subExt);

    await page.getByRole("button", { name: "新建额度" }).click();
    await page.getByRole("dialog").getByLabel("额度键").fill("api_calls");
    await page.getByRole("dialog").getByLabel("上限").fill("1000");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "创建额度" })
      .click();

    const row = page.locator("tbody tr", { hasText: "api_calls" });
    await expect(row).toContainText("1,000");
    await expect(row).toContainText("每月");

    await row.getByRole("button", { name: "删除" }).click();
    await page.getByLabel("输入 api_calls 确认").fill("api_calls");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "删除额度" })
      .click();

    await expect(page.getByRole("dialog")).toBeHidden();
    await expect(page.getByText("暂无额度上限", { exact: true })).toBeVisible();
  });
});
