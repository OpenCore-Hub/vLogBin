"use server";

import { z } from "zod";
import { requireAuth } from "@/lib/auth/rbac";
import type { Env } from "@/lib/env-shared";
import {
  streamEvents,
  type PlatformEvent,
} from "@/lib/api/operator";

export interface EventStreamActionState {
  ok: boolean;
  error?: string;
  events: PlatformEvent[];
  next_cursor: string | null;
  has_more: boolean;
}

export interface EventStreamQueryInput {
  providerId: string;
  env: Env;
  cursor?: string | null;
  type?: string | null;
  aggregateType?: string | null;
}

const eventStreamQuerySchema = z.object({
  providerId: z.string().trim().min(1, "缺少必要参数"),
  env: z.enum(["test", "live"]),
  cursor: z.string().trim().optional().nullable(),
  type: z.string().trim().optional().nullable(),
  aggregateType: z.string().trim().optional().nullable(),
});

export async function queryEventStreamAction(
  input: EventStreamQueryInput,
): Promise<EventStreamActionState> {
  await requireAuth();
  const parsed = eventStreamQuerySchema.safeParse(input);
  if (!parsed.success) {
    return {
      ok: false,
      error: parsed.error.issues[0]?.message ?? "查询参数无效",
      events: [],
      next_cursor: null,
      has_more: false,
    };
  }
  const { providerId, env, cursor, type, aggregateType } = parsed.data;

  try {
    const stream = await streamEvents(providerId, env, {
      cursor: cursor || undefined,
      type: type || undefined,
      aggregate_type: aggregateType || undefined,
      limit: 50,
    });
    return { ok: true, events: stream.events, next_cursor: stream.next_cursor, has_more: stream.has_more };
  } catch (err) {
    return {
      ok: false,
      error: err instanceof Error ? err.message : "事件流加载失败",
      events: [],
      next_cursor: null,
      has_more: false,
    };
  }
}
