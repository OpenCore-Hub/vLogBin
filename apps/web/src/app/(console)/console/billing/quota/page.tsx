import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listQuotaLimits,
  listQuotaReservations,
  listSubscriptions,
  type QuotaLimitUsage,
  type QuotaReservation,
  type Subscription,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { QuotaClient } from "./quota-client";

export const dynamic = "force-dynamic";

export default async function QuotaPage({
  searchParams,
}: {
  searchParams: Promise<{ subscription?: string }>;
}) {
  const session = await requireAuth();
  const env = await resolveEnv(session);
  const params = await searchParams;

  const provider = await resolveWorkspaceProvider();
  let subscriptions: Subscription[] = [];
  if (provider) {
    try {
      subscriptions = (await listSubscriptions(provider.id)).filter(
        (s) => s.environment_kind === env,
      );
    } catch {
      subscriptions = [];
    }
  }

  const selected =
    subscriptions.find((s) => s.id === params.subscription) ??
    subscriptions[0] ??
    null;

  let quotaLimits: QuotaLimitUsage[] = [];
  let reservations: QuotaReservation[] = [];
  let loadError: string | null = null;
  if (provider && selected) {
    try {
      [quotaLimits, reservations] = await Promise.all([
        listQuotaLimits(provider.id, selected.id, env),
        listQuotaReservations(provider.id, selected.id, env),
      ]);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "额度数据加载失败";
    }
  }

  return (
    <QuotaClient
      providerId={provider?.id ?? null}
      env={env}
      subscriptions={subscriptions}
      selectedSubscription={selected}
      quotaLimits={quotaLimits}
      reservations={reservations}
      loadError={loadError}
    />
  );
}
