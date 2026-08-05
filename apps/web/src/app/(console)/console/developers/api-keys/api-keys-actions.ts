"use server";

import { revalidatePath } from "next/cache";
import { requireAuth } from "@/lib/auth/rbac";
import type { Env } from "@/lib/env-shared";
import {
  createCredential,
  revokeCredential,
  rotateCredential,
  type Credential,
} from "@/lib/api/operator";

const API_KEYS_PATH = "/console/developers/api-keys";

export interface ApiKeyActionState {
  ok: boolean;
  error?: string;
  credential?: Credential;
  apiKey?: string;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return "发生未知错误，请稍后重试。";
}

function parseEnv(value: FormDataEntryValue | null): Env | null {
  return value === "test" || value === "live" ? value : null;
}

function parseScopes(formData: FormData): string[] {
  const values = formData.getAll("scopes");
  return values
    .map((v) => String(v).trim())
    .filter((v) => ["read", "write", "credentials:manage", "audit:read", "support:approve", "scim:manage"].includes(v));
}

function expiryFromForm(value: string): string | undefined {
  const days = Number(value);
  if (!Number.isFinite(days) || days <= 0) return undefined;
  return new Date(Date.now() + days * 24 * 60 * 60 * 1000).toISOString();
}

export async function createCredentialAction(
  _prev: ApiKeyActionState,
  formData: FormData,
): Promise<ApiKeyActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const name = String(formData.get("name") ?? "").trim();
  const scopes = parseScopes(formData);
  if (!providerId || !env) return { ok: false, error: "缺少必要参数" };
  if (!name) return { ok: false, error: "请输入密钥名称" };
  if (scopes.length === 0) return { ok: false, error: "至少选择一个权限" };

  try {
    const result = await createCredential(providerId, env, {
      name,
      scopes,
      expires_at: expiryFromForm(String(formData.get("expires") ?? "")),
    });
    if (!result.api_key || !result.credential) {
      return { ok: false, error: "创建失败：API 未返回密钥" };
    }
    revalidatePath(API_KEYS_PATH);
    return { ok: true, credential: result.credential, apiKey: result.api_key };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function rotateCredentialAction(
  _prev: ApiKeyActionState,
  formData: FormData,
): Promise<ApiKeyActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const credentialId = String(formData.get("credential_id") ?? "").trim();
  if (!providerId || !env || !credentialId) return { ok: false, error: "缺少必要参数" };

  try {
    const result = await rotateCredential(providerId, env, credentialId);
    if (!result.api_key || !result.credential) {
      return { ok: false, error: "轮换失败：API 未返回新密钥" };
    }
    revalidatePath(API_KEYS_PATH);
    return { ok: true, credential: result.credential, apiKey: result.api_key };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function revokeCredentialAction(
  _prev: ApiKeyActionState,
  formData: FormData,
): Promise<ApiKeyActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const credentialId = String(formData.get("credential_id") ?? "").trim();
  if (!providerId || !credentialId) return { ok: false, error: "缺少必要参数" };

  try {
    const credential = await revokeCredential(providerId, credentialId);
    if (!credential) return { ok: false, error: "吊销失败：API 未返回凭证" };
    revalidatePath(API_KEYS_PATH);
    return { ok: true, credential };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}
