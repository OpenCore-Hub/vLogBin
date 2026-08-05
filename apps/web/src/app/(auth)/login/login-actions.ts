"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import {
  buildAuthorizationUrl,
  generatePkcePair,
  generateState,
} from "@/lib/auth/oidc";
import { authConfig, isOidcConfigured } from "@/lib/auth/config";
import { createSession } from "@/lib/auth/session";
import { rememberEnv } from "@/lib/env";
import { loginFormSchema } from "@/lib/api/schemas";
import {
  OIDC_NEXT_COOKIE,
  OIDC_STATE_COOKIE,
  OIDC_VERIFIER_COOKIE,
} from "./login-state";
import type { LoginActionState } from "./login-state";

function safeNext(value: string | null): string {
  const v = value ?? "/console";
  // 只允许站内路径，防止开放重定向
  return v.startsWith("/") && !v.startsWith("//") ? v : "/console";
}

/** 发起 OIDC 登录：生成 PKCE 与 state，重定向到授权端点。 */
export async function startOidcLogin(formData: FormData): Promise<void> {
  const next = safeNext(String(formData.get("next") ?? ""));
  if (!isOidcConfigured()) {
    redirect("/auth/error?error=oidc_not_configured");
  }
  const state = generateState();
  const pkce = generatePkcePair();
  const jar = await cookies();
  jar.set(OIDC_STATE_COOKIE, state, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 600,
  });
  jar.set(OIDC_VERIFIER_COOKIE, pkce.verifier, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 600,
  });
  jar.set(OIDC_NEXT_COOKIE, next, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 600,
  });
  const url = await buildAuthorizationUrl({
    state,
    codeChallenge: pkce.challenge,
  });
  redirect(url);
}

/** operator-token 模式登录：用令牌探测平台 API，校验通过后建立会话。 */
export async function loginWithOperatorToken(
  _prev: LoginActionState,
  formData: FormData,
): Promise<LoginActionState> {
  const next = safeNext(String(formData.get("next") ?? ""));
  const parsed = loginFormSchema.safeParse({
    token: String(formData.get("token") ?? ""),
  });
  if (!parsed.success) {
    return {
      ok: false,
      error: parsed.error.issues[0]?.message ?? "输入无效",
      next,
    };
  }
  const token = parsed.data.token;

  // 真实探测：调用 operator API 的只读端点验证令牌有效性
  let res: Response;
  try {
    res = await fetch(`${authConfig.apiBaseUrl}/v1/operator/regions`, {
      headers: { Authorization: `Bearer ${token}` },
      cache: "no-store",
    });
  } catch {
    return {
      ok: false,
      error: `无法连接平台 API（${authConfig.apiBaseUrl}），请检查网络或配置。`,
      next,
    };
  }
  if (res.status === 401 || res.status === 403) {
    return { ok: false, error: "令牌无效或已失效，请重新输入。", next };
  }
  if (!res.ok) {
    return {
      ok: false,
      error: `平台 API 校验失败（${res.status}），请稍后重试。`,
      next,
    };
  }

  await createSession({
    sub: "operator",
    email: "operator@local",
    name: "Operator",
    roles: ["operator"],
    workspaceId: "default",
    env: "test",
    accessToken: token,
  });
  await rememberEnv("test");
  return { ok: true, next };
}
