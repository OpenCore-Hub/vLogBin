import "server-only";

import { cookies } from "next/headers";
import { PORTAL_COOKIE } from "../env-shared";

/** 读取客户门户 token（独立客户会话，与 Console 会话隔离）。 */
export async function getPortalToken(): Promise<string | null> {
  const jar = await cookies();
  return jar.get(PORTAL_COOKIE)?.value ?? null;
}

export async function requirePortalToken(): Promise<string> {
  const token = await getPortalToken();
  if (!token) throw new Error("未登录客户门户");
  return token;
}

/** 写入客户门户 token；仅在 API 验证成功后调用。 */
export async function setPortalToken(token: string): Promise<void> {
  const jar = await cookies();
  jar.set(PORTAL_COOKIE, token, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/portal",
    maxAge: 60 * 60 * 24,
  });
}

export async function clearPortalToken(): Promise<void> {
  const jar = await cookies();
  jar.delete(PORTAL_COOKIE);
}
