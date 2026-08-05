import type { Env } from "@/lib/env-shared";

export const QUERY_STALE_TIME = {
  standard: 30_000,
  audit: 60_000,
} as const;

export const consoleQueryKeys = {
  eventStream: (
    providerId: string | null,
    env: Env,
    type: string,
    aggregateType: string,
  ) => ["console", "events", providerId, env, type, aggregateType] as const,
  audit: (providerId: string | null, filters: Record<string, string>) =>
    ["console", "audit", providerId, filters] as const,
  policies: (providerId: string | null, env: Env, planCode: string) =>
    ["console", "policies", providerId, env, planCode] as const,
} as const;
