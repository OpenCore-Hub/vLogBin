import "server-only";

import { base64url, createRemoteJWKSet, jwtVerify } from "jose";
import { createHash, randomBytes } from "node:crypto";
import { authConfig, isOidcConfigured } from "./config";

/** ZITADEL / 通用 OIDC Discovery 文档。 */
export interface OidcDiscovery {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  userinfo_endpoint?: string;
  end_session_endpoint?: string;
  jwks_uri: string;
}

const DISCOVERY_TIMEOUT_MS = 8_000;

let cachedDiscovery: { url: string; doc: OidcDiscovery } | null = null;

export class OidcError extends Error {
  constructor(
    message: string,
    public readonly code:
      | "not-configured"
      | "discovery-failed"
      | "token-failed"
      | "invalid-token"
      | "state-mismatch",
  ) {
    super(message);
    this.name = "OidcError";
  }
}

async function discover(): Promise<OidcDiscovery> {
  if (!isOidcConfigured()) {
    throw new OidcError(
      "ZITADEL 未配置：请设置 ZITADEL_URL 与 ZITADEL_CLIENT_ID。",
      "not-configured",
    );
  }
  const url = `${authConfig.zitadel.issuer}/.well-known/openid-configuration`;
  if (cachedDiscovery && cachedDiscovery.url === url) {
    return cachedDiscovery.doc;
  }
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), DISCOVERY_TIMEOUT_MS);
  try {
    const res = await fetch(url, {
      signal: controller.signal,
      headers: { Accept: "application/json" },
      cache: "no-store",
    });
    if (!res.ok) {
      throw new OidcError(
        `OIDC Discovery 失败（${res.status}）：${url}`,
        "discovery-failed",
      );
    }
    const doc = (await res.json()) as OidcDiscovery;
    if (!doc.authorization_endpoint || !doc.token_endpoint || !doc.jwks_uri) {
      throw new OidcError("OIDC Discovery 响应缺少必需端点。", "discovery-failed");
    }
    cachedDiscovery = { url, doc };
    return doc;
  } catch (err) {
    if (err instanceof OidcError) throw err;
    throw new OidcError(
      `OIDC Discovery 失败：${err instanceof Error ? err.message : String(err)}`,
      "discovery-failed",
    );
  } finally {
    clearTimeout(timer);
  }
}

/** PKCE 挑战对（S256）。 */
export function generatePkcePair(): {
  verifier: string;
  challenge: string;
} {
  const verifier = base64url.encode(randomBytes(48));
  const challenge = base64url.encode(
    createHash("sha256").update(verifier).digest(),
  );
  return { verifier, challenge };
}

/** 随机 state（防 CSRF）。 */
export function generateState(): string {
  return base64url.encode(randomBytes(24));
}

/** 构建授权码请求 URL（Authorization Code + PKCE）。
 *  `prompt=create` 为 ZITADEL 自助注册扩展：直接进入注册界面。 */
export async function buildAuthorizationUrl(params: {
  state: string;
  codeChallenge: string;
  /** 可选 prompt 参数（如 "create" 直达注册流程）。 */
  prompt?: string;
}): Promise<string> {
  const doc = await discover();
  const url = new URL(doc.authorization_endpoint);
  url.searchParams.set("client_id", authConfig.zitadel.clientId);
  url.searchParams.set("redirect_uri", authConfig.zitadel.redirectUri);
  url.searchParams.set("response_type", "code");
  url.searchParams.set("scope", authConfig.zitadel.scopes);
  url.searchParams.set("state", params.state);
  url.searchParams.set("code_challenge", params.codeChallenge);
  url.searchParams.set("code_challenge_method", "S256");
  url.searchParams.set("nonce", base64url.encode(randomBytes(16)));
  if (params.prompt) {
    url.searchParams.set("prompt", params.prompt);
  }
  return url.toString();
}

export interface TokenSet {
  idToken: string;
  accessToken: string;
  refreshToken?: string;
  expiresIn: number;
  raw: unknown;
}

/** 用授权码换取令牌。 */
export async function exchangeCode(
  code: string,
  codeVerifier: string,
): Promise<TokenSet> {
  const doc = await discover();
  const body = new URLSearchParams({
    grant_type: "authorization_code",
    code,
    redirect_uri: authConfig.zitadel.redirectUri,
    client_id: authConfig.zitadel.clientId,
    code_verifier: codeVerifier,
  });
  if (authConfig.zitadel.clientSecret) {
    body.set("client_secret", authConfig.zitadel.clientSecret);
  }
  let res: Response;
  try {
    res = await fetch(doc.token_endpoint, {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        Accept: "application/json",
      },
      body,
      cache: "no-store",
    });
  } catch (err) {
    throw new OidcError(
      `令牌交换请求失败：${err instanceof Error ? err.message : String(err)}`,
      "token-failed",
    );
  }
  const json = (await res.json().catch(() => ({}))) as Record<string, unknown>;
  if (!res.ok) {
    throw new OidcError(
      `令牌交换失败（${res.status}）：${String(json.error_description ?? json.error ?? res.statusText)}`,
      "token-failed",
    );
  }
  const idToken = json.id_token;
  const accessToken = json.access_token;
  if (typeof idToken !== "string" || typeof accessToken !== "string") {
    throw new OidcError("令牌交换响应缺少 id_token / access_token。", "token-failed");
  }
  return {
    idToken,
    accessToken,
    refreshToken:
      typeof json.refresh_token === "string" ? json.refresh_token : undefined,
    expiresIn:
      typeof json.expires_in === "number" ? json.expires_in : 60 * 60,
    raw: json,
  };
}

/** 用 refresh_token 换取新令牌（静默续期）。 */
export async function refreshTokens(
  refreshToken: string,
): Promise<TokenSet> {
  const doc = await discover();
  const body = new URLSearchParams({
    grant_type: "refresh_token",
    refresh_token: refreshToken,
    client_id: authConfig.zitadel.clientId,
    scope: authConfig.zitadel.scopes,
  });
  if (authConfig.zitadel.clientSecret) {
    body.set("client_secret", authConfig.zitadel.clientSecret);
  }
  const res = await fetch(doc.token_endpoint, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      Accept: "application/json",
    },
    body,
    cache: "no-store",
  });
  const json = (await res.json().catch(() => ({}))) as Record<string, unknown>;
  if (!res.ok) {
    throw new OidcError(
      `令牌刷新失败（${res.status}）：${String(json.error_description ?? json.error ?? res.statusText)}`,
      "token-failed",
    );
  }
  const idToken = json.id_token;
  const accessToken = json.access_token;
  if (typeof idToken !== "string" || typeof accessToken !== "string") {
    throw new OidcError("令牌刷新响应缺少 id_token / access_token。", "token-failed");
  }
  return {
    idToken,
    accessToken,
    refreshToken:
      typeof json.refresh_token === "string" ? json.refresh_token : refreshToken,
    expiresIn:
      typeof json.expires_in === "number" ? json.expires_in : 60 * 60,
    raw: json,
  };
}

export interface VerifiedIdToken {
  sub: string;
  email?: string;
  emailVerified?: boolean;
  name?: string;
  preferredUsername?: string;
  roles: string[];
  nonce?: string;
}

/** 校验 id_token（iss / aud / exp / 算法白名单）并提取声明。 */
export async function verifyIdToken(idToken: string): Promise<VerifiedIdToken> {
  const doc = await discover();
  const jwks = createRemoteJWKSet(new URL(doc.jwks_uri));
  let payload: Record<string, unknown>;
  try {
    const result = await jwtVerify(idToken, jwks, {
      issuer: authConfig.zitadel.issuer,
      audience: authConfig.zitadel.clientId,
      algorithms: ["RS256", "RS384", "RS512", "ES256", "ES384", "ES512"],
    });
    payload = result.payload;
  } catch (err) {
    throw new OidcError(
      `id_token 校验失败：${err instanceof Error ? err.message : String(err)}`,
      "invalid-token",
    );
  }
  const rolesRaw = payload["urn:zitadel:iam:roles"];
  const roles = Array.isArray(rolesRaw)
    ? rolesRaw.filter((r): r is string => typeof r === "string")
    : typeof rolesRaw === "string"
      ? [rolesRaw]
      : [];
  return {
    sub: String(payload.sub ?? ""),
    email: typeof payload.email === "string" ? payload.email : undefined,
    emailVerified:
      typeof payload.email_verified === "boolean"
        ? payload.email_verified
        : undefined,
    name: typeof payload.name === "string" ? payload.name : undefined,
    preferredUsername:
      typeof payload.preferred_username === "string"
        ? payload.preferred_username
        : undefined,
    roles,
    nonce: typeof payload.nonce === "string" ? payload.nonce : undefined,
  };
}

/** 登出地址（end_session），无则返回 null。 */
export async function buildLogoutUrl(): Promise<string | null> {
  try {
    const doc = await discover();
    if (!doc.end_session_endpoint) return null;
    const url = new URL(doc.end_session_endpoint);
    url.searchParams.set(
      "post_logout_redirect_uri",
      authConfig.zitadel.postLogoutRedirectUri,
    );
    return url.toString();
  } catch {
    return null;
  }
}
