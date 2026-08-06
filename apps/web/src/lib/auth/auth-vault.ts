import "server-only";

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

async function request<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  if (!authConfig.zitadel.authVaultServiceToken) {
    throw new AuthVaultError(
      "AUTH_VAULT_SERVICE_TOKEN 未配置，无法访问服务端会话保险库。",
      "not-configured",
      503,
    );
  }
  let res: Response;
  try {
    res = await fetch(`${authConfig.apiBaseUrl}${path}`, {
      ...init,
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${authConfig.zitadel.authVaultServiceToken}`,
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
  const res = await fetch(`${authConfig.apiBaseUrl}/v1/auth/vault/${encodeURIComponent(id)}`, {
    method: "DELETE",
    headers: {
      Authorization: `Bearer ${authConfig.zitadel.authVaultServiceToken}`,
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
