import "server-only";

import { readFile } from "node:fs/promises";
import { createConnectTransport } from "@connectrpc/connect-node";
import { NewAuthorizationBearerInterceptor } from "@zitadel/client";
import { newSystemToken } from "@zitadel/client/node";
import {
  createOIDCServiceClient,
  createOrganizationServiceClient,
  createSessionServiceClient,
  createSettingsServiceClient,
  createUserServiceClient,
} from "@zitadel/client/v2";
import { authConfig, isCustomLoginConfigured } from "./config";
import { applyCustomHeaders } from "./custom-headers";

export class ZitadelCredentialError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ZitadelCredentialError";
  }
}

export interface ZitadelClients {
  session: ReturnType<typeof createSessionServiceClient>;
  user: ReturnType<typeof createUserServiceClient>;
  settings: ReturnType<typeof createSettingsServiceClient>;
  oidc: ReturnType<typeof createOIDCServiceClient>;
  org: ReturnType<typeof createOrganizationServiceClient>;
}

let cachedSystemToken: { token: string; expiresAt: number } | null = null;
let cachedClients: { token: string; clients: ZitadelClients } | null = null;
let cachedServiceOrganization: {
  organizationId: string;
  expiresAt: number;
} | null = null;

function baseApiUrl(): string {
  if (!authConfig.zitadel.apiUrl) {
    throw new ZitadelCredentialError(
      "ZITADEL API 地址未配置：请设置 ZITADEL_URL 或 ZITADEL_API_URL。",
    );
  }
  return authConfig.zitadel.apiUrl;
}

function zitadelInstanceHost(): string {
  return (
    authConfig.zitadel.instanceHost ||
    new URL(baseApiUrl()).host
  );
}

function zitadelPublicHost(): string {
  return (
    authConfig.zitadel.publicHost ||
    new URL(authConfig.baseUrl).host
  );
}

async function systemToken(): Promise<string> {
  const now = Date.now();
  if (cachedSystemToken && cachedSystemToken.expiresAt > now) {
    return cachedSystemToken.token;
  }

  let token: string;
  if (
    authConfig.zitadel.systemUserId &&
    (authConfig.zitadel.systemUserPrivateKey ||
      authConfig.zitadel.systemUserPrivateKeyFile)
  ) {
    const key = authConfig.zitadel.systemUserPrivateKeyFile
      ? await readFile(authConfig.zitadel.systemUserPrivateKeyFile, "utf8")
      : Buffer.from(authConfig.zitadel.systemUserPrivateKey, "base64").toString(
          "utf8",
        );
    token = await newSystemToken({
      audience: authConfig.zitadel.audience || baseApiUrl(),
      subject: authConfig.zitadel.systemUserId,
      key,
    });
  } else if (authConfig.zitadel.loginClientKeyFile) {
    const key = await readFile(authConfig.zitadel.loginClientKeyFile, "utf8");
    token = await newSystemToken({
      audience: authConfig.zitadel.audience || baseApiUrl(),
      subject: "login-client",
      key,
    });
  } else if (authConfig.zitadel.serviceUserToken) {
    token = authConfig.zitadel.serviceUserToken;
  } else if (authConfig.zitadel.loginClientPat) {
    token = authConfig.zitadel.loginClientPat;
  } else {
    throw new ZitadelCredentialError(
      "ZITADEL 服务凭据未配置：请设置 ZITADEL_LOGINCLIENT_KEYFILE、系统用户 JWT 或 ZITADEL_SERVICE_USER_TOKEN。",
    );
  }

  cachedSystemToken = { token, expiresAt: now + 50 * 60 * 1000 };
  return token;
}

export async function resolveZitadelToken(): Promise<string> {
  if (!isCustomLoginConfigured()) {
    throw new ZitadelCredentialError(
      "自建登录模式未启用或凭据不完整，拒绝调用 ZITADEL Session API。",
    );
  }
  return systemToken();
}

export async function getZitadelClients(): Promise<ZitadelClients> {
  const token = await resolveZitadelToken();
  if (cachedClients && cachedClients.token === token) {
    return cachedClients.clients;
  }

  const transport = createConnectTransport({
    httpVersion: "1.1",
    baseUrl: baseApiUrl(),
    interceptors: [
      NewAuthorizationBearerInterceptor(token),
      (next) => (req) => {
        req.header.set("x-zitadel-instance-host", zitadelInstanceHost());
        req.header.set("x-zitadel-public-host", zitadelPublicHost());
        applyCustomHeaders(
          {
            set: (key, value) => req.header.set(key, value),
            delete: (key) => req.header.delete(key),
          },
          authConfig.zitadel.customRequestHeaders,
        );
        return next(req);
      },
    ],
  });
  const clients: ZitadelClients = {
    session: createSessionServiceClient(transport),
    user: createUserServiceClient(transport),
    settings: createSettingsServiceClient(transport),
    oidc: createOIDCServiceClient(transport),
    org: createOrganizationServiceClient(transport),
  };
  cachedClients = { token, clients };
  return clients;
}

export async function getServiceAccountOrganizationId(): Promise<string> {
  const now = Date.now();
  if (cachedServiceOrganization && cachedServiceOrganization.expiresAt > now) {
    return cachedServiceOrganization.organizationId;
  }
  const token = await resolveZitadelToken();
  const response = await fetch(`${baseApiUrl()}/management/v1/orgs/me`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  });
  if (!response.ok) {
    throw new ZitadelCredentialError(
      `ZITADEL 服务账号组织读取失败：${response.status}`,
    );
  }
  const payload = (await response.json()) as { org?: { id?: string } };
  if (!payload.org?.id) {
    throw new ZitadelCredentialError(
      "ZITADEL 未返回服务账号组织 ID。",
    );
  }
  cachedServiceOrganization = {
    organizationId: payload.org.id,
    expiresAt: now + 10 * 60 * 1000,
  };
  return payload.org.id;
}
