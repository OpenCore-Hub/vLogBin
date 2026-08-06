import { test, expect } from "./helpers";

/**
 * E2E-25: Identity Users control plane — invite, edit role and remove a
 * workspace member.
 */
test.describe("Identity Users", () => {
  test("invite, update role and remove a workspace member", async ({
    page,
  }) => {
    await page.goto("/console/identity/users");

    await expect(page.getByRole("heading", { name: "Users" })).toBeVisible();

    const memberSub = `dev-${Date.now()}`;
    await page.getByRole("button", { name: "邀请成员" }).click();
    await page.getByLabel("用户 subject").fill(memberSub);
    await page.getByLabel("角色", { exact: true }).selectOption("provider_developer");
    await page.getByRole("dialog").getByRole("button", { name: "邀请成员", exact: true }).click();
    await expect(page.getByText(memberSub, { exact: true })).toBeVisible();
    await expect(page.getByText("provider_developer", { exact: true }).last()).toBeVisible();

    await page.getByRole("button", { name: `编辑 ${memberSub}` }).click();
    await page.getByLabel("角色", { exact: true }).selectOption("provider_billing");
    await page.getByRole("dialog").getByRole("button", { name: "保存修改" }).click();
    await expect(page.getByText("provider_billing", { exact: true }).last()).toBeVisible();

    await page.getByRole("button", { name: `移除 ${memberSub}` }).click();
    await page.getByLabel(`输入 ${memberSub} 确认`).fill(memberSub);
    await page.getByRole("dialog").getByRole("button", { name: "移除成员" }).click();
    await expect(page.getByText(memberSub, { exact: true })).toHaveCount(0);
  });
});
