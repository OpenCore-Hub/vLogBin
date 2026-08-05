import { test, expect, api } from "./helpers";

/**
 * E2E-20: Policies control plane — plan-level entitlement grants are
 * created, edited and deleted from the Console.
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
      entitlements: [
        { key: "max_users", value_type: "numeric", value: 10 },
      ],
    },
  ],
};

async function seedPublishedPlan(apiKey: string): Promise<void> {
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
}

test.describe("Policies control plane", () => {
  test("create, edit and delete a plan entitlement", async ({
    page,
    freshProvider,
  }) => {
    await seedPublishedPlan(freshProvider.apiKey);
    await page.goto("/console/identity/policies?env=test");

    await expect(page.getByRole("heading", { name: "Policies" })).toBeVisible();
    await expect(page.getByText("max_users", { exact: true })).toBeVisible();

    // Create a boolean entitlement.
    await page.getByRole("button", { name: "添加权益" }).click();
    await page.getByLabel("权益 key").fill("feature_export");
    await page.getByLabel("值", { exact: true }).selectOption("true");
    await page.getByRole("dialog").getByRole("button", { name: "添加权益", exact: true }).click();
    await expect(page.getByText("feature_export", { exact: true })).toBeVisible();

    // Edit it to a numeric value.
    await page.getByRole("button", { name: "编辑 feature_export" }).click();
    await page.getByLabel("值类型").selectOption("numeric");
    await page.getByLabel("值", { exact: true }).fill("50");
    await page.getByRole("dialog").getByRole("button", { name: "保存修改" }).click();
    await expect(page.getByText("50", { exact: true })).toBeVisible();

    // Delete with type-to-confirm.
    await page.getByRole("button", { name: "删除 feature_export" }).click();
    await page.getByLabel("输入 feature_export 确认").fill("feature_export");
    await page.getByRole("dialog").getByRole("button", { name: "删除权益" }).click();
    await expect(page.getByText("feature_export", { exact: true })).toHaveCount(0);
  });
});
