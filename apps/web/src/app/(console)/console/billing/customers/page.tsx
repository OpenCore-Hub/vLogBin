import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listCustomers,
  listProviders,
  type Customer,
} from "@/lib/api/operator";
import { CustomersClient } from "./customers-client";

export const dynamic = "force-dynamic";

export default async function CustomersPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const providers = await listProviders().catch(() => []);
  // 注册即建 workspace（R11）：当前实现为 1:1 provider；后续多 workspace
  // 切换落地后改由会话 workspaceId 映射。
  const provider = providers[0] ?? null;

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
