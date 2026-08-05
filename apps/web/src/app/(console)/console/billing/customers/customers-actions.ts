"use server";

import { revalidatePath } from "next/cache";
import { requireAuth } from "@/lib/auth/rbac";
import type { Env } from "@/lib/env-shared";
import {
  createCustomer,
  type Customer,
  type CustomerCreateInput,
} from "@/lib/api/operator";
import { createCustomerSchema } from "@/lib/api/schemas";

export interface CustomerActionState {
  ok: boolean;
  error?: string;
  customer?: Customer;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return "发生未知错误，请稍后重试。";
}

function parseEnv(value: FormDataEntryValue | null): Env | null {
  if (value === "test" || value === "live") return value;
  return null;
}

const CUSTOMERS_PATH = "/console/billing/customers";

/** 创建客户（写入当前环境；external_id 在 provider 环境内唯一）。 */
export async function createCustomerAction(
  _prev: CustomerActionState,
  formData: FormData,
): Promise<CustomerActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  if (!providerId || !env) return { ok: false, error: "缺少必要参数" };

  const parsed = createCustomerSchema.safeParse({
    external_id: formData.get("external_id"),
    account_type: formData.get("account_type"),
    display_name: formData.get("display_name"),
  });
  if (!parsed.success) {
    return { ok: false, error: parsed.error.issues[0]?.message ?? "输入无效" };
  }
  const input: CustomerCreateInput = parsed.data;

  try {
    const customer = await createCustomer(providerId, env, input);
    if (!customer) return { ok: false, error: "创建失败：API 未返回客户信息" };
    revalidatePath(CUSTOMERS_PATH);
    return { ok: true, customer };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}
