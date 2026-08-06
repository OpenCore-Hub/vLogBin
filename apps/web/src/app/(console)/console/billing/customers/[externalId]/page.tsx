import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  getCustomerDetail,
  type CustomerDetail,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { CustomerDetailClient } from "./customer-detail-client";

export const dynamic = "force-dynamic";

export default async function CustomerDetailPage({
  params,
}: {
  params: Promise<{ externalId: string }>;
}) {
  const session = await requireAuth();
  const env = await resolveEnv(session);
  const { externalId } = await params;

  const provider = await resolveWorkspaceProvider();

  let detail: CustomerDetail | null = null;
  let loadError: string | null = null;
  if (provider) {
    try {
      detail = await getCustomerDetail(provider.id, env, externalId);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "客户详情加载失败";
    }
  }

  return (
    <CustomerDetailClient
      providerId={provider?.id ?? null}
      env={env}
      externalId={externalId}
      detail={detail}
      loadError={loadError}
    />
  );
}
