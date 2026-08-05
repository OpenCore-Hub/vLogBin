"use server";

import { revalidatePath } from "next/cache";
import { requireAuth } from "@/lib/auth/rbac";
import type { Env } from "@/lib/env-shared";
import {
  deleteCustomDomain,
  deleteNotificationConfig,
  registerCustomDomain,
  revokeCustomDomain,
  setNotificationConfig,
  updateWorkspace,
  verifyCustomDomain,
  type CustomDomain,
  type NotificationConfig,
  type Workspace,
} from "@/lib/api/operator";

const SETTINGS_PATH = "/console/settings";

export interface SettingsActionState {
  ok: boolean;
  error?: string;
  workspace?: Workspace;
  domain?: CustomDomain;
  notification?: NotificationConfig;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return "发生未知错误，请稍后重试。";
}

function parseEnv(value: FormDataEntryValue | null): Env | null {
  return value === "test" || value === "live" ? value : null;
}

export async function updateWorkspaceAction(
  _prev: SettingsActionState,
  formData: FormData,
): Promise<SettingsActionState> {
  await requireAuth();
  const workspaceId = String(formData.get("workspace_id") ?? "").trim();
  const name = String(formData.get("name") ?? "").trim();
  const slug = String(formData.get("slug") ?? "").trim();
  if (!workspaceId) return { ok: false, error: "缺少必要参数" };
  if (!name || !slug) return { ok: false, error: "名称与 slug 不能为空" };

  try {
    const workspace = await updateWorkspace(workspaceId, { name, slug });
    if (!workspace) return { ok: false, error: "保存失败：API 未返回 workspace" };
    revalidatePath(SETTINGS_PATH);
    return { ok: true, workspace };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function registerDomainAction(
  _prev: SettingsActionState,
  formData: FormData,
): Promise<SettingsActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const domain = String(formData.get("domain") ?? "").trim();
  if (!providerId || !env) return { ok: false, error: "缺少必要参数" };
  if (!domain) return { ok: false, error: "请输入域名" };

  try {
    const created = await registerCustomDomain(providerId, env, domain);
    if (!created) return { ok: false, error: "注册失败：API 未返回域名" };
    revalidatePath(SETTINGS_PATH);
    return { ok: true, domain: created };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function verifyDomainAction(
  _prev: SettingsActionState,
  formData: FormData,
): Promise<SettingsActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const domainId = String(formData.get("domain_id") ?? "").trim();
  if (!providerId || !env || !domainId) return { ok: false, error: "缺少必要参数" };
  try {
    const domain = await verifyCustomDomain(providerId, env, domainId);
    if (!domain) return { ok: false, error: "验证失败：API 未返回域名" };
    revalidatePath(SETTINGS_PATH);
    return { ok: true, domain };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function revokeDomainAction(
  _prev: SettingsActionState,
  formData: FormData,
): Promise<SettingsActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const domainId = String(formData.get("domain_id") ?? "").trim();
  if (!providerId || !env || !domainId) return { ok: false, error: "缺少必要参数" };
  try {
    const domain = await revokeCustomDomain(providerId, env, domainId);
    if (!domain) return { ok: false, error: "吊销失败：API 未返回域名" };
    revalidatePath(SETTINGS_PATH);
    return { ok: true, domain };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function deleteDomainAction(
  _prev: SettingsActionState,
  formData: FormData,
): Promise<SettingsActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const domainId = String(formData.get("domain_id") ?? "").trim();
  if (!providerId || !env || !domainId) return { ok: false, error: "缺少必要参数" };
  try {
    await deleteCustomDomain(providerId, env, domainId);
    revalidatePath(SETTINGS_PATH);
    return { ok: true };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function setNotificationAction(
  _prev: SettingsActionState,
  formData: FormData,
): Promise<SettingsActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const channel = String(formData.get("channel") ?? "");
  const providerType = String(formData.get("provider_type") ?? "").trim();
  const fromAddress = String(formData.get("from_address") ?? "").trim();
  const rawConfig = String(formData.get("config") ?? "").trim();
  if (!providerId || !env) return { ok: false, error: "缺少必要参数" };
  if (channel !== "email" && channel !== "sms") return { ok: false, error: "渠道无效" };
  if (!providerType || !fromAddress) return { ok: false, error: "请填写渠道配置" };
  let config: Record<string, unknown>;
  try {
    config = JSON.parse(rawConfig) as Record<string, unknown>;
  } catch {
    return { ok: false, error: "配置必须是合法 JSON" };
  }
  if (!config || typeof config !== "object" || Array.isArray(config)) {
    return { ok: false, error: "配置必须是 JSON 对象" };
  }
  try {
    const notification = await setNotificationConfig(providerId, env, {
      channel,
      provider_type: providerType,
      config,
      from_address: fromAddress,
      enabled: formData.get("enabled") === "on",
    });
    if (!notification) return { ok: false, error: "保存失败：API 未返回配置" };
    revalidatePath(SETTINGS_PATH);
    return { ok: true, notification };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function deleteNotificationAction(
  _prev: SettingsActionState,
  formData: FormData,
): Promise<SettingsActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const channel = String(formData.get("channel") ?? "");
  if (!providerId || !env || (channel !== "email" && channel !== "sms")) {
    return { ok: false, error: "缺少必要参数" };
  }
  try {
    await deleteNotificationConfig(providerId, env, channel);
    revalidatePath(SETTINGS_PATH);
    return { ok: true };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}
