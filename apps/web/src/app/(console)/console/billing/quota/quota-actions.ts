"use server";

import { revalidatePath } from "next/cache";
import { requireAuth } from "@/lib/auth/rbac";
import type { Env } from "@/lib/env-shared";
import {
  deleteQuotaLimit,
  setQuotaLimit,
} from "@/lib/api/operator";

export interface QuotaActionState {
  ok: boolean;
  error?: string;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return "发生未知错误，请稍后重试。";
}

function parseEnv(value: FormDataEntryValue | null): Env | null {
  if (value === "test" || value === "live") return value;
  return null;
}

const QUOTA_PATH = "/console/billing/quota";

function quotaContext(formData: FormData): {
  providerId: string;
  subscriptionId: string;
  env: Env;
  error?: string;
} {
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const subscriptionId = String(formData.get("subscription_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  if (!providerId || !subscriptionId || !env) {
    return {
      providerId: "",
      subscriptionId: "",
      env: "test",
      error: "缺少必要参数",
    };
  }
  return { providerId, subscriptionId, env };
}

/** 创建或更新硬额度上限。 */
export async function setQuotaLimitAction(
  _prev: QuotaActionState,
  formData: FormData,
): Promise<QuotaActionState> {
  await requireAuth();
  const ctx = quotaContext(formData);
  if (ctx.error) return { ok: false, error: ctx.error };

  const key = String(formData.get("quota_key") ?? "").trim();
  const periodType = String(formData.get("period_type") ?? "");
  const limitValue = Number(formData.get("limit_value"));
  if (!key) return { ok: false, error: "额度键不能为空" };
  if (!["daily", "monthly", "total"].includes(periodType)) {
    return { ok: false, error: "周期必须是 daily / monthly / total" };
  }
  if (!Number.isInteger(limitValue) || limitValue < 0) {
    return { ok: false, error: "额度必须是大于等于 0 的整数" };
  }

  try {
    await setQuotaLimit(ctx.providerId, ctx.subscriptionId, ctx.env, key, {
      limit_value: limitValue,
      period_type: periodType,
    });
    revalidatePath(QUOTA_PATH);
    return { ok: true };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 删除硬额度上限。 */
export async function deleteQuotaLimitAction(
  _prev: QuotaActionState,
  formData: FormData,
): Promise<QuotaActionState> {
  await requireAuth();
  const ctx = quotaContext(formData);
  if (ctx.error) return { ok: false, error: ctx.error };

  const key = String(formData.get("quota_key") ?? "").trim();
  if (!key) return { ok: false, error: "缺少必要参数" };

  try {
    await deleteQuotaLimit(ctx.providerId, ctx.subscriptionId, ctx.env, key);
    revalidatePath(QUOTA_PATH);
    return { ok: true };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}
