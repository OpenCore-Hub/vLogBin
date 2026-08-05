import "server-only";

import { redirect } from "next/navigation";
import { SessionError, getSession, type Session } from "./session";

/** 必须已登录；未登录时携带 next 重定向到登录页。 */
export async function requireAuth(): Promise<Session> {
  const session = await getSession();
  if (!session) {
    redirect("/login");
  }
  return session;
}

/** 必须已登录且具备指定角色；否则 403（重定向到运营商台，避免信息泄露）。 */
export async function requireRole(role: string): Promise<Session> {
  const session = await requireAuth();
  if (!hasRole(session, role)) {
    redirect("/ops");
  }
  return session;
}

export function hasRole(session: Session, role: string): boolean {
  return session.roles.includes(role);
}

export function hasAnyRole(session: Session, roles: string[]): boolean {
  return roles.some((r) => session.roles.includes(r));
}

/** 从可能抛错的代码路径安全读取会话（未登录返回 null）。 */
export async function trySession(): Promise<Session | null> {
  try {
    return await getSession();
  } catch {
    return null;
  }
}

export { SessionError };
