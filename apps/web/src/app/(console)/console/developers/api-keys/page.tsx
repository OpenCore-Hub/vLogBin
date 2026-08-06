import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listCredentials,
  type Credential,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { ApiKeysClient } from "./api-keys-client";

export const dynamic = "force-dynamic";

export default async function ApiKeysPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const provider = await resolveWorkspaceProvider();

  let credentials: Credential[] = [];
  let loadError: string | null = null;
  if (provider) {
    try {
      credentials = await listCredentials(provider.id, env);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "密钥列表加载失败";
    }
  }

  return (
    <ApiKeysClient
      providerId={provider?.id ?? null}
      env={env}
      credentials={credentials}
      loadError={loadError}
    />
  );
}
