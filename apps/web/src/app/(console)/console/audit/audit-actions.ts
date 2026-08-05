"use server";

import { requireAuth } from "@/lib/auth/rbac";
import {
  queryAuditEvents,
  verifyAuditChain,
  type AuditChainVerifyResult,
  type AuditEvent,
} from "@/lib/api/operator";

export interface AuditPageActionState {
  ok: boolean;
  error?: string;
  events: AuditEvent[];
  next_cursor: number | null;
  verify?: AuditChainVerifyResult;
}

const empty: AuditPageActionState = { ok: false, events: [], next_cursor: null };

function toIso(value: string, endOfDay = false): string | undefined {
  if (!value) return undefined;
  if (value.includes("T")) {
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString();
  }
  const parsed = new Date(`${value}T${endOfDay ? "23:59:59" : "00:00:00"}`);
  return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString();
}

export async function fetchAuditPageAction(
  _prev: AuditPageActionState,
  formData: FormData,
): Promise<AuditPageActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  if (!providerId) return { ...empty, error: "缺少必要参数" };
  try {
    const cursorRaw = String(formData.get("cursor") ?? "").trim();
    const result = await queryAuditEvents(providerId, {
      cursor: cursorRaw ? Number(cursorRaw) : undefined,
      limit: 100,
      action: String(formData.get("action") ?? "").trim() || undefined,
      actor_type: String(formData.get("actor_type") ?? "").trim() || undefined,
      target_type: String(formData.get("target_type") ?? "").trim() || undefined,
      from: toIso(String(formData.get("from") ?? "").trim()),
      to: toIso(String(formData.get("to") ?? "").trim(), true),
    });
    return { ok: true, events: result.events, next_cursor: result.next_cursor };
  } catch (err) {
    return {
      ...empty,
      error: err instanceof Error ? err.message : "审计日志加载失败",
    };
  }
}

export async function verifyAuditChainAction(): Promise<AuditPageActionState> {
  await requireAuth();
  try {
    const verify = await verifyAuditChain();
    return { ok: true, events: [], next_cursor: null, verify: verify ?? undefined };
  } catch (err) {
    return {
      ...empty,
      error: err instanceof Error ? err.message : "哈希链验证失败",
    };
  }
}
