import "server-only";

export type AuthMode = "oidc" | "operator-token";

function str(v: string | undefined, fallback = ""): string {
  return v && v.trim().length > 0 ? v.trim() : fallback;
}

function num(v: string | undefined, fallback: number): number {
  const n = Number(v);
  return Number.isFinite(n) && n > 0 ? n : fallback;
}

/**
 * 认证配置（服务端专用）。
 * - oidc：ZITADEL 授权码 + PKCE（生产模式，多租户）
 * - operator-token：对接 API 的 OPERATOR_TOKEN 认证（本地/单租户开发模式）
 */
export const authConfig = {
  mode: (str(process.env.AUTH_MODE, "oidc") === "operator-token"
    ? "operator-token"
    : "oidc") as AuthMode,
  baseUrl: str(process.env.APP_BASE_URL, "http://localhost:3000").replace(
    /\/+$/,
    "",
  ),
  sessionSecret: str(process.env.SESSION_SECRET),
  sessionMaxAgeSeconds: num(
    process.env.SESSION_MAX_AGE_SECONDS,
    60 * 60 * 24 * 7,
  ),
  tokenRefreshLeewaySeconds: num(process.env.TOKEN_REFRESH_LEEWAY_SECONDS, 120),
  operatorToken: str(process.env.OPERATOR_TOKEN),
  zitadel: {
    issuer: str(process.env.ZITADEL_URL).replace(/\/+$/, ""),
    clientId: str(process.env.ZITADEL_CLIENT_ID),
    clientSecret: str(process.env.ZITADEL_CLIENT_SECRET),
    redirectUri: str(
      process.env.ZITADEL_REDIRECT_URI,
      `${str(process.env.APP_BASE_URL, "http://localhost:3000").replace(/\/+$/, "")}/auth/callback`,
    ),
    postLogoutRedirectUri: str(
      process.env.ZITADEL_POST_LOGOUT_REDIRECT_URI,
      `${str(process.env.APP_BASE_URL, "http://localhost:3000").replace(/\/+$/, "")}/login`,
    ),
    scopes: str(process.env.ZITADEL_SCOPES, "openid profile email offline_access"),
  },
  apiBaseUrl: str(
    process.env.PLATFORM_API_URL,
    "http://localhost:8080",
  ).replace(/\/+$/, ""),
} as const;

/** OIDC 是否完整配置（可发起登录）。 */
export function isOidcConfigured(): boolean {
  return (
    authConfig.zitadel.issuer.length > 0 && authConfig.zitadel.clientId.length > 0
  );
}

/** 会话密钥是否配置（未配置时禁止建立会话）。 */
export function isSessionSecretConfigured(): boolean {
  return authConfig.sessionSecret.length >= 32;
}
