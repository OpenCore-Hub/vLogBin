import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listCatalogPlans,
  type PlanCollection,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { PlansClient } from "./plans-client";

export const dynamic = "force-dynamic";

export default async function PlansPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const provider = await resolveWorkspaceProvider();

  let collection: PlanCollection = { plans: [], metrics: [] };
  let loadError: string | null = null;
  if (provider) {
    try {
      collection = await listCatalogPlans(provider.id, env);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "套餐列表加载失败";
    }
  }

  return (
    <PlansClient
      providerId={provider?.id ?? null}
      env={env}
      collection={collection}
      loadError={loadError}
    />
  );
}
