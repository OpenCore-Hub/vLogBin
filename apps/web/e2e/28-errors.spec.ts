import { test, expect } from "./helpers";

/**
 * E2E-28: Global error surfaces — unknown routes render the branded 404.
 */
test.describe("Global error surfaces", () => {
  test("unknown console route renders branded not-found", async ({ page }) => {
    await page.goto("/console/billing/not-a-real-page");

    await expect(page.getByText("404 · Not Found")).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "页面不存在" }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "返回控制台" }),
    ).toBeVisible();
  });
});
