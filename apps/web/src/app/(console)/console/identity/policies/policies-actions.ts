"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { requireAuth } from "@/lib/auth/rbac";
import type { Env } from "@/lib/env-shared";
import {
  deletePlanEntitlement,
  listPlanEntitlements,
  setPlanEntitlement,
  type EntitlementGrant,
} from "@/lib/api/operator";
import { entitlementInputSchema } from "@/lib/validate";

const POLICIES_PATH = "/console/identity/policies";

export interface EntitlementListState {
  ok: boolean;
  error?: string;
  grants: EntitlementGrant[];
}

export interface PoliciesActionState {
  ok: boolean;
  error?: string;
  grant?: EntitlementGrant;
}

export interface EntitlementQueryInput {
  providerId: string;
  env: Env;
  planCode: string;
}

const entitlementQuerySchema = z.object({
  providerId: z.string().trim().min(1, "缺少必要参数"),
  env: z.enum(["test", "live"]),
  planCode: z.string().trim().min(1, "缺少必要参数"),
});

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return "发生未知错误，请稍后重试。";
}

function parseEnv(value: FormDataEntryValue | null): Env | null {
  return value === "test" || value === "live" ? value : null;
}

function parseValue(raw: string): { value?: unknown; error?: string } {
  try {
    return { value: JSON.parse(raw) };
  } catch {
    return { error: "权益值必须是合法 JSON，例如 true、10 或 \"30d\"" };
  }
}

export async function listEntitlementsAction(
  input: EntitlementQueryInput,
): Promise<EntitlementListState> {
  await requireAuth();
  const parsed = entitlementQuerySchema.safeParse(input);
  if (!parsed.success) {
    return {
      ok: false,
      error: parsed.error.issues[0]?.message ?? "查询参数无效",
      grants: [],
    };
  }
  try {
    const grants = await listPlanEntitlements(
      parsed.data.providerId,
      parsed.data.env,
      parsed.data.planCode,
    );
    return { ok: true, grants };
  } catch (err) {
    return { ok: false, error: errorMessage(err), grants: [] };
  }
}

export async function setEntitlementAction(
  _prev: PoliciesActionState,
  formData: FormData,
): Promise<PoliciesActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const planCode = String(formData.get("plan_code") ?? "").trim();
  const key = String(formData.get("key") ?? "").trim();
  const valueType = String(formData.get("value_type") ?? "").trim();
  const parsedValue = parseValue(String(formData.get("value") ?? "").trim());
  if (!providerId || !env || !planCode || !key) {
    return { ok: false, error: "缺少必要参数" };
  }
  if (parsedValue.error) return { ok: false, error: parsedValue.error };

  const parsed = entitlementInputSchema.safeParse({
    key,
    value_type: valueType,
    value: parsedValue.value,
  });
  if (!parsed.success) {
    return { ok: false, error: parsed.error.issues[0]?.message ?? "输入无效" };
  }

  try {
    const grant = await setPlanEntitlement(
      providerId,
      env,
      planCode,
      key,
      parsed.data,
    );
    if (!grant) return { ok: false, error: "保存失败：API 未返回权益" };
    revalidatePath(POLICIES_PATH);
    return { ok: true, grant };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function deleteEntitlementAction(
  _prev: PoliciesActionState,
  formData: FormData,
): Promise<PoliciesActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const planCode = String(formData.get("plan_code") ?? "").trim();
  const key = String(formData.get("key") ?? "").trim();
  if (!providerId || !env || !planCode || !key) {
    return { ok: false, error: "缺少必要参数" };
  }

  try {
    await deletePlanEntitlement(providerId, env, planCode, key);
    revalidatePath(POLICIES_PATH);
    return { ok: true };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}
