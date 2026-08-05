import { NextResponse, type NextRequest } from "next/server";
import { SESSION_COOKIE, SESSION_MAX_AGE_SECONDS } from "./lib/env-shared";

/** 受保护路由前缀（粗粒度；细粒度 RBAC 由服务端校验）。 */
const PROTECTED_PREFIXES = ["/console", "/ops"];

export function middleware(req: NextRequest) {
  const { pathname } = req.nextUrl;
  const isProtected = PROTECTED_PREFIXES.some((p) => pathname.startsWith(p));
  const sessionCookie = req.cookies.get(SESSION_COOKIE)?.value;

  // 受保护路由：无会话 → 登录（携带 next 回跳）
  if (isProtected && !sessionCookie) {
    const loginUrl = new URL("/login", req.url);
    loginUrl.searchParams.set("next", pathname + req.nextUrl.search);
    return NextResponse.redirect(loginUrl);
  }

  // 已登录访问 /login 或 /signup → 回控制台
  if ((pathname === "/login" || pathname === "/signup") && sessionCookie) {
    return NextResponse.redirect(new URL("/console", req.url));
  }

  const res = NextResponse.next();

  // 滑动会话续期：活跃用户每次请求刷新 cookie 有效期（值不变，仅续期）
  if (sessionCookie) {
    res.cookies.set(SESSION_COOKIE, sessionCookie, {
      httpOnly: true,
      secure: process.env.NODE_ENV === "production",
      sameSite: "lax",
      path: "/",
      maxAge: SESSION_MAX_AGE_SECONDS,
    });
  }

  // 把 URL 上的 ?env= 透传给服务端组件（临时覆盖，优先级最高）
  const urlEnv = req.nextUrl.searchParams.get("env");
  if (urlEnv === "test" || urlEnv === "live") {
    res.headers.set("x-vlb-env", urlEnv);
  }

  return res;
}

export const config = {
  matcher: [
    "/((?!_next/static|_next/image|favicon.ico|.*\\.(?:svg|png|jpg|jpeg|gif|webp|ico)$).*)",
  ],
};
