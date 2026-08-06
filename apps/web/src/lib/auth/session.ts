import "server-only";

import { cookies } from "next/headers";
import { SignJWT, jwtVerify } from "jose";
import { authConfig, isSessionSecretConfigured } from "./config";
import { sealSecret, unsealSecret } from "./crypto";
import { OidcError, refreshTokens, verifyIdToken } from "./oidc";
import { SESSION_COOKIE } from "../env-shared";

export { SESSION_COOKIE };

export type Env = "test" | "live";

export interface Session {
  sub: string;
  email: string;
  name: string;
  roles: string[];
  workspaceId: string;
  env: Env;
  accessToken?: string;
  refreshToken?: string;
  tokenExp?: number;
  expiresAt: number;
}

interface SessionClaims {
  sub: string;
  email: string;
  name: string;
  roles: string[];
  workspaceId: string;
  env: Env;
  /** base64url(iv).base64url(tag).base64url(data) */
  at?: string;
  rt?: string;
  exp_: number;
  /** 会话过期时间（jose payload 注入） */
  exp?: number;
}

export class SessionError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "SessionError";
  }
}

/* ---------------- Token 加解密（AES-256-GCM，密钥由 SESSION_SECRET 派生） ---------------- */

/* ---------------- 会话 JWT ---------------- */

function secretKey(): Uint8Array {
  return new TextEncoder().encode(authConfig.sessionSecret);
}

async function signClaims(claims: SessionClaims): Promise<string> {
  const exp =
    claims.exp ?? Math.floor(Date.now() / 1000) + authConfig.sessionMaxAgeSeconds;
  return new SignJWT({ ...claims, exp_: undefined })
    .setProtectedHeader({ alg: "HS256" })
    .setIssuedAt()
    .setExpirationTime(exp)
    .sign(secretKey());
}

async function verifyClaims(token: string): Promise<SessionClaims> {
  try {
    const { payload } = await jwtVerify(token, secretKey(), {
      algorithms: ["HS256"],
    });
    if (typeof payload.sub !== "string" || !payload.sub) {
      throw new SessionError("会话缺少主体");
    }
    return payload as unknown as SessionClaims;
  } catch (err) {
    if (err instanceof SessionError) throw err;
    throw new SessionError(
      err instanceof Error ? err.message : "会话校验失败",
    );
  }
}

/* ---------------- 会话操作 ---------------- */

function cookieOptions(maxAge: number) {
  return {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax" as const,
    path: "/",
    maxAge,
  };
}

export function sessionToClaims(s: Omit<Session, "expiresAt">): SessionClaims {
  return {
    sub: s.sub,
    email: s.email,
    name: s.name,
    roles: s.roles,
    workspaceId: s.workspaceId,
    env: s.env,
    at: s.accessToken ? sealSecret(s.accessToken) : undefined,
    rt: s.refreshToken ? sealSecret(s.refreshToken) : undefined,
    exp_: s.tokenExp ?? 0,
  };
}

export function claimsToSession(c: SessionClaims): Session {
  return {
    sub: c.sub,
    email: c.email,
    name: c.name,
    roles: c.roles,
    workspaceId: c.workspaceId,
    env: c.env,
    accessToken: c.at ? unsealSecret(c.at) : undefined,
    refreshToken: c.rt ? unsealSecret(c.rt) : undefined,
    tokenExp: c.exp_ || undefined,
    expiresAt: 0, // 由调用方补充
  };
}

export interface CreateSessionInput {
  sub: string;
  email: string;
  name: string;
  roles: string[];
  workspaceId: string;
  env: Env;
  accessToken?: string;
  refreshToken?: string;
  tokenExp?: number;
}

/** 生成 vLogBin 会话 JWT，不写 cookie；由调用方决定如何提交。 */
export async function createSessionToken(
  input: CreateSessionInput,
): Promise<{ token: string; expiresAt: number }> {
  if (!isSessionSecretConfigured()) {
    throw new SessionError(
      "未配置 SESSION_SECRET（至少 32 字符），拒绝建立会话",
    );
  }
  const expiresAt = Math.floor(Date.now() / 1000) + authConfig.sessionMaxAgeSeconds;
  const token = await signClaims({
    ...sessionToClaims(input),
    exp: expiresAt,
  });
  return { token, expiresAt };
}

/** 读取当前会话（服务端）。 */
export async function getSession(): Promise<Session | null> {
  const jar = await cookies();
  const raw = jar.get(SESSION_COOKIE)?.value;
  if (!raw) return null;
  try {
    const claims = await verifyClaims(raw);
    const session = claimsToSession(claims);
    session.expiresAt = claims.exp ?? 0;
    return session;
  } catch {
    return null;
  }
}

/** 读取会话，不存在或失效则抛出。 */
export async function requireSession(): Promise<Session> {
  const session = await getSession();
  if (!session) {
    throw new SessionError("未登录或会话已失效");
  }
  return session;
}

/** 建立会话（写入 httpOnly cookie）。 */
export async function createSession(input: CreateSessionInput): Promise<void> {
  const { token } = await createSessionToken(input);
  const jar = await cookies();
  jar.set(SESSION_COOKIE, token, cookieOptions(authConfig.sessionMaxAgeSeconds));
}

/** 更新会话（保留身份，覆盖凭据/环境）。 */
export async function updateSession(
  patch: Partial<CreateSessionInput>,
): Promise<Session> {
  const current = await getSession();
  if (!current) throw new SessionError("会话不存在");
  const next: CreateSessionInput = {
    sub: patch.sub ?? current.sub,
    email: patch.email ?? current.email,
    name: patch.name ?? current.name,
    roles: patch.roles ?? current.roles,
    workspaceId: patch.workspaceId ?? current.workspaceId,
    env: patch.env ?? current.env,
    accessToken: patch.accessToken ?? current.accessToken,
    refreshToken: patch.refreshToken ?? current.refreshToken,
    tokenExp: patch.tokenExp ?? current.tokenExp,
  };
  await createSession(next);
  return (await getSession()) as Session;
}

/** 销毁会话。 */
export async function destroySession(): Promise<void> {
  const jar = await cookies();
  jar.delete(SESSION_COOKIE);
}

/** 会话是否接近过期（供滑动续期判断）。 */
export function isSessionExpiring(session: Session): boolean {
  const now = Math.floor(Date.now() / 1000);
  return session.expiresAt - now < 5 * 60;
}

/* ---------------- 静默续期（OIDC access token） ---------------- */

/** 判断 access token 是否过期（含提前量）。 */
export function isAccessTokenExpiring(session: Session): boolean {
  return Boolean(
    session.refreshToken &&
      session.tokenExp &&
      session.tokenExp - Math.floor(Date.now() / 1000) <
        authConfig.tokenRefreshLeewaySeconds,
  );
}

/**
 * 纯刷新：返回带新凭据的会话对象，不写 cookie。
 * 调用方按自身上下文决定如何提交（Server Action 直接 updateSession；
 * RSC 用 after() 延后提交）。
 */
export async function refreshAccessToken(
  session: Session,
): Promise<Session> {
  if (!session.refreshToken) return session;
  const refreshed = await refreshTokens(session.refreshToken);
  const verified = await verifyIdToken(refreshed.idToken);
  return {
    ...session,
    email: verified.email ?? session.email,
    name: verified.name ?? session.name,
    accessToken: refreshed.accessToken,
    refreshToken: refreshed.refreshToken,
    tokenExp: Math.floor(Date.now() / 1000) + refreshed.expiresIn,
  };
}

/**
 * 若 access token 已过期（或接近过期）且有 refresh token，则静默刷新。
 * 默认会写回 cookie（适合 Server Action / Route Handler）；
 * 传 commit=false 时仅返回刷新后的会话，由调用方决定提交方式。
 */
export async function refreshAccessTokenIfNeeded(
  session: Session,
  options: { commit?: boolean } = {},
): Promise<Session> {
  const { commit = true } = options;
  if (isAccessTokenExpiring(session)) {
    try {
      const updated = await refreshAccessToken(session);
      if (commit) {
        await updateSession({
          accessToken: updated.accessToken,
          refreshToken: updated.refreshToken,
          tokenExp: updated.tokenExp,
          email: updated.email,
          name: updated.name,
        });
      }
      return updated;
    } catch (err) {
      if (err instanceof OidcError && err.code === "token-failed") {
        // refresh token 失效：销毁会话，要求重新登录
        await destroySession();
      }
      throw err;
    }
  }
  return session;
}
