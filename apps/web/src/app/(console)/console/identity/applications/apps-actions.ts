"use server";

import { revalidatePath } from "next/cache";
import { requireAuth } from "@/lib/auth/rbac";
import type { Env } from "@/lib/env-shared";
import {
  createHostedAuthApp,
  disableHostedAuth,
  rotateHostedAuthSecret,
  updateHostedAuthRedirectURIs,
  type HostedAuthConfig,
} from "@/lib/api/operator";
import { createHostedAuthAppSchema } from "@/lib/validate";

export interface AppActionState {
  ok: boolean;
  error?: string;
  app?: HostedAuthConfig;
  /** 明文客户端密钥，仅创建/轮换成功后返回一次（R17）。 */
  secret?: string;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return "发生未知错误，请稍后重试。";
}

function parseEnv(value: FormDataEntryValue | null): Env | null {
  if (value === "test" || value === "live") return value;
  return null;
}

const APPLICATIONS_PATH = "/console/identity/applications";

/** 创建 OIDC 应用（细粒度 RBAC：登录用户 + API 侧 operator 校验）。 */
export async function createAppAction(
  _prev: AppActionState,
  formData: FormData,
): Promise<AppActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  if (!providerId || !env) return { ok: false, error: "缺少必要参数" };

  const parsed = createHostedAuthAppSchema.safeParse({
    name: formData.get("name"),
    redirect_uris: String(formData.get("redirect_uris") ?? "")
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean),
  });
  if (!parsed.success) {
    return { ok: false, error: parsed.error.issues[0]?.message ?? "输入无效" };
  }

  try {
    const app = await createHostedAuthApp(providerId, env, parsed.data);
    if (!app) return { ok: false, error: "创建失败：API 未返回应用信息" };
    revalidatePath(APPLICATIONS_PATH);
    return { ok: true, app, secret: app.client_secret };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 轮换客户端密钥（明文只显示一次）。 */
export async function rotateSecretAction(
  _prev: AppActionState,
  formData: FormData,
): Promise<AppActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  if (!providerId || !env) return { ok: false, error: "缺少必要参数" };

  try {
    const app = await rotateHostedAuthSecret(providerId, env);
    if (!app) return { ok: false, error: "轮换失败：API 未返回应用信息" };
    revalidatePath(APPLICATIONS_PATH);
    return { ok: true, app, secret: app.client_secret };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 更新回调地址（不更换客户端密钥）。 */
export async function updateRedirectURIsAction(
  _prev: AppActionState,
  formData: FormData,
): Promise<AppActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  if (!providerId || !env) return { ok: false, error: "缺少必要参数" };

  const redirectUris = String(formData.get("redirect_uris") ?? "")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
  const parsed = createHostedAuthAppSchema.safeParse({
    name: "保留",
    redirect_uris: redirectUris,
  });
  if (!parsed.success) {
    return { ok: false, error: parsed.error.issues[0]?.message ?? "回调地址无效" };
  }

  try {
    const app = await updateHostedAuthRedirectURIs(
      providerId,
      env,
      parsed.data.redirect_uris,
    );
    if (!app) return { ok: false, error: "更新失败：API 未返回应用信息" };
    revalidatePath(APPLICATIONS_PATH);
    return { ok: true, app };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 删除 OIDC 应用（ZITADEL 项目一并移除）。 */
export async function disableAppAction(
  _prev: AppActionState,
  formData: FormData,
): Promise<AppActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  if (!providerId || !env) return { ok: false, error: "缺少必要参数" };

  try {
    await disableHostedAuth(providerId, env);
    revalidatePath(APPLICATIONS_PATH);
    return { ok: true };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}
