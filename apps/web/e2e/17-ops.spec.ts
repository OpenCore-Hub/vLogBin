import { test, expect, api } from "./helpers";

/**
 * E2E-17: 运营商台增强 (§8 M3) — Providers / 审核 / Cell 运维三个页签。
 */
test.describe("Ops enhancement", () => {
  test("provider table stays intact and new tabs render", async ({ page }) => {
    await page.goto("/ops");
    await expect(page.getByRole("heading", { name: "Providers" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "审核" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Cell 运维" })).toBeVisible();

    await page.getByRole("tab", { name: "审核" }).click();
    await expect(page.getByText("待审核", { exact: true })).toBeVisible();
    await expect(page.getByText("风险审核记录", { exact: true })).toBeVisible();
    await expect(page.getByText("支持会话", { exact: true })).toBeVisible();

    await page.getByRole("tab", { name: "Cell 运维" }).click();
    await expect(page.getByText("Cells", { exact: true })).toBeVisible();
    await expect(page.getByText("故障切换", { exact: true })).toBeVisible();
    await expect(page.getByText("Cell 迁移", { exact: true })).toBeVisible();
  });

  test("create a cell from the Cell ops tab", async ({ page }) => {
    const regions = await api("GET", "/v1/operator/regions");
    expect(regions.status).toBe(200);
    const regionId = regions.body.regions[0].id as string;
    const code = `e2e-${Date.now()}`;

    await page.goto("/ops");
    await page.getByRole("tab", { name: "Cell 运维" }).click();
    await page.getByRole("button", { name: "新建 Cell" }).click();
    await page.getByRole("dialog").getByLabel("Region").selectOption(regionId);
    await page.getByRole("dialog").getByLabel("Code").fill(code);
    await page.getByRole("dialog").getByRole("button", { name: "创建 Cell" }).click();
    await expect(
      page.getByRole("dialog").getByText("Cell 已创建", { exact: true }),
    ).toBeVisible();
  });
});
