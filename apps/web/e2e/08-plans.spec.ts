import { test, expect, api } from "./helpers";

/**
 * E2E-08: Plans page — a human seeds a published catalog with a starter
 * plan, then creates / edits / deletes a plan through the Console UI.
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

async function seedPublishedCatalog(apiKey: string): Promise<void> {
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

test.describe("Plans page", () => {
  test("create, edit and delete a plan through the Console", async ({
    page,
    freshProvider,
  }) => {
    await seedPublishedCatalog(freshProvider.apiKey);

    await page.goto("/console/billing/plans");

    // Seeded starter plan is visible with its fixed price.
    await expect(page.getByRole("heading", { name: "套餐" })).toBeVisible();
    await expect(page.getByText("Starter", { exact: true })).toBeVisible();
    await expect(page.getByText("starter", { exact: true })).toBeVisible();
    await expect(page.getByText("每月", { exact: true }).first()).toBeVisible();

    // Create a per-unit plan referencing the cloned api_calls metric.
    await page.getByRole("button", { name: "创建套餐" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("套餐代码").fill("pro");
    await dialog.getByLabel("套餐名称").fill("Pro");
    await dialog.getByLabel("货币").fill("USD");

    await dialog.getByLabel("计费模型").selectOption("per_unit");
    await dialog.getByLabel("计费指标").selectOption("api_calls");
    await dialog.getByLabel("单价").fill("0.05");
    await dialog.getByRole("button", { name: "创建套餐" }).click();

    await expect(
      page.getByText("套餐创建成功", { exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "返回套餐列表" }).click();

    await expect(page.getByText("Pro", { exact: true })).toBeVisible();
    await expect(page.getByText("pro", { exact: true })).toBeVisible();

    // Edit the new plan's name.
    await page.getByRole("button", { name: "Pro 操作" }).click();
    await page.getByRole("menuitem", { name: /编辑套餐/ }).click();
    const editDialog = page.getByRole("dialog");
    await editDialog.getByLabel("套餐名称").fill("Pro Plus");
    await editDialog.getByRole("button", { name: "保存修改" }).click();
    await expect(page.getByText("套餐已更新", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "返回套餐列表" }).click();

    await expect(page.getByText("Pro Plus", { exact: true })).toBeVisible();
    await expect(page.getByText("pro", { exact: true })).toBeVisible();

    // Delete the plan (type-to-confirm with the plan name).
    await page.getByRole("button", { name: "Pro Plus 操作" }).click();
    await page.getByRole("menuitem", { name: /删除套餐/ }).click();
    const deleteDialog = page.getByRole("dialog");
    await deleteDialog.getByLabel(/输入/).fill("Pro Plus");
    await deleteDialog.getByTestId("confirm-dialog-confirm").click();

    await expect(page.getByText("Pro Plus", { exact: true })).toHaveCount(0);
    await expect(page.getByText("Starter", { exact: true })).toBeVisible();
  });
});
