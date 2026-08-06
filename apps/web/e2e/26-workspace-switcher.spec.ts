import { test, expect } from "./helpers";

/**
 * E2E-26: Workspace switcher (M4) — the current workspace is visible in the
 * topbar and Console pages still resolve the active provider.
 */
test.describe("Workspace switcher", () => {
  test("shows active workspace and keeps pages healthy", async ({ page }) => {
    await page.goto("/console");
    await expect(page.getByLabel(/当前工作区/)).toBeVisible();

    await page.goto("/console/billing/plans");
    await expect(
      page.getByRole("heading", { name: "套餐", exact: true }),
    ).toBeVisible();
  });
});
