import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  getWorkspace,
  listCustomDomains,
  listMyWorkspaces,
  listNotificationConfigs,
  listProviders,
  type CustomDomain,
  type NotificationConfig,
  type Workspace,
} from "@/lib/api/operator";
import { SettingsClient } from "./settings-client";

export const dynamic = "force-dynamic";

export default async function SettingsPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const [providers, workspaces] = await Promise.all([
    listProviders().catch(() => []),
    listMyWorkspaces().catch(() => []),
  ]);
  const workspace = workspaces[0] ?? null;
  const provider = providers[0] ?? null;
  const providerId = workspace?.id ?? provider?.id ?? null;

  let resolvedWorkspace: Workspace | null = workspace;
  let domains: CustomDomain[] = [];
  let notifications: NotificationConfig[] = [];
  let loadError: string | null = null;
  if (providerId) {
    try {
      const [ws, customDomains, configs] = await Promise.all([
        getWorkspace(providerId).catch(() => null),
        listCustomDomains(providerId, env),
        listNotificationConfigs(providerId, env),
      ]);
      resolvedWorkspace = ws ?? workspace;
      domains = customDomains;
      notifications = configs;
    } catch (err) {
      loadError = err instanceof Error ? err.message : "设置加载失败";
    }
  }

  return (
    <SettingsClient
      providerId={providerId}
      providerName={workspace?.name ?? provider?.name ?? ""}
      providerSlug={workspace?.slug ?? provider?.slug ?? ""}
      env={env}
      workspace={resolvedWorkspace}
      domains={domains}
      notifications={notifications}
      loadError={loadError}
    />
  );
}
