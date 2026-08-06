import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  listInvoices,
  type Invoice,
} from "@/lib/api/operator";
import { resolveWorkspaceProvider } from "@/lib/workspace";
import { InvoicesClient } from "./invoices-client";

export const dynamic = "force-dynamic";

export default async function InvoicesPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const provider = await resolveWorkspaceProvider();

  let invoices: Invoice[] = [];
  let loadError: string | null = null;
  if (provider) {
    try {
      invoices = await listInvoices(provider.id, env);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "账单列表加载失败";
    }
  }

  return (
    <InvoicesClient
      providerId={provider?.id ?? null}
      env={env}
      invoices={invoices}
      loadError={loadError}
    />
  );
}
