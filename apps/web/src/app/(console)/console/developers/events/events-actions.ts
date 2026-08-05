"use server";

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

function parseEnv(value: FormDataEntryValue | null): Env | null {
  return value === "test" || value === "live" ? value : null;
}

export async function fetchEventsAction(
  _prev: EventStreamActionState,
  formData: FormData,
): Promise<EventStreamActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  if (!providerId || !env) return { ok: false, error: "缺少必要参数", events: [], next_cursor: null, has_more: false };

  const cursor = String(formData.get("cursor") ?? "").trim() || undefined;
  const type = String(formData.get("type") ?? "").trim() || undefined;
  const aggregateType = String(formData.get("aggregate_type") ?? "").trim() || undefined;

  try {
    const stream = await streamEvents(providerId, env, {
      cursor,
      type,
      aggregate_type: aggregateType,
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
