import { readFile } from "node:fs/promises";
import { createConnectTransport } from "@connectrpc/connect-node";
import { NewAuthorizationBearerInterceptor } from "@zitadel/client";
import { newSystemToken } from "@zitadel/client/node";
import {
  createSessionServiceClient,
  createSettingsServiceClient,
  createUserServiceClient,
} from "@zitadel/client/v2";

function required(name) {
  const value = process.env[name]?.trim();
  if (!value) {
    throw new Error(`Missing required environment variable: ${name}`);
  }
  return value;
}

function assertVersionGate() {
  const raw = process.env.ZITADEL_VERSION;
  if (!raw) return undefined;
  const version = raw.replace(/^v/, "");
  const [major, minor] = version.split(".").map((part) => Number.parseInt(part, 10));
  if (
    !Number.isInteger(major) ||
    !Number.isInteger(minor) ||
    major < 4 ||
    (major === 4 && minor < 6)
  ) {
    throw new Error(
      `ZITADEL_VERSION ${raw} does not meet production baseline >=4.6.0`,
    );
  }
  return version;
}

async function resolveToken() {
  const apiUrl = process.env.ZITADEL_API_URL?.trim() || required("ZITADEL_URL");
  if (process.env.ZITADEL_LOGINCLIENT_KEYFILE) {
    const key = await readFile(process.env.ZITADEL_LOGINCLIENT_KEYFILE, "utf8");
    return newSystemToken({
      audience: process.env.AUDIENCE || apiUrl,
      subject: "login-client",
      key,
    });
  }
  if (
    process.env.SYSTEM_USER_ID &&
    (process.env.SYSTEM_USER_PRIVATE_KEY || process.env.SYSTEM_USER_PRIVATE_KEY_FILE)
  ) {
    const key = process.env.SYSTEM_USER_PRIVATE_KEY_FILE
      ? await readFile(process.env.SYSTEM_USER_PRIVATE_KEY_FILE, "utf8")
      : Buffer.from(process.env.SYSTEM_USER_PRIVATE_KEY, "base64").toString("utf8");
    return newSystemToken({
      audience: process.env.AUDIENCE || apiUrl,
      subject: process.env.SYSTEM_USER_ID,
      key,
    });
  }
  if (process.env.ZITADEL_SERVICE_USER_TOKEN) {
    return process.env.ZITADEL_SERVICE_USER_TOKEN;
  }
  if (process.env.ZITADEL_LOGIN_CLIENT_PAT) {
    return process.env.ZITADEL_LOGIN_CLIENT_PAT;
  }
  throw new Error(
    "Missing ZITADEL service credential: set ZITADEL_LOGINCLIENT_KEYFILE, system user JWT, ZITADEL_SERVICE_USER_TOKEN or ZITADEL_LOGIN_CLIENT_PAT",
  );
}

const apiUrl = (process.env.ZITADEL_API_URL?.trim() || required("ZITADEL_URL")).replace(/\/+$/, "");
const version = assertVersionGate();
const token = await resolveToken();
const transport = createConnectTransport({
  httpVersion: "1.1",
  baseUrl: apiUrl,
  interceptors: [NewAuthorizationBearerInterceptor(token)],
});

const settingsService = createSettingsServiceClient(transport);
const userService = createUserServiceClient(transport);
const sessionService = createSessionServiceClient(transport);

const loginSettings = await settingsService.getLoginSettings({});
if (!loginSettings.settings) {
  throw new Error("Contract probe failed: getLoginSettings returned no settings");
}

const users = await userService.listUsers({
  query: { offset: BigInt(0), limit: 1, asc: true },
  sortingColumn: 1,
  queries: [],
});
if (!Array.isArray(users.result)) {
  throw new Error("Contract probe failed: listUsers returned no result array");
}

const sessions = await sessionService.listSessions({ queries: [] });
if (!Array.isArray(sessions.sessions)) {
  throw new Error("Contract probe failed: listSessions returned no sessions array");
}

console.log(
  JSON.stringify(
    {
      ok: true,
      apiUrl,
      version,
      loginSettings: Boolean(loginSettings.settings),
      userListSupported: true,
      sessionListSupported: true,
      userCount: users.result.length,
      sessionCount: sessions.sessions.length,
    },
    null,
    2,
  ),
);
