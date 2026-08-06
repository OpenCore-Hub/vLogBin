import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listSubscriptions,
  type Subscription,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { SubscriptionsClient } from "./subscriptions-client";

export const dynamic = "force-dynamic";

export default async function SubscriptionsPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const provider = await resolveWorkspaceProvider();

  let subscriptions: Subscription[] = [];
  let loadError: string | null = null;
  if (provider) {
    try {
      subscriptions = (await listSubscriptions(provider.id)).filter(
        (s) => s.environment_kind === env,
      );
    } catch (err) {
      loadError = err instanceof Error ? err.message : "订阅列表加载失败";
    }
  }

  return (
    <SubscriptionsClient
      providerId={provider?.id ?? null}
      env={env}
      subscriptions={subscriptions}
      loadError={loadError}
    />
  );
}
