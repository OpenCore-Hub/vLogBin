import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listCatalogPlans,
  listProviders,
  type PlanCollection,
} from "@/lib/api/operator";
import { PlansClient } from "./plans-client";

export const dynamic = "force-dynamic";

export default async function PlansPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const providers = await listProviders().catch(() => []);
  // 注册即建 workspace（R11）：当前实现为 1:1 provider；后续多 workspace
  // 切换落地后改由会话 workspaceId 映射。
  const provider = providers[0] ?? null;

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
