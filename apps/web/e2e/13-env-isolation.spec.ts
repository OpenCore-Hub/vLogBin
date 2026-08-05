import { test, expect, api, transitionViaAPI } from "./helpers";

/**
 * E2E-13: Environment isolation end-to-end — seeds distinct test/live plans
 * and customers through the operator control plane, then verifies the
 * Console env switcher shows only the active environment's data.
 */
function planBody(code: string, name: string) {
  return {
    code,
    name,
    interval: "monthly",
    currency: "USD",
    prices: [
      {
        charge_model: "fixed",
        properties: { amount_cents: 1000, currency: "USD" },
      },
    ],
  };
}

test.describe("Environment isolation", () => {
  test("test and live data stay isolated across the Console", async ({
    page,
    freshProvider,
  }) => {
    // Create the live environment so the switcher can target it.
    await transitionViaAPI(freshProvider.id, "LIVE_REVIEW");
    await transitionViaAPI(freshProvider.id, "LIVE_ACTIVE");

    const ts = Date.now();
    const testPlanCode = `iso-test-${ts}`;
    const livePlanCode = `iso-live-${ts}`;
    const testCust = `iso-cust-${ts}`;
    const liveCust = `iso-live-cust-${ts}`;

    // Seed test environment data.
    const testPlan = await api(
      "POST",
      `/v1/operator/providers/${freshProvider.id}/catalog/plans?env=test`,
      planBody(testPlanCode, "Test Plan"),
    );
    expect(testPlan.status, `test plan: ${JSON.stringify(testPlan.body)}`).toBe(201);
    const testCustRes = await api(
      "POST",
      `/v1/operator/providers/${freshProvider.id}/customers?env=test`,
      {
        external_id: testCust,
        account_type: "business",
        display_name: "Test Customer",
      },
    );
    expect(testCustRes.status, `test customer: ${JSON.stringify(testCustRes.body)}`).toBe(201);

    await page.goto("/console/billing/plans?env=test");
    await expect(page.getByText("Test Plan", { exact: true })).toBeVisible();

    // Switch to live: the test plan must disappear.
    await page.getByRole("button", { name: /测试环境/ }).click();
    await page.getByText("生产环境", { exact: true }).click();
    await expect(page).toHaveURL(/env=live/);
    await expect(page.getByText("还没有套餐", { exact: true })).toBeVisible();

    // Seed a live plan, then verify only the live plan is visible.
    const livePlan = await api(
      "POST",
      `/v1/operator/providers/${freshProvider.id}/catalog/plans?env=live`,
      planBody(livePlanCode, "Live Starter"),
    );
    expect(livePlan.status, `live plan: ${JSON.stringify(livePlan.body)}`).toBe(201);
    await page.goto("/console/billing/plans?env=live");
    await expect(page.getByText("Live Starter", { exact: true })).toBeVisible();
    await expect(page.getByText("Test Plan", { exact: true })).toHaveCount(0);

    // Customers: live is empty until seeded, then isolated both ways.
    await page.goto("/console/billing/customers?env=live");
    await expect(page.getByText("还没有客户", { exact: true })).toBeVisible();

    const liveCustRes = await api(
      "POST",
      `/v1/operator/providers/${freshProvider.id}/customers?env=live`,
      {
        external_id: liveCust,
        account_type: "individual",
        display_name: "Live Customer",
      },
    );
    expect(liveCustRes.status, `live customer: ${JSON.stringify(liveCustRes.body)}`).toBe(201);

    await page.goto("/console/billing/customers?env=live");
    await expect(page.getByText("Live Customer", { exact: true })).toBeVisible();
    await expect(page.getByText("Test Customer", { exact: true })).toHaveCount(0);

    await page.getByRole("button", { name: /生产环境/ }).click();
    await page.getByText("测试环境", { exact: true }).click();
    await expect(page).toHaveURL(/env=test/);
    await expect(page.getByText("Test Customer", { exact: true })).toBeVisible();
    await expect(page.getByText("Live Customer", { exact: true })).toHaveCount(0);
  });
});
