import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listInvoices,
  listProviders,
  type Invoice,
} from "@/lib/api/operator";
import { PaymentsClient } from "./payments-client";

export const dynamic = "force-dynamic";

export default async function PaymentsPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const providers = await listProviders().catch(() => []);
  const provider = providers[0] ?? null;

  let invoices: Invoice[] = [];
  let loadError: string | null = null;
  if (provider) {
    try {
      invoices = await listInvoices(provider.id, env);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "支付数据加载失败";
    }
  }

  return (
    <PaymentsClient
      providerId={provider?.id ?? null}
      env={env}
      invoices={invoices}
      loadError={loadError}
    />
  );
}
