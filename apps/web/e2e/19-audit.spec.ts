import { test, expect } from "./helpers";

/**
 * E2E-19: 审计日志前端（M3）— filters, paginated table and tamper-evident
 * chain verification.
 */
test.describe("Audit log", () => {
  test("renders audit table and verifies the hash chain", async ({ page }) => {
    await page.goto("/console/audit");

    await expect(page.getByRole("heading", { name: "审计日志" })).toBeVisible();
    await expect(page.getByText("审计哈希链", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "动作" })).toBeVisible();

    const rows = page.locator("tbody tr");
    if ((await rows.count()) > 0) {
      await expect(rows.first()).toBeVisible();
    }

    await page.getByRole("button", { name: "验证完整性" }).click();
    await expect(page.getByText("链完整", { exact: true })).toBeVisible();
  });
});
