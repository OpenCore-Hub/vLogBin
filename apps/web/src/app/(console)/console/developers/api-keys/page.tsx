import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listCredentials,
  listProviders,
  type Credential,
} from "@/lib/api/operator";
import { ApiKeysClient } from "./api-keys-client";

export const dynamic = "force-dynamic";

export default async function ApiKeysPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const providers = await listProviders().catch(() => []);
  const provider = providers[0] ?? null;

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
