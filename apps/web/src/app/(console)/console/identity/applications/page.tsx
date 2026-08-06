import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listHostedAuthApps,
  type HostedAuthConfig,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { ApplicationsClient } from "./applications-client";

export const dynamic = "force-dynamic";

export default async function ApplicationsPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const provider = await resolveWorkspaceProvider();

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
