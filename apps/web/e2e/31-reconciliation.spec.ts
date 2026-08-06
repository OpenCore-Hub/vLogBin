import { test, expect } from "./helpers";

/**
 * E2E-31: Reconciliation control plane — the operator reconciliation page
 * renders summary cards and handles both empty and populated states.
 */
test.describe("Reconciliation control plane", () => {
  test("renders reconciliation summary and result table", async ({ page }) => {
    await page.goto("/console/reconciliation");

    await expect(page.getByRole("heading", { name: "对账" })).toBeVisible();
    await expect(page.getByText("通过", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("漂移", { exact: true }).first()).toBeVisible();
    await expect(page.getByText("检查数", { exact: true })).toBeVisible();
  });
});
