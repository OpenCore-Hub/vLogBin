import { test, expect } from "./helpers";

/**
 * E2E-10: Invoices page — verifies the Console bills list renders its empty
 * state and the detail page handles unknown invoice ids without crashing.
 * Invoice rows are only created by billing-engine sync, so data-backed flows
 * are covered by the Go integration tests.
 */
test.describe("Invoices page", () => {
  test("renders empty state and handles unknown invoice detail", async ({
    page,
  }) => {
    await page.goto("/console/billing/invoices");

    await expect(
      page.getByRole("heading", { name: "账单", exact: true }),
    ).toBeVisible();
    await expect(page.getByText("暂无账单", { exact: true })).toBeVisible();

    await page.goto(
      "/console/billing/invoices/00000000-0000-0000-0000-000000000000",
    );
    await expect(
      page.getByText("账单详情加载失败", { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "返回账单列表" }),
    ).toBeVisible();
  });
});
