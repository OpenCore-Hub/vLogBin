import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  getInvoiceDetail,
  listProviders,
  type InvoiceDetail,
} from "@/lib/api/operator";
import { InvoiceDetailClient } from "./invoice-detail-client";

export const dynamic = "force-dynamic";

export default async function InvoiceDetailPage({
  params,
}: {
  params: Promise<{ invoiceId: string }>;
}) {
  const session = await requireAuth();
  const env = await resolveEnv(session);
  const { invoiceId } = await params;

  const providers = await listProviders().catch(() => []);
  const provider = providers[0] ?? null;

  let detail: InvoiceDetail | null = null;
  let loadError: string | null = null;
  if (provider) {
    try {
      detail = await getInvoiceDetail(provider.id, env, invoiceId);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "账单详情加载失败";
    }
  }

  return (
    <InvoiceDetailClient
      providerId={provider?.id ?? null}
      env={env}
      invoiceId={invoiceId}
      detail={detail}
      loadError={loadError}
    />
  );
}
