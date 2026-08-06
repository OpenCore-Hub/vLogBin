import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listUsageEvents,
  type UsageEvent,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { UsageClient } from "./usage-client";

export const dynamic = "force-dynamic";

export default async function UsagePage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const provider = await resolveWorkspaceProvider();

  let events: UsageEvent[] = [];
  let loadError: string | null = null;
  if (provider) {
    try {
      events = await listUsageEvents(provider.id);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "用量数据加载失败";
    }
  }

  return (
    <UsageClient
      providerId={provider?.id ?? null}
      env={env}
      events={events}
      loadError={loadError}
    />
  );
}
