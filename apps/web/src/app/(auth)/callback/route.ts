import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { NextResponse, type NextRequest } from "next/server";
import { exchangeCode, verifyIdToken } from "@/lib/auth/oidc";
import { createSession } from "@/lib/auth/session";
import { provisionWorkspace } from "@/lib/api/operator";
import {
  OIDC_STATE_COOKIE,
  OIDC_VERIFIER_COOKIE,
  OIDC_NEXT_COOKIE,
} from "../login/login-state";

function fail(error: string, description?: string): NextResponse {
  const url = new URL("/error", "http://localhost");
  url.searchParams.set("error", error);
  if (description) url.searchParams.set("description", description);
  return NextResponse.redirect(url.toString());
}

export async function GET(req: NextRequest) {
  const { searchParams } = req.nextUrl;
  const code = searchParams.get("code");
  const state = searchParams.get("state");
  const oidcError = searchParams.get("error");

  const jar = await cookies();
  const expectedState = jar.get(OIDC_STATE_COOKIE)?.value;
  const verifier = jar.get(OIDC_VERIFIER_COOKIE)?.value;
  const nextRaw = jar.get(OIDC_NEXT_COOKIE)?.value;
  const next = nextRaw && nextRaw.startsWith("/") && !nextRaw.startsWith("//")
    ? nextRaw
    : "/console";

  // 清理临时 cookies
  jar.delete(OIDC_STATE_COOKIE);
  jar.delete(OIDC_VERIFIER_COOKIE);
  jar.delete(OIDC_NEXT_COOKIE);

  if (oidcError) {
    return fail("oidc_denied", oidcError);
  }
  if (!code) return fail("missing_code");
  if (!state || !expectedState || state !== expectedState) {
    return fail("state_mismatch", "登录状态校验失败，请重试");
  }
  if (!verifier) return fail("missing_verifier");

  let tokenSet;
  try {
    tokenSet = await exchangeCode(code, verifier);
  } catch (err) {
    return fail(
      "token_exchange_failed",
      err instanceof Error ? err.message : "令牌交换失败",
    );
  }

  let identity;
  try {
    identity = await verifyIdToken(tokenSet.idToken);
  } catch (err) {
    return fail(
      "invalid_id_token",
      err instanceof Error ? err.message : "身份令牌校验失败",
    );
  }

  if (!identity.sub) return fail("missing_subject");

  // R11：注册即初始化 —— 同步创建默认 workspace 并为首用户自动授予
  // provider_admin。service 层在单个事务内完成，任一失败则整体回滚；
  // 此处任一失败都视为注册失败，用户可重试。
  let workspaceId = "default";
  try {
    const signup = await provisionWorkspace(tokenSet.accessToken, {
      email: identity.email ?? "",
      name: identity.name ?? identity.preferredUsername ?? identity.email ?? "",
    });
    if (!signup.workspace?.id) {
      return fail("signup_failed", "工作区初始化失败，请重试");
    }
    workspaceId = signup.workspace.id;
  } catch (err) {
    return fail(
      "signup_failed",
      err instanceof Error ? err.message : "工作区初始化失败，请重试",
    );
  }

  await createSession({
    sub: identity.sub,
    email: identity.email ?? "",
    name: identity.name ?? identity.preferredUsername ?? identity.email ?? "",
    roles: identity.roles,
    workspaceId,
    env: "test",
    accessToken: tokenSet.accessToken,
    refreshToken: tokenSet.refreshToken,
    tokenExp: Math.floor(Date.now() / 1000) + tokenSet.expiresIn,
  });

  return redirect(next);
}
