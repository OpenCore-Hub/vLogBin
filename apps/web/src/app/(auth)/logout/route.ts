import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { NextResponse } from "next/server";
import { buildLogoutUrl } from "@/lib/auth/oidc";
import { authConfig } from "@/lib/auth/config";
import { SESSION_COOKIE } from "@/lib/auth/session";
import { clearIdpFlow, clearLoginFlow } from "@/lib/auth/login-flow";
import { deleteSession } from "@/lib/auth/zitadel-session";
import {
  ZITADEL_SESSIONS_COOKIE,
  getRememberedSessions,
} from "@/lib/auth/zitadel-sessions-store";

export async function GET() {
  const jar = await cookies();
  const remembered = authConfig.mode === "oidc-custom-login"
    ? await getRememberedSessions()
    : [];
  jar.delete(SESSION_COOKIE);
  jar.delete(ZITADEL_SESSIONS_COOKIE);
  await clearLoginFlow();
  await clearIdpFlow();

  // 自建登录模式：直接终止 ZITADEL Session，再回登录页。
  if (authConfig.mode === "oidc-custom-login") {
    await Promise.allSettled(
      remembered.map((session) =>
        deleteSession({
          sessionId: session.sessionId,
          sessionToken: session.sessionToken,
        }),
      ),
    );
    redirect("/login");
  }

  // OIDC 模式：跳转 IdP 登出（end_session）
  if (authConfig.mode === "oidc") {
    const logoutUrl = await buildLogoutUrl();
    if (logoutUrl) {
      return NextResponse.redirect(logoutUrl);
    }
  }
  redirect("/login");
}
