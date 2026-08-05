import { test, expect } from "./helpers";

/**
 * E2E-06: Console navigation — human uses the sidebar, topbar and
 * environment switcher to move around the operator console.
 */
test.describe("Console navigation", () => {
  test("sidebar links navigate between overview and providers", async ({
    page,
  }) => {
    await page.goto("/console");
    await expect(
      page.getByRole("heading", { name: "概览" }),
    ).toBeVisible();

    await page.getByRole("link", { name: "运营商台" }).click();
    await expect(page).toHaveURL("/ops");
    await expect(
      page.getByRole("heading", { name: "Providers" }),
    ).toBeVisible();

    await page.getByRole("link", { name: /概览/ }).click();
    await expect(page).toHaveURL("/console");
  });

  test("environment switcher shows test env and switches to live", async ({
    page,
  }) => {
    await page.goto("/ops");

    const switcher = page.getByRole("button", { name: /测试环境/ });
    await expect(switcher).toBeVisible();

    await switcher.click();
    await page.getByText("生产环境", { exact: true }).click();

    // 切换后 URL 携带 ?env=live（switchEnv 通过 searchParams 记忆）。
    await expect(page).toHaveURL(/env=live/);
    await expect(
      page.getByText("已切换到生产环境", { exact: true }),
    ).toBeVisible();

    // 切回测试环境。
    await page.getByRole("button", { name: /生产环境/ }).click();
    await page.getByText("测试环境", { exact: true }).click();
    await expect(page).toHaveURL(/env=test/);
    await expect(
      page.getByText("已切换到测试环境", { exact: true }),
    ).toBeVisible();
  });

  test("topbar user menu shows logout", async ({ page }) => {
    await page.goto("/ops");

    // 打开用户菜单并找到退出登录（触发器已带 aria-label="用户菜单"）。
    await page.getByRole("button", { name: "用户菜单" }).click();

    await expect(page.getByRole("menuitem", { name: /退出登录/ })).toBeVisible();
  });

  test("narrow viewport opens the navigation drawer and navigates", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/console");

    await page.getByRole("button", { name: "打开导航菜单" }).click();
    await expect(
      page.getByRole("dialog", { name: "导航菜单" }),
    ).toBeVisible();

    await page.getByRole("link", { name: "运营商台" }).click();
    await expect(page).toHaveURL("/ops");
  });
});
