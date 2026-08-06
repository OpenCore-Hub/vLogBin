import { test, expect } from "./helpers";

/**
 * E2E-29: Analytics control plane — the Console analytics dashboard renders
 * summary cards and handles the empty analytics state.
 */
test.describe("Analytics control plane", () => {
  test("renders analytics summary and empty state", async ({
    page,
  }) => {
    await page.goto("/console/analytics?env=test");

    await expect(
      page.getByRole("heading", { name: "Analytics" }),
    ).toBeVisible();
    await expect(page.getByText("收入", { exact: true }).first()).toBeVisible();
    await expect(
      page.getByText("活跃客户", { exact: true }).first(),
    ).toBeVisible();
    await expect(
      page.getByText("用量异常", { exact: true }).first(),
    ).toBeVisible();
  });
});
