import { test, expect } from "./helpers";

/**
 * E2E-01: Provider list (ops console) — human opens the Providers page
 * and sees the list of providers with their lifecycle states.
 */
test.describe("Providers list", () => {
  test("providers page renders with table", async ({ page }) => {
    await page.goto("/ops");

    await expect(
      page.getByRole("heading", { name: "Providers" }),
    ).toBeVisible();
    // header 或 EmptyState 中总有一个「新建 Provider」入口
    await expect(
      page.getByRole("link", { name: "新建 Provider" }).first(),
    ).toBeVisible();

    // 数据自适应：有数据验证表格列头，空库验证空状态提示
    const rowCount = await page.locator("table tbody tr").count();
    if (rowCount === 0) {
      await expect(page.getByText("还没有 Provider")).toBeVisible();
    } else {
      await expect(
        page.getByRole("columnheader", { name: "Provider" }),
      ).toBeVisible();
      await expect(
        page.getByRole("columnheader", { name: "生命周期" }),
      ).toBeVisible();
    }
  });

  test("empty state shows prompt when no providers", async ({ page }) => {
    await page.goto("/ops");
    const emptyMessage = page.getByText("还没有 Provider");
    const tableRows = page.locator("table tbody tr");
    const rowCount = await tableRows.count();
    if (rowCount === 0) {
      await expect(emptyMessage).toBeVisible();
    }
  });

  test("clicking a provider row navigates to detail page", async ({
    page,
    freshProvider,
  }) => {
    await page.goto("/ops");
    await page.reload(); // ensure the fresh provider appears

    const link = page.getByRole("link", { name: freshProvider.slug });
    await expect(link).toBeVisible();
    await link.click();

    await expect(page).toHaveURL(new RegExp(`/ops/${freshProvider.id}`));
    await expect(
      page.getByRole("heading", { name: freshProvider.slug + " name" }),
    ).toBeVisible();
  });
});
