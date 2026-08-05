import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  getCustomerDetail,
  listProviders,
  type CustomerDetail,
} from "@/lib/api/operator";
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

  const providers = await listProviders().catch(() => []);
  const provider = providers[0] ?? null;

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
