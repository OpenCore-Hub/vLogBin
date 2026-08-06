import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  getCatalogVersionDetail,
  listCatalogPlans,
  listCatalogVersions,
  type CatalogVersion,
  type CatalogVersionDetail,
  type PlanCollection,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { CatalogClient } from "./catalog-client";

export const dynamic = "force-dynamic";

const EMPTY_PLANS: PlanCollection = { plans: [], metrics: [] };

export default async function CatalogPage({
  searchParams,
}: {
  searchParams: Promise<{ version?: string }>;
}) {
  const session = await requireAuth();
  const env = await resolveEnv(session);
  const params = await searchParams;

  const provider = await resolveWorkspaceProvider();

  let versions: CatalogVersion[] = [];
  let detail: CatalogVersionDetail | null = null;
  let currentPlans = EMPTY_PLANS;
  let loadError: string | null = null;
  if (provider) {
    try {
      versions = (await listCatalogVersions(provider.id))
        .filter((v) => v.environment_kind === env)
        .sort((a, b) => b.version - a.version);
      const requested =
        typeof params.version === "string" ? params.version : null;
      const selected =
        versions.find((v) => v.id === requested) ?? versions[0] ?? null;
      const [versionDetail, plans] = await Promise.all([
        selected
          ? getCatalogVersionDetail(provider.id, selected.id)
          : Promise.resolve(null),
        listCatalogPlans(provider.id, env).catch(() => EMPTY_PLANS),
      ]);
      detail = versionDetail;
      currentPlans = plans;
    } catch (err) {
      loadError = err instanceof Error ? err.message : "目录数据加载失败";
    }
  }

  return (
    <CatalogClient
      providerId={provider?.id ?? null}
      env={env}
      versions={versions}
      selectedVersionId={detail?.version.id ?? null}
      detail={detail}
      currentPlans={currentPlans}
      loadError={loadError}
    />
  );
}
