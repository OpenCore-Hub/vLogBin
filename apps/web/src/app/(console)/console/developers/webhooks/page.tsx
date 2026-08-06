import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listWebhookDeliveries,
  listWebhooks,
  type WebhookDelivery,
  type WebhookEndpoint,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { WebhooksClient } from "./webhooks-client";

export const dynamic = "force-dynamic";

export default async function WebhooksPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const provider = await resolveWorkspaceProvider();

  let endpoints: WebhookEndpoint[] = [];
  let deliveries: WebhookDelivery[] = [];
  let loadError: string | null = null;
  if (provider) {
    try {
      [endpoints, deliveries] = await Promise.all([
        listWebhooks(provider.id, env),
        listWebhookDeliveries(provider.id, env),
      ]);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "Webhook 数据加载失败";
    }
  }

  return (
    <WebhooksClient
      providerId={provider?.id ?? null}
      env={env}
      endpoints={endpoints}
      deliveries={deliveries}
      loadError={loadError}
    />
  );
}
