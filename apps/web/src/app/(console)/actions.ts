"use server";

import { requireAuth } from "@/lib/auth/rbac";
import { rememberEnv } from "@/lib/env";
import type { Env } from "@/lib/env-shared";

/** 切换环境并持久化到 cookie（URL 显式 ?env= 由客户端组件追加）。 */
export async function switchEnv(env: string): Promise<void> {
  await requireAuth();
  if (env !== "test" && env !== "live") {
    throw new Error("invalid environment");
  }
  await rememberEnv(env as Env);
}
