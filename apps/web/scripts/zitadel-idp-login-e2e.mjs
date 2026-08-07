import { chromium } from "@playwright/test";

const webUrl = process.env.VLOGBIN_WEB_URL?.replace(/\/+$/, "");
const idpName = process.env.ZITADEL_E2E_IDP_NAME || "vLogBin E2E IdP";

if (!webUrl) {
  throw new Error("Set VLOGBIN_WEB_URL");
}

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();

async function finalState() {
  return {
    url: page.url(),
    body: (await page.locator("body").innerText()).slice(0, 1200),
  };
}

try {
  await page.goto(`${webUrl}/login`, { waitUntil: "networkidle" });
  await page.getByRole("button", { name: "开始登录" }).click();
  await page.waitForURL(
    (url) =>
      url.pathname === "/login" &&
      url.searchParams.has("authRequest"),
    { timeout: 30_000 },
  );

  await page
    .getByRole("button", { name: idpName, exact: true })
    .waitFor({ state: "visible", timeout: 30_000 });
  await page.getByRole("button", { name: idpName, exact: true }).click();

  // Dex mockCallback redirects back to ZITADEL without an HTML form, while
  // form-based IdPs render a submit button. Race both paths so the E2E stays
  // faithful to a real enterprise IdP without being coupled to one UX.
  await Promise.race([
    page.waitForURL(
      (url) =>
        url.pathname === "/idps/callback" ||
        url.pathname === "/idps/complete-registration",
      { timeout: 30_000 },
    ),
    (async () => {
      const submit = page
        .locator('button[type="submit"], input[type="submit"]')
        .first();
      await submit.waitFor({ state: "attached", timeout: 5_000 });
      await submit.click();
    })().catch(() => undefined),
  ]);

  if (page.url().includes("/idps/complete-registration")) {
    await page.getByLabel("名字").fill("Kilgore");
    await page.getByLabel("姓氏").fill("Trout");
    await page.getByRole("button", { name: "创建账号" }).click();
  }

  await page.waitForFunction(
    () => {
      const path = window.location.pathname;
      const search = window.location.search;
      return (
        path === "/idps/callback" ||
        (path === "/callback" && search.includes("code=")) ||
        path.startsWith("/console") ||
        path === "/error"
      );
    },
    undefined,
    { timeout: 90_000 },
  );

  if (process.env.WAIT_CONSOLE === "true") {
    await page.waitForFunction(
      () =>
        window.location.pathname.startsWith("/console") ||
        window.location.pathname === "/error",
      undefined,
      { timeout: 30_000 },
    );
  }

  console.log(JSON.stringify({ ok: true, ...(await finalState()) }, null, 2));
} catch (err) {
  console.log("FINAL_STATE", JSON.stringify(await finalState(), null, 2));
  throw err;
} finally {
  await browser.close();
}
