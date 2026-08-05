import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listHostedAuthApps,
  listProviders,
  type HostedAuthConfig,
} from "@/lib/api/operator";
import { ApplicationsClient } from "./applications-client";

export const dynamic = "force-dynamic";

export default async function ApplicationsPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const providers = await listProviders().catch(() => []);
  // 注册即建 workspace（R11）：当前实现为 1:1 provider；后续多 workspace
  // 切换落地后改由会话 workspaceId 映射。
  const provider = providers[0] ?? null;

  let apps: HostedAuthConfig[] = [];
  let loadError: string | null = null;
  if (provider) {
    try {
      apps = await listHostedAuthApps(provider.id, env);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "应用列表加载失败";
    }
  }

  return (
    <ApplicationsClient
      providerId={provider?.id ?? null}
      env={env}
      apps={apps}
      loadError={loadError}
    />
  );
}
