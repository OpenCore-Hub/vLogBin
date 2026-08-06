import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listCustomers,
  type Customer,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { CustomersClient } from "./customers-client";

export const dynamic = "force-dynamic";

export default async function CustomersPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const provider = await resolveWorkspaceProvider();

  let customers: Customer[] = [];
  let loadError: string | null = null;
  if (provider) {
    try {
      customers = await listCustomers(provider.id, env);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "客户列表加载失败";
    }
  }

  return (
    <CustomersClient
      providerId={provider?.id ?? null}
      env={env}
      customers={customers}
      loadError={loadError}
    />
  );
}
