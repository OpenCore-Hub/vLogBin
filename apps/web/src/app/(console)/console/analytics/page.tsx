import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  getAnalyticsDashboard,
  type AnalyticsDashboard,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { AnalyticsClient } from "./analytics-client";

export const dynamic = "force-dynamic";

const EMPTY: AnalyticsDashboard = {
  revenue: [],
  mau: [],
  conversion: [],
  churn: [],
  anomalies: [],
  generated_at: "",
};

export default async function AnalyticsPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const provider = await resolveWorkspaceProvider();

  let dashboard = EMPTY;
  let loadError: string | null = null;
  if (provider) {
    try {
      dashboard = await getAnalyticsDashboard(provider.id, env);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "分析数据加载失败";
    }
  }

  return (
    <AnalyticsClient
      providerId={provider?.id ?? null}
      env={env}
      dashboard={dashboard}
      loadError={loadError}
    />
  );
}
