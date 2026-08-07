import { createHash, randomBytes } from "node:crypto";
import { chromium } from "@playwright/test";
import { create } from "@bufbuild/protobuf";
import { createConnectTransport } from "@connectrpc/connect-node";
import { NewAuthorizationBearerInterceptor } from "@zitadel/client";
import { createFeatureServiceClient } from "@zitadel/client/v2";
import { SetInstanceFeaturesRequestSchema } from "@zitadel/proto/zitadel/feature/v2/instance_pb";
import { LoginV2Schema } from "@zitadel/proto/zitadel/feature/v2/feature_pb";

const zitadelUrl = process.env.ZITADEL_URL?.replace(/\/+$/, "");
const consoleClientId = process.env.ZITADEL_CONSOLE_CLIENT_ID;
const consoleRedirectUri = process.env.ZITADEL_CONSOLE_REDIRECT_URI;
const adminLogin = process.env.ZITADEL_ADMIN_LOGIN;
const adminPassword = process.env.ZITADEL_ADMIN_PASSWORD;
const appName =
  process.env.ZITADEL_E2E_APP_NAME || `vlogbin-e2e-${Date.now()}`;
const appRedirectUri = process.env.ZITADEL_E2E_REDIRECT_URI;
const idpName =
  process.env.ZITADEL_E2E_IDP_NAME || "vLogBin E2E IdP";
const idpIssuer =
  process.env.ZITADEL_E2E_IDP_ISSUER || "http://idp.localhost:18082/dex";
const idpClientId =
  process.env.ZITADEL_E2E_IDP_CLIENT_ID || "vlogbin-idp";
const idpClientSecret =
  process.env.ZITADEL_E2E_IDP_CLIENT_SECRET || "e2e-idp-client-secret";

if (
  !zitadelUrl ||
  !consoleClientId ||
  !consoleRedirectUri ||
  !adminLogin ||
  !adminPassword ||
  !appRedirectUri
) {
  throw new Error(
    "Set ZITADEL_URL, ZITADEL_CONSOLE_CLIENT_ID, ZITADEL_CONSOLE_REDIRECT_URI, ZITADEL_ADMIN_LOGIN, ZITADEL_ADMIN_PASSWORD, ZITADEL_E2E_REDIRECT_URI",
  );
}

function base64Url(value) {
  return Buffer.from(value).toString("base64url");
}

const verifier = base64Url(randomBytes(48));
const challenge = createHash("sha256").update(verifier).digest("base64url");
const state = base64Url(randomBytes(24));
const nonce = base64Url(randomBytes(16));

const authorize = new URL(`${zitadelUrl}/oauth/v2/authorize`);
authorize.searchParams.set("client_id", consoleClientId);
authorize.searchParams.set("redirect_uri", consoleRedirectUri);
authorize.searchParams.set("response_type", "code");
authorize.searchParams.set("scope", "openid profile email offline_access");
authorize.searchParams.set("state", state);
authorize.searchParams.set("code_challenge", challenge);
authorize.searchParams.set("code_challenge_method", "S256");
authorize.searchParams.set("nonce", nonce);

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.goto(authorize.toString(), { waitUntil: "networkidle" });

await page.locator('input[name="loginName"]').fill(adminLogin);
await page.getByRole("button", { name: "Continue" }).click();
await page.locator('input[name="password"]').waitFor({ state: "visible" });
await page.locator('input[name="password"]').fill(adminPassword);
await page.getByRole("button", { name: "Continue" }).click();

await page.waitForURL((url) => url.searchParams.has("code"), {
  timeout: 60_000,
});
const callbackUrl = new URL(page.url());
const code = callbackUrl.searchParams.get("code");
const returnedState = callbackUrl.searchParams.get("state");
await browser.close();

if (!code || returnedState !== state) {
  throw new Error("OIDC login did not return a valid code/state");
}

const discovery = await fetch(`${zitadelUrl}/.well-known/openid-configuration`).then(
  (response) => response.json(),
);
const tokenResponse = await fetch(discovery.token_endpoint, {
  method: "POST",
  headers: { "Content-Type": "application/x-www-form-urlencoded" },
  body: new URLSearchParams({
    grant_type: "authorization_code",
    code,
    redirect_uri: consoleRedirectUri,
    client_id: consoleClientId,
    code_verifier: verifier,
  }),
});
const tokenJson = await tokenResponse.json();
if (!tokenResponse.ok || !tokenJson.access_token) {
  throw new Error(
    `Token exchange failed: ${tokenResponse.status} ${JSON.stringify(tokenJson)}`,
  );
}

const accessToken = tokenJson.access_token;
const loginBaseUri =
  process.env.ZITADEL_E2E_LOGIN_BASE_URI || "http://localhost:3100/";
const featureTransport = createConnectTransport({
  httpVersion: "1.1",
  baseUrl: zitadelUrl,
  interceptors: [NewAuthorizationBearerInterceptor(accessToken)],
});
const featureService = createFeatureServiceClient(featureTransport);
await featureService.setInstanceFeatures(
  create(SetInstanceFeaturesRequestSchema, {
    loginV2: create(LoginV2Schema, {
      required: true,
      baseUri: loginBaseUri,
    }),
  }),
);

const authHeaders = {
  Authorization: `Bearer ${accessToken}`,
  "Content-Type": "application/json",
};

const projectResponse = await fetch(`${zitadelUrl}/management/v1/projects`, {
  method: "POST",
  headers: authHeaders,
  body: JSON.stringify({
    name: appName,
    projectRoleAssertion: false,
    hasProjectCheck: false,
  }),
});
const projectJson = await projectResponse.json();
if (!projectResponse.ok || !projectJson.id) {
  throw new Error(
    `Create project failed: ${projectResponse.status} ${JSON.stringify(projectJson)}`,
  );
}

const appResponse = await fetch(
  `${zitadelUrl}/management/v1/projects/${projectJson.id}/apps/oidc`,
  {
    method: "POST",
    headers: authHeaders,
    body: JSON.stringify({
      name: appName,
      redirectUris: [appRedirectUri],
      responseTypes: ["OIDC_RESPONSE_TYPE_CODE"],
      grantTypes: ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE", "OIDC_GRANT_TYPE_REFRESH_TOKEN"],
      appType: "OIDC_APP_TYPE_WEB",
      authMethodType: "OIDC_AUTH_METHOD_TYPE_BASIC",
      accessTokenType: "OIDC_TOKEN_TYPE_JWT",
      idTokenRoleAssertion: true,
      accessTokenRoleAssertion: true,
      loginVersion: {
        loginV2: {
          baseUri: process.env.ZITADEL_E2E_LOGIN_BASE_URI || "http://localhost:3100/",
        },
      },
    }),
  },
);
const appJson = await appResponse.json();
if (!appResponse.ok || !appJson.clientId) {
  throw new Error(
    `Create OIDC app failed: ${appResponse.status} ${JSON.stringify(appJson)}`,
  );
}

const idpResponse = await fetch(`${zitadelUrl}/admin/v1/idps/generic_oidc`, {
  method: "POST",
  headers: authHeaders,
  body: JSON.stringify({
    name: idpName,
    issuer: idpIssuer,
    clientId: idpClientId,
    clientSecret: idpClientSecret,
    scopes: ["openid", "profile", "email"],
    providerOptions: {
      isLinkingAllowed: false,
      isCreationAllowed: true,
      isAutoCreation: true,
      isAutoUpdate: true,
      autoLinking: "AUTO_LINKING_OPTION_UNSPECIFIED",
    },
    isIdTokenMapping: true,
    usePkce: false,
  }),
});
const idpJson = await idpResponse.json();
if (!idpResponse.ok || !idpJson.id) {
  throw new Error(
    `Create generic OIDC IdP failed: ${idpResponse.status} ${JSON.stringify(idpJson)}`,
  );
}

const policyResponse = await fetch(`${zitadelUrl}/admin/v1/policies/login`, {
  method: "PUT",
  headers: authHeaders,
  body: JSON.stringify({
    allowUsernamePassword: true,
    allowRegister: true,
    allowExternalIdp: true,
    forceMfa: false,
    passwordlessType: "PASSWORDLESS_TYPE_ALLOWED",
    hidePasswordReset: false,
    ignoreUnknownUsernames: false,
    defaultRedirectUri: appRedirectUri,
    allowDomainDiscovery: false,
    disableLoginWithEmail: false,
    disableLoginWithPhone: false,
    forceMfaLocalOnly: false,
  }),
});
const policyJson = await policyResponse.json();
if (!policyResponse.ok) {
  throw new Error(
    `Update login policy failed: ${policyResponse.status} ${JSON.stringify(policyJson)}`,
  );
}

const policyIdpResponse = await fetch(
  `${zitadelUrl}/admin/v1/policies/login/idps`,
  {
    method: "POST",
    headers: authHeaders,
    body: JSON.stringify({ idpId: idpJson.id }),
  },
);
const policyIdpJson = await policyIdpResponse.json();
if (!policyIdpResponse.ok) {
  throw new Error(
    `Add IdP to login policy failed: ${policyIdpResponse.status} ${JSON.stringify(policyIdpJson)}`,
  );
}

console.log(
  JSON.stringify(
    {
      ok: true,
      projectId: projectJson.id,
      appId: appJson.appId,
      clientId: appJson.clientId,
      clientSecret: appJson.clientSecret,
      idpId: idpJson.id,
      idpName,
    },
    null,
    2,
  ),
);
