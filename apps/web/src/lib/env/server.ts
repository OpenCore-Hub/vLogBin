import "server-only";

import { cookies, headers } from "next/headers";
import { ENV_COOKIE } from "./shared";
import type { Env, Session } from "../auth/session";

export { ENV_COOKIE };
export type { Env } from "./shared";

export const ENV_LABELS: Record<Env, { label: string; description: string }> = {
  test: { label: "Test", description: "隔离的测试环境，可自由变更" },
  live: { label: "Live", description: "生产环境，变更需谨慎" },
};

/**
 * 环境解析优先级（设计基线 §6.2.3）：
 * 1. URL 显式 ?env=（临时覆盖，不写 cookie）
 * 2. cookie vlb_env（持久化记忆）
 * 3. 会话默认（默认 test）
 */
export async function resolveEnv(_session?: Session | null): Promise<Env> {
  void _session; // 保留签名以兼容调用方；当前默认即 test
  const h = await headers();
  const urlEnv = normalizeEnv(h.get("x-vlb-env"));
  if (urlEnv) return urlEnv;
  const jar = await cookies();
  const cookieEnv = normalizeEnv(jar.get(ENV_COOKIE)?.value);
  if (cookieEnv) return cookieEnv;
  return "test";
}

function normalizeEnv(v: string | null | undefined): Env | null {
  if (v === "test" || v === "live") return v;
  return null;
}

/** 记忆环境到 cookie（server action / route handler 中调用）。 */
export async function rememberEnv(env: Env): Promise<void> {
  const jar = await cookies();
  jar.set(ENV_COOKIE, env, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 60 * 60 * 24 * 365,
  });
}
