import { test, expect } from "./helpers";

/**
 * E2E-22: Payments control plane — renders payment summary cards and the
 * invoice payment-status surface from the Console.
 */
test.describe("Payments control plane", () => {
  test("renders payment summary and empty invoice state", async ({
    page,
    freshProvider,
  }) => {
    await page.goto("/console/billing/payments?env=test");

    await expect(page.getByRole("heading", { name: "支付" })).toBeVisible();
    await expect(page.getByText("支付成功", { exact: true })).toBeVisible();
    await expect(page.getByText("待支付", { exact: true })).toBeVisible();
    await expect(page.getByText("支付失败", { exact: true })).toBeVisible();
    await expect(page.getByText("暂无账单", { exact: true })).toBeVisible();
  });
});
