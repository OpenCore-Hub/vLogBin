import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listProviders,
  streamEvents,
  type PlatformEvent,
} from "@/lib/api/operator";
import { EventsClient } from "./events-client";

export const dynamic = "force-dynamic";

export default async function EventsPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const providers = await listProviders().catch(() => []);
  const provider = providers[0] ?? null;

  let events: PlatformEvent[] = [];
  let nextCursor: string | null = null;
  let hasMore = false;
  let loadError: string | null = null;
  if (provider) {
    try {
      const stream = await streamEvents(provider.id, env, { limit: 50 });
      events = stream.events;
      nextCursor = stream.next_cursor;
      hasMore = stream.has_more;
    } catch (err) {
      loadError = err instanceof Error ? err.message : "事件流加载失败";
    }
  }

  return (
    <EventsClient
      providerId={provider?.id ?? null}
      env={env}
      initial={{ events, next_cursor: nextCursor, has_more: hasMore }}
      loadError={loadError}
    />
  );
}
