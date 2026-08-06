import { test } from "@playwright/test";

const webUrl = process.env.VLOGBIN_WEB_URL?.replace(/\/+$/, "");
const adminLogin = process.env.ZITADEL_ADMIN_LOGIN;
const adminPassword = process.env.ZITADEL_ADMIN_PASSWORD;

test.skip(
  !webUrl || !adminLogin || !adminPassword,
  "Set VLOGBIN_WEB_URL, ZITADEL_ADMIN_LOGIN and ZITADEL_ADMIN_PASSWORD",
);

test("real ZITADEL custom login reaches OIDC callback", async ({ page }) => {
  await page.goto(`${webUrl}/login`, { waitUntil: "networkidle" });
  await page.getByRole("button", { name: "开始登录" }).click();
  await page.waitForURL(
    (url) => url.pathname === "/login" && url.searchParams.has("authRequest"),
    { timeout: 30_000 },
  );

  await page.locator('input[name="identifier"]').fill(adminLogin!);
  await page.getByRole("button", { name: "继续" }).click();
  await page.locator('input[name="password"]').waitFor({ state: "visible" });
  await page.locator('input[name="password"]').fill(adminPassword!);
  await page.getByRole("button", { name: "登录" }).click();

  await page.waitForURL(
    (url) => url.pathname === "/callback" && url.searchParams.has("code"),
    { timeout: 60_000 },
  );
});
