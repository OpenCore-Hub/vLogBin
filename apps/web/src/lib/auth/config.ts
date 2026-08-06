import "server-only";

export type AuthMode = "oidc" | "oidc-custom-login" | "operator-token";

function str(v: string | undefined, fallback = ""): string {
  return v && v.trim().length > 0 ? v.trim() : fallback;
}

function num(v: string | undefined, fallback: number): number {
  const n = Number(v);
  return Number.isFinite(n) && n > 0 ? n : fallback;
}

/**
 * 认证配置（服务端专用）。
 * - oidc：ZITADEL 授权码 + PKCE（生产模式，托管登录页）
 * - oidc-custom-login：ZITADEL Session API 自建登录页（品牌一致性）
 * - operator-token：对接 API 的 OPERATOR_TOKEN 认证（本地/单租户开发模式）
 */
export const authConfig = {
  mode: (str(process.env.AUTH_MODE, "oidc") === "operator-token"
    ? "operator-token"
    : str(process.env.AUTH_MODE, "oidc") === "oidc-custom-login"
      ? "oidc-custom-login"
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
    apiUrl: str(
      process.env.ZITADEL_API_URL,
      str(process.env.ZITADEL_URL),
    ).replace(/\/+$/, ""),
    clientId: str(process.env.ZITADEL_CLIENT_ID),
    clientSecret: str(process.env.ZITADEL_CLIENT_SECRET),
    redirectUri: str(
      process.env.ZITADEL_REDIRECT_URI,
      `${str(process.env.APP_BASE_URL, "http://localhost:3000").replace(/\/+$/, "")}/callback`,
    ),
    postLogoutRedirectUri: str(
      process.env.ZITADEL_POST_LOGOUT_REDIRECT_URI,
      `${str(process.env.APP_BASE_URL, "http://localhost:3000").replace(/\/+$/, "")}/login`,
    ),
    scopes: str(process.env.ZITADEL_SCOPES, "openid profile email offline_access"),
    audience: str(
      process.env.AUDIENCE,
      str(process.env.ZITADEL_API_URL, str(process.env.ZITADEL_URL)),
    ).replace(/\/+$/, ""),
    loginClientKeyFile: str(process.env.ZITADEL_LOGINCLIENT_KEYFILE),
    systemUserId: str(process.env.SYSTEM_USER_ID),
    systemUserPrivateKey: str(process.env.SYSTEM_USER_PRIVATE_KEY),
    systemUserPrivateKeyFile: str(process.env.SYSTEM_USER_PRIVATE_KEY_FILE),
    serviceUserToken: str(process.env.ZITADEL_SERVICE_USER_TOKEN),
    loginClientPat: str(process.env.ZITADEL_LOGIN_CLIENT_PAT),
    authVaultServiceToken: str(process.env.AUTH_VAULT_SERVICE_TOKEN),
    authVaultPrivateKey: str(process.env.AUTH_VAULT_SERVICE_PRIVATE_KEY),
    authVaultPrivateKeyFile: str(process.env.AUTH_VAULT_SERVICE_PRIVATE_KEY_FILE),
    authVaultAudience: str(process.env.AUTH_VAULT_AUDIENCE, "vlogbin-auth-vault"),
    trustedDomains: str(process.env.ZITADEL_TRUSTED_DOMAIN, "").split(",").map((v) => v.trim()).filter(Boolean),
    customLoginAllowedOrgs: str(
      process.env.ZITADEL_CUSTOM_LOGIN_ALLOWED_ORGS,
      "",
    ).split(",").map((v) => v.trim()).filter(Boolean),
    customLoginAllowedUsers: str(
      process.env.ZITADEL_CUSTOM_LOGIN_ALLOWED_USERS,
      "",
    ).split(",").map((v) => v.trim()).filter(Boolean),
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

/** 自建登录模式所需的 ZITADEL 服务凭据是否完整。 */
export function isCustomLoginConfigured(): boolean {
  if (authConfig.mode !== "oidc-custom-login") {
    return false;
  }
  return Boolean(
    authConfig.zitadel.apiUrl &&
      (authConfig.zitadel.loginClientKeyFile ||
        (authConfig.zitadel.systemUserId &&
          (authConfig.zitadel.systemUserPrivateKey ||
            authConfig.zitadel.systemUserPrivateKeyFile)) ||
        authConfig.zitadel.serviceUserToken ||
        authConfig.zitadel.loginClientPat),
  );
}

/** 用户/组织白名单为空表示全部放行；非空时仅在名单内使用自建登录。 */
export function isCustomLoginAllowedForUser(userId: string | undefined): boolean {
  if (!userId || authConfig.zitadel.customLoginAllowedUsers.length === 0) {
    return true;
  }
  return authConfig.zitadel.customLoginAllowedUsers.includes(userId);
}

export function isCustomLoginAllowedForOrg(orgId: string | undefined): boolean {
  if (!orgId || authConfig.zitadel.customLoginAllowedOrgs.length === 0) {
    return true;
  }
  return authConfig.zitadel.customLoginAllowedOrgs.includes(orgId);
}

/** 会话密钥是否配置（未配置时禁止建立会话）。 */
export function isSessionSecretConfigured(): boolean {
  return authConfig.sessionSecret.length >= 32;
}
