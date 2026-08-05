import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { NextResponse } from "next/server";
import { buildLogoutUrl } from "@/lib/auth/oidc";
import { authConfig } from "@/lib/auth/config";
import { SESSION_COOKIE } from "@/lib/auth/session";

export async function GET() {
  const jar = await cookies();
  jar.delete(SESSION_COOKIE);

  // OIDC 模式：跳转 IdP 登出（end_session）
  if (authConfig.mode === "oidc") {
    const logoutUrl = await buildLogoutUrl();
    if (logoutUrl) {
      return NextResponse.redirect(logoutUrl);
    }
  }
  redirect("/login");
}
