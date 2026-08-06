import { requireAuth } from "@/lib/auth/rbac";
import {
  getAuditChain,
  getAuditStats,
  queryAuditEvents,
  type AuditChainState,
  type AuditEvent,
  type AuditStats,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { AuditClient } from "./audit-client";

export const dynamic = "force-dynamic";

function isoDaysAgo(days: number): string {
  return new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString();
}

export default async function AuditPage() {
  await requireAuth();
  const provider = await resolveWorkspaceProvider();

  let events: AuditEvent[] = [];
  let nextCursor: number | null = null;
  let stats: AuditStats | null = null;
  let chain: AuditChainState | null = null;
  let loadError: string | null = null;
  const from = isoDaysAgo(7);
  const to = new Date().toISOString();

  if (provider) {
    try {
      const [page, auditStats, auditChain] = await Promise.all([
        queryAuditEvents(provider.id, { limit: 100 }),
        getAuditStats(provider.id, from, to, "day").catch(() => null),
        getAuditChain().catch(() => null),
      ]);
      events = page.events;
      nextCursor = page.next_cursor;
      stats = auditStats;
      chain = auditChain;
    } catch (err) {
      loadError = err instanceof Error ? err.message : "审计日志加载失败";
    }
  } else {
    chain = await getAuditChain().catch(() => null);
  }

  return (
    <AuditClient
      providerId={provider?.id ?? null}
      initialEvents={events}
      nextCursor={nextCursor}
      stats={stats}
      chain={chain}
      defaultFrom={from}
      defaultTo={to}
      loadError={loadError}
    />
  );
}
