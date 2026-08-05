import { notFound } from "next/navigation";
import { requireRole } from "@/lib/auth/rbac";
import { LifecycleBadge } from "@/components/ui/badge";
import {
  getProvider,
  listCatalogVersions,
  getCatalogVersionDetail,
  listSubscriptions,
  listCustomers,
  listUsageEvents,
  listInvoices,
  listCapabilities,
  listRegions,
  listAuditEvents,
  listCredentials,
  listWebhookDeliveries,
  type CatalogVersion,
  type CatalogVersionDetail,
  type Capability,
  type Region,
  type AuditEvent,
  type Credential,
  type WebhookDelivery,
} from "@/lib/api/operator";
import { ProviderDetail } from "./provider-detail";

export const dynamic = "force-dynamic";

async function safeList<T>(fn: () => Promise<T[]>): Promise<T[]> {
  try {
    return await fn();
  } catch {
    return [];
  }
}

export default async function ProviderDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  await requireRole("operator");
  const { id } = await params;

  const { provider, environments } = await getProvider(id);
  if (!provider) notFound();

  const [
    versions,
    subscriptions,
    customers,
    usageEvents,
    invoices,
    capabilities,
    regions,
    auditEvents,
    credentials,
    deliveries,
  ] = await Promise.all([
    safeList(() => listCatalogVersions(id)),
    safeList(() => listSubscriptions(id)),
    safeList(() => listCustomers(id)),
    safeList(() => listUsageEvents(id)),
    safeList(() => listInvoices(id)),
    safeList<Capability>(() => listCapabilities(id)),
    safeList<Region>(() => listRegions()),
    safeList<AuditEvent>(() => listAuditEvents(id)),
    safeList<Credential>(() => listCredentials(id)),
    safeList<WebhookDelivery>(() => listWebhookDeliveries(id)),
  ]);

  // 最新已发布版本详情（用于目录 tab 的指标/套餐展示）。
  let detail: CatalogVersionDetail | null = null;
  const published = versions
    .filter((v): v is CatalogVersion => v.state === "published")
    .sort((a, b) => b.version - a.version)[0];
  if (published) {
    try {
      detail = await getCatalogVersionDetail(id, published.id);
    } catch {
      detail = null;
    }
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-center gap-3">
        <h1 className="min-w-0 truncate text-2xl font-semibold tracking-tight">
          {provider.name}
        </h1>
        <LifecycleBadge state={provider.lifecycle_state} />
        <span className="font-mono text-sm text-muted-foreground">@{provider.slug}</span>
      </header>

      <ProviderDetail
        provider={provider}
        environments={environments}
        capabilities={capabilities}
        versions={versions}
        detail={detail}
        subscriptions={subscriptions}
        customers={customers}
        usageEvents={usageEvents}
        invoices={invoices}
        regions={regions}
        auditEvents={auditEvents}
        credentials={credentials}
        deliveries={deliveries}
      />
    </div>
  );
}
