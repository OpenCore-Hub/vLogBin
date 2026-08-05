import { test as loggedInTest, expect } from "./helpers";
import { test as rawTest } from "@playwright/test";

/**
 * E2E-07: Signup flow.
 * - 未登录：/signup 展示注册页（operator-token 模式下降级提示自助注册不可用）
 * - 官网未登录 CTA「免费开始 / 创建工作空间」指向 /signup
 * - 登录页提供「立即注册」入口
 * - 已登录访问 /signup → 重定向 /console
 */
rawTest.describe("Signup (unauthenticated)", () => {
  rawTest("renders signup page with local-mode notice", async ({ page }) => {
    await page.goto("/signup");
    await expect(
      page.getByRole("heading", { name: "创建 vLogBin 工作空间" }),
    ).toBeVisible();
    // operator-token 模式：自助注册不可用，展示说明并引导登录
    await expect(
      page.getByText("令牌登录模式不支持自助注册"),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "前往登录" }),
    ).toBeVisible();
    await expect(page.getByRole("link", { name: "立即登录" })).toBeVisible();
  });

  rawTest("marketing CTAs point to /signup when logged out", async ({
    page,
  }) => {
    await page.goto("/");
    await expect(page.getByRole("link", { name: "免费开始" }).first()).toHaveAttribute(
      "href",
      "/signup",
    );
    await expect(
      page.getByRole("link", { name: "创建工作空间" }),
    ).toHaveAttribute("href", "/signup");
  });

  rawTest("login page links to signup", async ({ page }) => {
    await page.goto("/login");
    await expect(page.getByRole("link", { name: "立即注册" })).toHaveAttribute(
      "href",
      "/signup?next=%2Fconsole",
    );
  });
});

loggedInTest.describe("Signup (authenticated)", () => {
  loggedInTest("redirects /signup to console when logged in", async ({
    page,
  }) => {
    await page.goto("/signup");
    await expect(page).toHaveURL(/\/console/, { timeout: 10_000 });
  });
});
