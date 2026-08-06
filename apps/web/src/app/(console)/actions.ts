"use server";

import { revalidatePath } from "next/cache";
import { cookies } from "next/headers";
import { requireAuth } from "@/lib/auth/rbac";
import { rememberEnv } from "@/lib/env";
import {
  ONBOARDING_DISMISS_COOKIE,
  WORKSPACE_COOKIE,
  type Env,
} from "@/lib/env-shared";

/** 切换环境并持久化到 cookie（URL 显式 ?env= 由客户端组件追加）。 */
export async function switchEnv(env: string): Promise<void> {
  await requireAuth();
  if (env !== "test" && env !== "live") {
    throw new Error("invalid environment");
  }
  await rememberEnv(env as Env);
}

/** 跳过 First-Run 引导并持久化到 cookie（R18：始终可见、不强制）。 */
export async function dismissOnboarding(): Promise<void> {
  await requireAuth();
  const jar = await cookies();
  jar.set(ONBOARDING_DISMISS_COOKIE, "1", {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 60 * 60 * 24 * 365,
  });
  revalidatePath("/console");
}

/** 恢复 First-Run 引导（清除跳过标记）。 */
export async function restoreOnboarding(): Promise<void> {
  await requireAuth();
  const jar = await cookies();
  jar.delete(ONBOARDING_DISMISS_COOKIE);
  revalidatePath("/console");
}

/** 切换当前 workspace（workspace_id == provider_id，M4）。 */
export async function switchWorkspace(workspaceId: string): Promise<void> {
  await requireAuth();
  const id = workspaceId.trim();
  if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(id)) {
    throw new Error("invalid workspace id");
  }
  const jar = await cookies();
  jar.set(WORKSPACE_COOKIE, id, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: 60 * 60 * 24 * 365,
  });
  revalidatePath("/console", "layout");
}
