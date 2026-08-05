"use server";

import { z } from "zod";
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

export interface AuditPageQueryInput {
  providerId: string;
  cursor?: number | null;
  action?: string | null;
  actorType?: string | null;
  targetType?: string | null;
  from?: string | null;
  to?: string | null;
}

const auditPageQuerySchema = z.object({
  providerId: z.string().trim().min(1, "缺少必要参数"),
  cursor: z.number().int().nonnegative().optional().nullable(),
  action: z.string().trim().optional().nullable(),
  actorType: z.string().trim().optional().nullable(),
  targetType: z.string().trim().optional().nullable(),
  from: z.string().trim().optional().nullable(),
  to: z.string().trim().optional().nullable(),
});

function toIso(value: string | null | undefined, endOfDay = false): string | undefined {
  if (!value) return undefined;
  if (value.includes("T")) {
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString();
  }
  const parsed = new Date(`${value}T${endOfDay ? "23:59:59" : "00:00:00"}`);
  return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString();
}

export async function queryAuditPageAction(
  input: AuditPageQueryInput,
): Promise<AuditPageActionState> {
  await requireAuth();
  const parsed = auditPageQuerySchema.safeParse(input);
  if (!parsed.success) {
    return {
      ...empty,
      error: parsed.error.issues[0]?.message ?? "查询参数无效",
    };
  }
  try {
    const result = await queryAuditEvents(parsed.data.providerId, {
      cursor: parsed.data.cursor ?? undefined,
      limit: 100,
      action: parsed.data.action || undefined,
      actor_type: parsed.data.actorType || undefined,
      target_type: parsed.data.targetType || undefined,
      from: toIso(parsed.data.from),
      to: toIso(parsed.data.to, true),
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
