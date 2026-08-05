"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import {
  buildAuthorizationUrl,
  generatePkcePair,
  generateState,
} from "@/lib/auth/oidc";
import { isOidcConfigured } from "@/lib/auth/config";
import {
  OIDC_NEXT_COOKIE,
  OIDC_STATE_COOKIE,
  OIDC_VERIFIER_COOKIE,
} from "../login/login-state";

function safeNext(value: string | null): string {
  const v = value ?? "/console";
  // 只允许站内路径，防止开放重定向
  return v.startsWith("/") && !v.startsWith("//") ? v : "/console";
}

/**
 * 发起 OIDC 注册（ZITADEL 自助注册）：
 * 生成 PKCE 与 state 后重定向到授权端点，并携带 prompt=create
 * 直达注册界面；回跳 callback 后按注册用户建立会话。
 */
export async function startOidcSignup(formData: FormData): Promise<void> {
  const next = safeNext(String(formData.get("next") ?? ""));
  if (!isOidcConfigured()) {
    redirect("/auth/error?error=oidc_not_configured");
  }
  const state = generateState();
  const pkce = generatePkcePair();
  const jar = await cookies();
  const cookieBase = {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax" as const,
    path: "/",
    maxAge: 600,
  };
  jar.set(OIDC_STATE_COOKIE, state, cookieBase);
  jar.set(OIDC_VERIFIER_COOKIE, pkce.verifier, cookieBase);
  jar.set(OIDC_NEXT_COOKIE, next, cookieBase);
  const url = await buildAuthorizationUrl({
    state,
    codeChallenge: pkce.challenge,
    prompt: "create",
  });
  redirect(url);
}
