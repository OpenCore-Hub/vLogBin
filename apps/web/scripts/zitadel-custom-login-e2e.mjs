import { chromium } from "@playwright/test";

const webUrl = process.env.VLOGBIN_WEB_URL?.replace(/\/+$/, "");
const adminLogin = process.env.ZITADEL_ADMIN_LOGIN;
const adminPassword = process.env.ZITADEL_ADMIN_PASSWORD;

if (!webUrl || !adminLogin || !adminPassword) {
  throw new Error(
    "Set VLOGBIN_WEB_URL, ZITADEL_ADMIN_LOGIN, ZITADEL_ADMIN_PASSWORD",
  );
}

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();

await page.goto(`${webUrl}/login`, { waitUntil: "networkidle" });
await page.getByRole("button", { name: "开始登录" }).click();
await page.waitForURL((url) => url.pathname === "/login" && url.searchParams.has("authRequest"), {
  timeout: 30_000,
});

await page.locator('input[name="identifier"]').fill(adminLogin);
await page.getByRole("button", { name: "继续" }).click();
  await page.locator('input[name="password"]').waitFor({ state: "visible" });
  await page.locator('input[name="password"]').fill(adminPassword);
  await page.getByRole("button", { name: "登录" }).click();

try {
  await page.waitForURL(
    (url) => url.pathname === "/callback" && url.searchParams.has("code"),
    { timeout: 60_000 },
  );
} catch (err) {
  console.log("FAILED_URL", page.url());
  console.log(
    "FAILED_BODY",
    (await page.locator("body").innerText()).slice(0, 3000),
  );
  throw err;
}

console.log(
  JSON.stringify(
    {
      ok: true,
      callbackUrl: page.url(),
      finalUrl: process.env.WAIT_CONSOLE === "true" ? page.url() : undefined,
    },
    null,
    2,
  ),
);

if (process.env.WAIT_CONSOLE === "true") {
  await page.waitForURL(
    (url) => url.pathname.startsWith("/console") || url.pathname === "/error",
    { timeout: 30_000 },
  );
  console.log("FINAL_URL", page.url());
  console.log(
    "FINAL_BODY",
    (await page.locator("body").innerText()).slice(0, 1000),
  );
}

await browser.close();
