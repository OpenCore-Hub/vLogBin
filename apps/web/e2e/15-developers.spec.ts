import { test, expect } from "./helpers";

/**
 * E2E-15: Developers control plane (§8 M3) — API Keys create/rotate/revoke,
 * Webhook endpoint create/delete with one-time signing secret, and the
 * event stream viewer.
 */
test.describe("Developers control plane", () => {
  test("API Keys: create, rotate, and revoke from the console", async ({ page }) => {
    const keyName = `ci-key-${Date.now()}`;
    await page.goto("/console/developers/api-keys?env=test");
    await expect(page.getByRole("heading", { name: "API Keys" })).toBeVisible();

    await page.getByRole("button", { name: "创建密钥" }).click();
    await page.getByLabel("密钥名称").fill(keyName);
    await page.locator('input[name="scopes"][value="write"]').check();
    await page.locator('input[name="scopes"][value="audit:read"]').check();
    await page.getByRole("dialog").getByRole("button", { name: "创建密钥", exact: true }).click();

    await expect(page.getByText("密钥创建成功", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "复制密钥" })).toBeVisible();
    await page.getByRole("button", { name: "完成", exact: true }).click();

    const row = page.locator("tr", { hasText: keyName });
    await expect(row).toContainText("pk_test_");

    // Rotate through the row menu.
    await row.getByRole("button", { name: `${keyName} 操作` }).click();
    await page.getByText("轮换密钥", { exact: true }).click();
    await page.getByRole("dialog").getByRole("button", { name: "确认轮换" }).click();
    await expect(page.getByText("密钥已轮换", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "复制密钥" })).toBeVisible();
    await page.getByRole("button", { name: "完成", exact: true }).click();

    // Revoke with type-to-confirm.
    const rotatedRow = page.locator("tr", { hasText: keyName });
    await rotatedRow.getByRole("button", { name: `${keyName} 操作` }).click();
    await page.getByText("吊销密钥", { exact: true }).click();
    await page.getByLabel(`输入 ${keyName} 确认`).fill(keyName);
    await page.getByRole("dialog").getByRole("button", { name: "吊销密钥", exact: true }).click();
    await expect(page.locator("tbody tr", { hasText: keyName })).toHaveCount(2);
    await expect(page.getByText("已吊销", { exact: true }).first()).toBeVisible();
  });

  test("Webhooks: create endpoint, show secret once, then delete", async ({ page }) => {
    const webhookUrl = `https://8.8.8.8/hook-${Date.now()}`;
    await page.goto("/console/developers/webhooks?env=test");
    await expect(page.getByRole("heading", { name: "Webhooks" })).toBeVisible();

    await page.getByRole("button", { name: "创建端点" }).click();
    await page.getByLabel("回调 URL").fill(webhookUrl);
    await page.getByLabel("customer.created", { exact: true }).check();
    await page.getByRole("dialog").getByRole("button", { name: "创建端点", exact: true }).click();

    await expect(page.getByText("Webhook 端点创建成功", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "复制签名密钥" })).toBeVisible();
    await page.getByRole("button", { name: "完成", exact: true }).click();

    const row = page.locator("tr", { hasText: webhookUrl });
    await expect(row).toContainText("customer.created");
    await expect(row).not.toContainText("secret");

    await row.getByRole("button", { name: /删除/ }).click();
    await page.getByLabel(`输入 ${webhookUrl} 确认`).fill(webhookUrl);
    await page.getByRole("dialog").getByRole("button", { name: "删除端点", exact: true }).click();
    await expect(page.locator("tr", { hasText: webhookUrl })).toHaveCount(0);
  });

  test("Events: stream renders and payload detail opens", async ({ page }) => {
    await page.goto("/console/developers/events?env=test");
    await expect(page.getByRole("heading", { name: "事件流" })).toBeVisible();

    await expect(page.locator("tbody tr").first()).toBeVisible();

    await page.locator("tbody tr").first().getByRole("button", { name: /查看事件/ }).click();
    await expect(page.getByRole("dialog")).toContainText("payload");
    await expect(page.getByRole("dialog").getByText("json", { exact: true })).toBeVisible();
  });
});
