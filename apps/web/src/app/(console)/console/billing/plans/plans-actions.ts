"use server";

import { revalidatePath } from "next/cache";
import { requireAuth } from "@/lib/auth/rbac";
import type { Env } from "@/lib/env-shared";
import {
  createCatalogPlan,
  deleteCatalogPlan,
  updateCatalogPlan,
  type PlanDetail,
  type PlanInput,
} from "@/lib/api/operator";
import { planInputSchema } from "@/lib/validate";

export interface PlanActionState {
  ok: boolean;
  error?: string;
  detail?: PlanDetail;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return "发生未知错误，请稍后重试。";
}

function parseEnv(value: FormDataEntryValue | null): Env | null {
  if (value === "test" || value === "live") return value;
  return null;
}

const PLANS_PATH = "/console/billing/plans";

function parsePlanPayload(formData: FormData): {
  payload: PlanInput;
  error?: string;
} {
  const raw = String(formData.get("payload") ?? "");
  let payload: unknown;
  try {
    payload = JSON.parse(raw);
  } catch {
    return { payload: {} as PlanInput, error: "表单数据无效，请重试。" };
  }
  const parsed = planInputSchema.safeParse(payload);
  if (!parsed.success) {
    return {
      payload: {} as PlanInput,
      error: parsed.error.issues[0]?.message ?? "输入无效",
    };
  }
  return { payload: parsed.data };
}

/** 创建套餐（写入当前 draft 目录版本）。 */
export async function createPlanAction(
  _prev: PlanActionState,
  formData: FormData,
): Promise<PlanActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  if (!providerId || !env) return { ok: false, error: "缺少必要参数" };

  const { payload, error } = parsePlanPayload(formData);
  if (error) return { ok: false, error };

  try {
    const detail = await createCatalogPlan(providerId, env, payload);
    if (!detail) return { ok: false, error: "创建失败：API 未返回套餐信息" };
    revalidatePath(PLANS_PATH);
    return { ok: true, detail };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 更新套餐（plan code 不可变，body code 必须与路径一致）。 */
export async function updatePlanAction(
  _prev: PlanActionState,
  formData: FormData,
): Promise<PlanActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const code = String(formData.get("code") ?? "").trim();
  if (!providerId || !env || !code) return { ok: false, error: "缺少必要参数" };

  const { payload, error } = parsePlanPayload(formData);
  if (error) return { ok: false, error };
  if (payload.code !== code) {
    return { ok: false, error: "套餐代码不可修改" };
  }

  try {
    const detail = await updateCatalogPlan(providerId, env, code, payload);
    if (!detail) return { ok: false, error: "更新失败：API 未返回套餐信息" };
    revalidatePath(PLANS_PATH);
    return { ok: true, detail };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 删除套餐（仅 draft 版本；已发布内容不受影响）。 */
export async function deletePlanAction(
  _prev: PlanActionState,
  formData: FormData,
): Promise<PlanActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const code = String(formData.get("code") ?? "").trim();
  if (!providerId || !env || !code) return { ok: false, error: "缺少必要参数" };

  try {
    await deleteCatalogPlan(providerId, env, code);
    revalidatePath(PLANS_PATH);
    return { ok: true };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}
