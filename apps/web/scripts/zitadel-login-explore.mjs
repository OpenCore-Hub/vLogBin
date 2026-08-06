import { createHash, randomBytes } from "node:crypto";
import { chromium } from "@playwright/test";

const zitadelUrl = process.env.ZITADEL_URL?.replace(/\/+$/, "");
const clientId = process.env.ZITADEL_CONSOLE_CLIENT_ID;
const redirectUri = process.env.ZITADEL_CONSOLE_REDIRECT_URI;
const loginName = process.env.ZITADEL_ADMIN_LOGIN;
const password = process.env.ZITADEL_ADMIN_PASSWORD;

if (!zitadelUrl || !clientId || !redirectUri || !loginName || !password) {
  throw new Error(
    "Set ZITADEL_URL, ZITADEL_CONSOLE_CLIENT_ID, ZITADEL_CONSOLE_REDIRECT_URI, ZITADEL_ADMIN_LOGIN, ZITADEL_ADMIN_PASSWORD",
  );
}

const verifier = Buffer.from(randomBytes(48)).toString("base64url");
const challenge = createHash("sha256").update(verifier).digest("base64url");
const state = Buffer.from(randomBytes(24)).toString("base64url");
const nonce = Buffer.from(randomBytes(16)).toString("base64url");

const authorize = new URL(`${zitadelUrl}/oauth/v2/authorize`);
authorize.searchParams.set("client_id", clientId);
authorize.searchParams.set("redirect_uri", redirectUri);
authorize.searchParams.set("response_type", "code");
authorize.searchParams.set("scope", "openid profile email offline_access");
authorize.searchParams.set("state", state);
authorize.searchParams.set("code_challenge", challenge);
authorize.searchParams.set("code_challenge_method", "S256");
authorize.searchParams.set("nonce", nonce);

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();

page.on("response", (response) => {
  if (response.url().includes("/ui/v2/login/")) {
    console.log("LOGIN_RESPONSE", response.status(), response.url());
  }
});

await page.goto(authorize.toString(), { waitUntil: "networkidle" });
console.log("URL_AFTER_AUTHORIZE", page.url());
console.log("TITLE", await page.title());

const inputs = await page.locator("input").evaluateAll((nodes) =>
  nodes.map((node) => ({
    name: node.getAttribute("name"),
    id: node.id,
    type: node.getAttribute("type"),
    placeholder: node.getAttribute("placeholder"),
  })),
);
console.log("INPUTS", JSON.stringify(inputs, null, 2));
console.log(
  "TEXT",
  (await page.locator("body").innerText()).slice(0, 2000),
);

await page.locator('input[name="loginName"]').fill(loginName);
await page.getByRole("button", { name: "Continue" }).click();
await page.locator("input[type='password']").waitFor({ state: "visible" });
console.log("URL_AFTER_LOGINNAME", page.url());
console.log(
  "PASSWORD_INPUTS",
  JSON.stringify(
    await page.locator("input").evaluateAll((nodes) =>
      nodes.map((node) => ({
        name: node.getAttribute("name"),
        id: node.id,
        type: node.getAttribute("type"),
        placeholder: node.getAttribute("placeholder"),
      })),
    ),
    null,
    2,
  ),
);
console.log(
  "PASSWORD_TEXT",
  (await page.locator("body").innerText()).slice(0, 2000),
);

await browser.close();
