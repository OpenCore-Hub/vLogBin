import "server-only";

import { readFile } from "node:fs/promises";
import { SignJWT, importPKCS8 } from "jose";
import { authConfig } from "./config";

export interface CreateAuthVaultInput {
  userSub: string;
  email: string;
  name: string;
  roles: string[];
  workspaceId: string;
  env: string;
  accessToken: string;
  refreshToken: string;
  tokenExp: number;
  ttlSeconds: number;
}

export interface AuthVault {
  id: string;
  userSub: string;
  email: string;
  name: string;
  roles: string[];
  workspaceId: string;
  env: string;
  accessToken: string;
  refreshToken: string;
  tokenExp: number;
  expiresAt: string;
}

export class AuthVaultError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = "AuthVaultError";
  }
}

let cachedServiceToken: { token: string; expiresAt: number } | null = null;

async function resolveServiceToken(): Promise<string> {
  if (
    authConfig.zitadel.authVaultPrivateKey ||
    authConfig.zitadel.authVaultPrivateKeyFile
  ) {
    if (cachedServiceToken && cachedServiceToken.expiresAt > Date.now()) {
      return cachedServiceToken.token;
    }
    const key = authConfig.zitadel.authVaultPrivateKeyFile
      ? await readFile(authConfig.zitadel.authVaultPrivateKeyFile, "utf8")
      : authConfig.zitadel.authVaultPrivateKey;
    const token = await new SignJWT({})
      .setProtectedHeader({ alg: "RS256" })
      .setIssuer("vlogbin-web")
      .setSubject("web-backend")
      .setAudience(authConfig.zitadel.authVaultAudience)
      .setIssuedAt()
      .setExpirationTime("5m")
      .sign(await importPKCS8(key, "RS256"));
    cachedServiceToken = { token, expiresAt: Date.now() + 4 * 60 * 1000 };
    return token;
  }
  if (authConfig.zitadel.authVaultServiceToken) {
    return authConfig.zitadel.authVaultServiceToken;
  }
  throw new AuthVaultError(
    "AUTH_VAULT_SERVICE_PRIVATE_KEY 或 AUTH_VAULT_SERVICE_TOKEN 未配置，无法访问服务端会话保险库。",
    "not-configured",
    503,
  );
}

async function request<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const serviceToken = await resolveServiceToken();
  let res: Response;
  try {
    res = await fetch(`${authConfig.apiBaseUrl}${path}`, {
      ...init,
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${serviceToken}`,
        ...init?.headers,
      },
      cache: "no-store",
    });
  } catch (err) {
    throw new AuthVaultError(
      `Auth vault unreachable: ${err instanceof Error ? err.message : String(err)}`,
      "unreachable",
      0,
    );
  }
  if (!res.ok) {
    let message = res.statusText;
    try {
      const body = (await res.json()) as { error?: { message?: string } };
      message = body.error?.message ?? message;
    } catch {
      // keep status text
    }
    throw new AuthVaultError(message, "request-failed", res.status);
  }
  return (await res.json()) as T;
}

export async function createAuthVault(
  input: CreateAuthVaultInput,
): Promise<AuthVault> {
  const data = await request<{ vault: AuthVault }>("/v1/auth/vault", {
    method: "POST",
    body: JSON.stringify(input),
  });
  if (!data.vault?.id) {
    throw new AuthVaultError("auth vault create response missing id", "invalid-response", 502);
  }
  return data.vault;
}

export async function getAuthVault(id: string): Promise<AuthVault> {
  const data = await request<{ vault: AuthVault }>(
    `/v1/auth/vault/${encodeURIComponent(id)}`,
  );
  if (!data.vault?.id) {
    throw new AuthVaultError("auth vault get response missing vault", "invalid-response", 502);
  }
  return data.vault;
}

export async function deleteAuthVault(id: string): Promise<void> {
  const serviceToken = await resolveServiceToken();
  const res = await fetch(`${authConfig.apiBaseUrl}/v1/auth/vault/${encodeURIComponent(id)}`, {
    method: "DELETE",
    headers: {
      Authorization: `Bearer ${serviceToken}`,
    },
  });
  if (!res.ok) {
    throw new AuthVaultError(
      `delete auth vault failed: ${res.status}`,
      "request-failed",
      res.status,
    );
  }
}
