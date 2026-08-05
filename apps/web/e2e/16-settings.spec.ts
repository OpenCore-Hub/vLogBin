import { test, expect, api } from "./helpers";

/**
 * E2E-16: Console Settings (§8 M3) — basic workspace info, custom domains,
 * and notification configs, grouped by 基础 / 安全 / 高级.
 */

async function ensureWorkspaceProvider(slug: string): Promise<string> {
  const r = await api("POST", "/v1/signup", {
    email: `${slug}@example.com`,
    name: `${slug} name`,
  });
  expect(r.status, `signup: ${JSON.stringify(r.body)}`).toBe(200);
  const providerId = r.body.provider.id as string;
  const lifecycle = r.body.provider.lifecycle_state as string;
  if (lifecycle === "REGISTERED") {
    const act = await api(
      "POST",
      `/v1/operator/providers/${providerId}/activate`,
      { home_region_code: "cn-shanghai" },
    );
    expect(
      [200, 409].includes(act.status),
      `activate: ${act.status} ${JSON.stringify(act.body)}`,
    ).toBe(true);
  }
  return providerId;
}

test.describe("Console Settings", () => {
  test("basic: workspace name and slug can be saved", async ({ page }) => {
    await ensureWorkspaceProvider(`settings-${Date.now()}`);
    const name = `Workspace ${Date.now()}`;
    const slug = `workspace-${Date.now()}`;

    await page.goto("/console/settings?env=test");
    await expect(page.getByRole("heading", { name: "设置" })).toBeVisible();
    await page.getByLabel("名称").fill(name);
    await page.getByLabel("Slug").fill(slug);
    await page.getByRole("button", { name: "保存基础信息" }).click();
    await expect(page.getByText("已保存", { exact: true })).toBeVisible();
  });

  test("security: register and delete a custom domain", async ({ page }) => {
    await ensureWorkspaceProvider(`settings-${Date.now()}`);
    const domain = `auth-${Date.now()}.example.com`;

    await page.goto("/console/settings?env=test");
    await page.getByRole("tab", { name: "安全" }).click();
    await page.getByRole("button", { name: "注册域名", exact: true }).click();
    await page.getByRole("dialog").getByLabel("域名").fill(domain);
    await page.getByRole("dialog").getByRole("button", { name: "注册域名", exact: true }).click();

    await expect(page.getByText("域名已注册", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "复制 Token" })).toBeVisible();
    await page.getByRole("button", { name: "完成", exact: true }).click();

    const row = page.locator("tr", { hasText: domain });
    await expect(row).toContainText("pending");
    await row.getByRole("button", { name: `删除 ${domain}` }).click();
    await page.getByLabel(`输入 ${domain} 确认`).fill(domain);
    await page.getByRole("dialog").getByRole("button", { name: "删除域名", exact: true }).click();
    await expect(page.locator("tr", { hasText: domain })).toHaveCount(0);
  });

  test("advanced: email notification config can be saved", async ({ page }) => {
    await ensureWorkspaceProvider(`settings-${Date.now()}`);

    await page.goto("/console/settings?env=test");
    await page.getByRole("tab", { name: "高级" }).click();
    await page.locator("#email-provider").fill("smtp");
    await page.locator("#email-from").fill(`noreply-${Date.now()}@example.com`);
    await page.getByRole("button", { name: "保存邮件通知" }).click();
    await expect(page.getByText("已保存", { exact: true })).toBeVisible();
  });
});
