import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listCatalogPlans,
  listPlanEntitlements,
  type EntitlementGrant,
  type PlanDetail,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { PoliciesClient } from "./policies-client";

export const dynamic = "force-dynamic";

export default async function PoliciesPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const provider = await resolveWorkspaceProvider();

  let plans: PlanDetail[] = [];
  let initialGrants: EntitlementGrant[] = [];
  let loadError: string | null = null;
  if (provider) {
    try {
      plans = (await listCatalogPlans(provider.id, env)).plans;
      const first = plans[0];
      if (first) {
        initialGrants = await listPlanEntitlements(
          provider.id,
          env,
          first.plan.code,
        );
      }
    } catch (err) {
      loadError = err instanceof Error ? err.message : "策略列表加载失败";
    }
  }

  return (
    <PoliciesClient
      providerId={provider?.id ?? null}
      env={env}
      plans={plans}
      initialPlanCode={plans[0]?.plan.code ?? null}
      initialEntitlements={initialGrants}
      loadError={loadError}
    />
  );
}
