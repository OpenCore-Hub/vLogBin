import { requireRole } from "@/lib/auth/rbac";
import { listProviders, listRegions } from "@/lib/api/operator";
import type { Provider, Region } from "@/lib/api/operator";
import { OpsClient } from "./ops-client";

export const dynamic = "force-dynamic";

export default async function OpsPage() {
  await requireRole("operator");

  let providers: Provider[] = [];
  let regions: Region[] = [];
  let error: string | null = null;
  try {
    providers = await listProviders();
    // 区域列表仅用于把 home_region_id 翻译成 code，失败不影响主列表。
    regions = await listRegions().catch(() => [] as Region[]);
  } catch (err) {
    error = err instanceof Error ? err.message : "无法加载 Provider 列表，请稍后重试";
  }

  return (
    <OpsClient
      providers={providers}
      regions={regions}
      reviewRows={[]}
      awaitingReviews={[]}
      supportSessions={[]}
      cells={[]}
      failovers={[]}
      migrations={[]}
      error={error}
    />
  );
}
