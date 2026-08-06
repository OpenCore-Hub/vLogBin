import { test, expect, api, createProviderViaAPI } from "./helpers";

/**
 * E2E-18: 客户门户 Portal (§8 M3) — customer token login, isolated dashboard,
 * invoices / usage / payment tabs.
 */
test.describe("Customer Portal", () => {
  test("customer logs in with portal token and sees only their data", async ({ page }) => {
    const provider = await createProviderViaAPI(`portal-${Date.now()}`);
    const externalId = `portal-cust-${Date.now()}`;

    const cust = await api(
      "POST",
      `/v1/operator/providers/${provider.id}/customers?env=test`,
      {
        external_id: externalId,
        account_type: "business",
        display_name: "Portal Customer",
      },
    );
    expect(cust.status, `create customer: ${JSON.stringify(cust.body)}`).toBe(201);

    const tokenRes = await api(
      "POST",
      `/v1/operator/providers/${provider.id}/customers/${externalId}/portal-token?env=test`,
      undefined,
    );
    expect(tokenRes.status, `issue portal token: ${JSON.stringify(tokenRes.body)}`).toBe(201);
    const token = tokenRes.body.token as string;

    await page.goto("/portal/login");
    await page.getByLabel("门户邀请 Token").fill(token);
    await page.getByRole("button", { name: "进入门户" }).click();

    await expect(page.getByRole("heading", { name: "Portal Customer" })).toBeVisible();
    await expect(page.getByText(externalId, { exact: true })).toBeVisible();

    await page.getByRole("tab", { name: "用量" }).click();
    await expect(page.getByText("暂无用量事件", { exact: true })).toBeVisible();

    await page.getByRole("tab", { name: "支付" }).click();
    await expect(page.getByText("暂无支付记录", { exact: true })).toBeVisible();
    await expect(page.getByText("尚未接入支付渠道", { exact: false })).toBeVisible();

    await page.getByRole("button", { name: "退出" }).click();
    await expect(page.getByRole("heading", { name: "客户门户" })).toBeVisible();
  });
});
