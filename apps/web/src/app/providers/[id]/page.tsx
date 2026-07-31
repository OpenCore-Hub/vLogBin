import Link from "next/link";
import { notFound } from "next/navigation";
import {
  getProvider,
  listCatalogVersions,
  listSubscriptions,
  listCustomers,
  listUsageEvents,
  listInvoices,
  listCapabilities,
  listWebhooks,
  listWebhookDeliveries,
  type Capability,
  type WebhookEndpoint,
  type WebhookDelivery,
  type CatalogVersion,
  type Customer,
  type Environment,
  type Invoice,
  type Provider,
  type Subscription,
  type UsageEvent,
} from "@/lib/api";
import { LifecycleBadge } from "@/components/lifecycle-badge";
import { LifecycleActions } from "./lifecycle-actions";

export const dynamic = "force-dynamic";

function formatDate(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString("en-US");
}

function MetaRow({ label, value }: { label: string; value?: string }) {
  return (
    <div className="flex gap-4 py-2 text-sm">
      <dt className="w-40 shrink-0 text-zinc-500">{label}</dt>
      <dd className="font-mono text-zinc-900">{value || "—"}</dd>
    </div>
  );
}

function badgeClass(styles: string): string {
  return `inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${styles}`;
}

function KindBadge({ kind }: { kind: string }) {
  const styles =
    kind === "live"
      ? "bg-emerald-100 text-emerald-800 ring-emerald-300"
      : kind === "test"
        ? "bg-sky-100 text-sky-800 ring-sky-300"
        : "bg-zinc-100 text-zinc-700 ring-zinc-300";
  return <span className={badgeClass(styles)}>{kind}</span>;
}

const CATALOG_STATE_STYLES: Record<string, string> = {
  draft: "bg-zinc-100 text-zinc-700 ring-zinc-300",
  validated: "bg-amber-100 text-amber-800 ring-amber-300",
  published: "bg-emerald-100 text-emerald-800 ring-emerald-300",
  retired: "bg-red-100 text-red-800 ring-red-300",
};

function CatalogStateBadge({ state }: { state: string }) {
  const styles =
    CATALOG_STATE_STYLES[state] ?? "bg-zinc-100 text-zinc-700 ring-zinc-300";
  return <span className={badgeClass(styles)}>{state || "unknown"}</span>;
}

function SubscriptionStatusBadge({ status }: { status: string }) {
  const styles =
    status === "active"
      ? "bg-emerald-100 text-emerald-800 ring-emerald-300"
      : "bg-zinc-100 text-zinc-700 ring-zinc-300";
  return <span className={badgeClass(styles)}>{status || "unknown"}</span>;
}

function AccountTypeBadge({ accountType }: { accountType: string }) {
  const styles =
    accountType === "individual"
      ? "bg-sky-100 text-sky-800 ring-sky-300"
      : accountType === "business"
        ? "bg-emerald-100 text-emerald-800 ring-emerald-300"
        : "bg-zinc-100 text-zinc-700 ring-zinc-300";
  return <span className={badgeClass(styles)}>{accountType || "unknown"}</span>;
}

function UsageKindBadge({ kind }: { kind: string }) {
  const styles =
    kind === "ingestion"
      ? "bg-sky-100 text-sky-800 ring-sky-300"
      : kind === "reversal"
        ? "bg-amber-100 text-amber-800 ring-amber-300"
        : "bg-zinc-100 text-zinc-700 ring-zinc-300";
  return <span className={badgeClass(styles)}>{kind || "unknown"}</span>;
}

const INVOICE_TYPE_STYLES: Record<string, string> = {
  subscription: "bg-sky-100 text-sky-800 ring-sky-300",
  add_on: "bg-violet-100 text-violet-800 ring-violet-300",
  credit: "bg-emerald-100 text-emerald-800 ring-emerald-300",
  one_off: "bg-amber-100 text-amber-800 ring-amber-300",
  progressive_billing: "bg-zinc-100 text-zinc-700 ring-zinc-300",
};

function InvoiceTypeBadge({ type }: { type: string }) {
  const styles =
    INVOICE_TYPE_STYLES[type] ?? "bg-zinc-100 text-zinc-700 ring-zinc-300";
  return <span className={badgeClass(styles)}>{type || "unknown"}</span>;
}

const INVOICE_STATUS_STYLES: Record<string, string> = {
  draft: "bg-zinc-100 text-zinc-700 ring-zinc-300",
  finalized: "bg-emerald-100 text-emerald-800 ring-emerald-300",
  voided: "bg-rose-100 text-rose-800 ring-rose-300",
  pending: "bg-amber-100 text-amber-800 ring-amber-300",
  failed: "bg-red-100 text-red-800 ring-red-300",
};

function InvoiceStatusBadge({ status }: { status: string }) {
  const styles =
    INVOICE_STATUS_STYLES[status] ?? "bg-zinc-100 text-zinc-700 ring-zinc-300";
  return <span className={badgeClass(styles)}>{status || "unknown"}</span>;
}

const INVOICE_PAYMENT_STYLES: Record<string, string> = {
  pending: "bg-amber-100 text-amber-800 ring-amber-300",
  succeeded: "bg-emerald-100 text-emerald-800 ring-emerald-300",
  failed: "bg-rose-100 text-rose-800 ring-rose-300",
};

function PaymentStatusBadge({ status }: { status: string }) {
  const styles =
    INVOICE_PAYMENT_STYLES[status] ?? "bg-zinc-100 text-zinc-700 ring-zinc-300";
  return <span className={badgeClass(styles)}>{status || "unknown"}</span>;
}

function SectionError({ message }: { message: string }) {
  return (
    <p
      role="alert"
      className="rounded-md border border-red-300 bg-red-50 p-4 text-sm text-red-800"
    >
      {message}
    </p>
  );
}

function CatalogVersionsTable({
  versions,
  providerId,
}: {
  versions: CatalogVersion[];
  providerId: string;
}) {
  if (versions.length === 0) {
    return (
      <p className="rounded-md border border-zinc-200 bg-white p-4 text-sm text-zinc-500">
        No catalog versions yet.
      </p>
    );
  }
  return (
    <div className="overflow-hidden rounded-md border border-zinc-200 bg-white">
      <table className="w-full text-left">
        <thead className="bg-zinc-50 text-xs uppercase tracking-wide text-zinc-500">
          <tr>
            <th className="px-4 py-2 font-medium">Version</th>
            <th className="px-4 py-2 font-medium">State</th>
            <th className="px-4 py-2 font-medium">Environment</th>
            <th className="px-4 py-2 font-medium">Metrics</th>
            <th className="px-4 py-2 font-medium">Plans</th>
            <th className="px-4 py-2 font-medium">Published</th>
            <th className="px-4 py-2 font-medium"></th>
          </tr>
        </thead>
        <tbody>
          {versions.map((v) => (
            <tr key={v.id} className="border-t border-zinc-200">
              <td className="px-4 py-3 font-mono text-sm">v{v.version}</td>
              <td className="px-4 py-3">
                <CatalogStateBadge state={v.state} />
              </td>
              <td className="px-4 py-3">
                <KindBadge kind={v.environment_kind} />
              </td>
              <td className="px-4 py-3 text-sm text-zinc-600">
                {v.metrics_count}
              </td>
              <td className="px-4 py-3 text-sm text-zinc-600">
                {v.plans_count}
              </td>
              <td className="px-4 py-3 text-sm text-zinc-600">
                {formatDate(v.published_at)}
              </td>
              <td className="px-4 py-3 text-sm">
                <Link
                  href={`/providers/${providerId}/catalog/${v.id}`}
                  className="text-sky-700 hover:underline"
                >
                  View
                </Link>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function SubscriptionsTable({
  subscriptions,
}: {
  subscriptions: Subscription[];
}) {
  if (subscriptions.length === 0) {
    return (
      <p className="rounded-md border border-zinc-200 bg-white p-4 text-sm text-zinc-500">
        No subscriptions yet.
      </p>
    );
  }
  return (
    <div className="overflow-hidden rounded-md border border-zinc-200 bg-white">
      <table className="w-full text-left">
        <thead className="bg-zinc-50 text-xs uppercase tracking-wide text-zinc-500">
          <tr>
            <th className="px-4 py-2 font-medium">External ID</th>
            <th className="px-4 py-2 font-medium">Customer</th>
            <th className="px-4 py-2 font-medium">Plan</th>
            <th className="px-4 py-2 font-medium">Status</th>
            <th className="px-4 py-2 font-medium">Environment</th>
            <th className="px-4 py-2 font-medium">Started</th>
          </tr>
        </thead>
        <tbody>
          {subscriptions.map((s) => (
            <tr key={s.id} className="border-t border-zinc-200">
              <td className="px-4 py-3 font-mono text-sm text-zinc-900">
                {s.external_id || "—"}
              </td>
              <td className="px-4 py-3 font-mono text-sm text-zinc-600">
                {s.customer_external_id || "—"}
              </td>
              <td className="px-4 py-3 text-sm">{s.plan_code || "—"}</td>
              <td className="px-4 py-3">
                <SubscriptionStatusBadge status={s.status} />
              </td>
              <td className="px-4 py-3">
                <KindBadge kind={s.environment_kind} />
              </td>
              <td className="px-4 py-3 text-sm text-zinc-600">
                {formatDate(s.started_at)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function EnvironmentTable({
  environments,
}: {
  environments: Environment[];
}) {
  if (environments.length === 0) {
    return (
      <p className="rounded-md border border-zinc-200 bg-white p-4 text-sm text-zinc-500">
        No environments.
      </p>
    );
  }
  return (
    <div className="overflow-hidden rounded-md border border-zinc-200 bg-white">
      <table className="w-full text-left">
        <thead className="bg-zinc-50 text-xs uppercase tracking-wide text-zinc-500">
          <tr>
            <th className="px-4 py-2 font-medium">Kind</th>
            <th className="px-4 py-2 font-medium">Status</th>
            <th className="px-4 py-2 font-medium">Issuer</th>
            <th className="px-4 py-2 font-medium">Created</th>
          </tr>
        </thead>
        <tbody>
          {environments.map((env) => (
            <tr key={env.id} className="border-t border-zinc-200">
              <td className="px-4 py-3">
                <KindBadge kind={env.kind} />
              </td>
              <td className="px-4 py-3 text-sm">{env.status}</td>
              <td className="px-4 py-3 font-mono text-sm text-zinc-600">
                {env.issuer ?? "—"}
              </td>
              <td className="px-4 py-3 text-sm text-zinc-600">
                {formatDate(env.created_at)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function CustomersTable({ customers }: { customers: Customer[] }) {
  if (customers.length === 0) {
    return (
      <p className="rounded-md border border-zinc-200 bg-white p-4 text-sm text-zinc-500">
        No customers yet.
      </p>
    );
  }
  return (
    <div className="overflow-hidden rounded-md border border-zinc-200 bg-white">
      <table className="w-full text-left">
        <thead className="bg-zinc-50 text-xs uppercase tracking-wide text-zinc-500">
          <tr>
            <th className="px-4 py-2 font-medium">External ID</th>
            <th className="px-4 py-2 font-medium">Account Type</th>
            <th className="px-4 py-2 font-medium">Display Name</th>
            <th className="px-4 py-2 font-medium">Environment</th>
            <th className="px-4 py-2 font-medium">Created</th>
          </tr>
        </thead>
        <tbody>
          {customers.map((c) => (
            <tr key={c.id} className="border-t border-zinc-200">
              <td className="px-4 py-3 font-mono text-sm text-zinc-900">
                {c.external_id || "—"}
              </td>
              <td className="px-4 py-3">
                <AccountTypeBadge accountType={c.account_type} />
              </td>
              <td className="px-4 py-3 text-sm">{c.display_name || "—"}</td>
              <td className="px-4 py-3">
                <KindBadge kind={c.environment_kind} />
              </td>
              <td className="px-4 py-3 text-sm text-zinc-600">
                {formatDate(c.created_at)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function UsageEventsTable({ events }: { events: UsageEvent[] }) {
  if (events.length === 0) {
    return (
      <p className="rounded-md border border-zinc-200 bg-white p-4 text-sm text-zinc-500">
        No usage events yet.
      </p>
    );
  }
  return (
    <div className="overflow-hidden rounded-md border border-zinc-200 bg-white">
      <table className="w-full text-left">
        <thead className="bg-zinc-50 text-xs uppercase tracking-wide text-zinc-500">
          <tr>
            <th className="px-4 py-2 font-medium">Transaction ID</th>
            <th className="px-4 py-2 font-medium">Kind</th>
            <th className="px-4 py-2 font-medium">Metric</th>
            <th className="px-4 py-2 font-medium">Customer</th>
            <th className="px-4 py-2 font-medium">Environment</th>
            <th className="px-4 py-2 font-medium">Event Time</th>
          </tr>
        </thead>
        <tbody>
          {events.map((e) => (
            <tr key={e.id} className="border-t border-zinc-200">
              <td className="px-4 py-3 font-mono text-sm text-zinc-900">
                {e.transaction_id || "—"}
              </td>
              <td className="px-4 py-3">
                <UsageKindBadge kind={e.kind} />
              </td>
              <td className="px-4 py-3 text-sm">{e.metric_code || "—"}</td>
              <td className="px-4 py-3 font-mono text-sm text-zinc-600">
                {e.customer_external_id || "—"}
              </td>
              <td className="px-4 py-3">
                <KindBadge kind={e.environment_kind} />
              </td>
              <td className="px-4 py-3 text-sm text-zinc-600">
                {formatDate(e.event_timestamp)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function formatAmount(cents: number, currency: string): string {
  return `${currency} ${(cents / 100).toFixed(2)}`;
}

function InvoicesTable({ invoices }: { invoices: Invoice[] }) {
  if (invoices.length === 0) {
    return (
      <p className="rounded-md border border-zinc-200 bg-white p-4 text-sm text-zinc-500">
        No invoices yet.
      </p>
    );
  }
  return (
    <div className="overflow-hidden rounded-md border border-zinc-200 bg-white">
      <table className="w-full text-left">
        <thead className="bg-zinc-50 text-xs uppercase tracking-wide text-zinc-500">
          <tr>
            <th className="px-4 py-2 font-medium">Number</th>
            <th className="px-4 py-2 font-medium">Type</th>
            <th className="px-4 py-2 font-medium">Status</th>
            <th className="px-4 py-2 font-medium">Payment</th>
            <th className="px-4 py-2 font-medium">Customer</th>
            <th className="px-4 py-2 font-medium">Amount</th>
            <th className="px-4 py-2 font-medium">Issuing Date</th>
            <th className="px-4 py-2 font-medium">Environment</th>
          </tr>
        </thead>
        <tbody>
          {invoices.map((inv) => (
            <tr key={inv.id} className="border-t border-zinc-200">
              <td className="px-4 py-3 font-mono text-sm text-zinc-900">
                {inv.number || "—"}
              </td>
              <td className="px-4 py-3">
                <InvoiceTypeBadge type={inv.invoice_type} />
              </td>
              <td className="px-4 py-3">
                <InvoiceStatusBadge status={inv.status} />
              </td>
              <td className="px-4 py-3">
                <PaymentStatusBadge status={inv.payment_status} />
              </td>
              <td className="px-4 py-3 font-mono text-sm text-zinc-600">
                {inv.customer_external_id || "—"}
              </td>
              <td className="px-4 py-3 font-mono text-sm text-zinc-900">
                {formatAmount(inv.total_amount_cents, inv.currency || "—")}
              </td>
              <td className="px-4 py-3 text-sm text-zinc-600">
                {inv.issuing_date || "—"}
              </td>
              <td className="px-4 py-3">
                <KindBadge kind={inv.environment_kind} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default async function ProviderDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  const [
    providerResult,
    versionsResult,
    subscriptionsResult,
    customersResult,
    usageEventsResult,
    invoicesResult,
    capabilitiesResult,
    webhooksResult,
    webhookDeliveriesResult,
  ] = await Promise.allSettled([
    getProvider(id),
    listCatalogVersions(id),
    listSubscriptions(id),
    listCustomers(id),
    listUsageEvents(id),
    listInvoices(id),
    listCapabilities(id),
    listWebhooks(id),
    listWebhookDeliveries(id),
  ]);

  let provider: Provider | null = null;
  let environments: Environment[] = [];
  let loadError: string | null = null;
  if (providerResult.status === "fulfilled") {
    provider = providerResult.value.provider;
    environments = providerResult.value.environments;
  } else {
    loadError =
      providerResult.reason instanceof Error
        ? providerResult.reason.message
        : "Failed to load provider";
  }

  let catalogVersions: CatalogVersion[] = [];
  let versionsError: string | null = null;
  if (versionsResult.status === "fulfilled") {
    catalogVersions = versionsResult.value;
  } else {
    versionsError =
      versionsResult.reason instanceof Error
        ? versionsResult.reason.message
        : "Failed to load catalog versions";
  }

  let subscriptions: Subscription[] = [];
  let subscriptionsError: string | null = null;
  if (subscriptionsResult.status === "fulfilled") {
    subscriptions = subscriptionsResult.value;
  } else {
    subscriptionsError =
      subscriptionsResult.reason instanceof Error
        ? subscriptionsResult.reason.message
        : "Failed to load subscriptions";
  }

  let customers: Customer[] = [];
  let customersError: string | null = null;
  if (customersResult.status === "fulfilled") {
    customers = customersResult.value;
  } else {
    customersError =
      customersResult.reason instanceof Error
        ? customersResult.reason.message
        : "Failed to load customers";
  }

  let usageEvents: UsageEvent[] = [];
  let usageEventsError: string | null = null;
  if (usageEventsResult.status === "fulfilled") {
    usageEvents = usageEventsResult.value;
  } else {
    usageEventsError =
      usageEventsResult.reason instanceof Error
        ? usageEventsResult.reason.message
        : "Failed to load usage events";
  }

  let invoices: Invoice[] = [];
  let invoicesError: string | null = null;
  if (invoicesResult.status === "fulfilled") {
    invoices = invoicesResult.value;
  } else {
    invoicesError =
      invoicesResult.reason instanceof Error
        ? invoicesResult.reason.message
        : "Failed to load invoices";
  }

  let capabilities: Capability[] = [];
  let capabilitiesError: string | null = null;
  if (capabilitiesResult.status === "fulfilled") {
    capabilities = capabilitiesResult.value;
  } else {
    capabilitiesError =
      capabilitiesResult.reason instanceof Error
        ? capabilitiesResult.reason.message
        : "Failed to load capabilities";
  }

  let webhooks: WebhookEndpoint[] = [];
  let webhooksError: string | null = null;
  if (webhooksResult.status === "fulfilled") {
    webhooks = webhooksResult.value;
  } else {
    webhooksError =
      webhooksResult.reason instanceof Error
        ? webhooksResult.reason.message
        : "Failed to load webhooks";
  }

  let webhookDeliveries: WebhookDelivery[] = [];
  let webhookDeliveriesError: string | null = null;
  if (webhookDeliveriesResult.status === "fulfilled") {
    webhookDeliveries = webhookDeliveriesResult.value;
  } else {
    webhookDeliveriesError =
      webhookDeliveriesResult.reason instanceof Error
        ? webhookDeliveriesResult.reason.message
        : "Failed to load webhook deliveries";
  }

  if (loadError) {
    return (
      <div className="flex flex-col gap-4">
        <Link
          href="/"
          className="text-sm text-sky-700 hover:underline"
        >
          ← Back to providers
        </Link>
        <div
          role="alert"
          className="rounded-md border border-red-300 bg-red-50 p-4 text-sm text-red-800"
        >
          Could not load provider: {loadError}
        </div>
      </div>
    );
  }

  if (!provider) notFound();

  return (
    <div className="flex flex-col gap-8">
      <div>
        <Link href="/" className="text-sm text-sky-700 hover:underline">
          ← Back to providers
        </Link>
        <div className="mt-2 flex items-center gap-3">
          <h1 className="text-xl font-semibold tracking-tight">
            {provider.name}
          </h1>
          <LifecycleBadge state={provider.lifecycle_state} />
        </div>
        <p className="mt-1 font-mono text-sm text-zinc-500">{provider.slug}</p>
      </div>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Metadata
        </h2>
        <dl className="mt-2 divide-y divide-zinc-100 rounded-md border border-zinc-200 bg-white px-4">
          <MetaRow label="ID" value={provider.id} />
          <MetaRow label="SLA tier" value={provider.sla_tier} />
          <MetaRow label="Home region ID" value={provider.home_region_id} />
          <MetaRow label="Cell ID" value={provider.cell_id} />
          <MetaRow label="Created" value={formatDate(provider.created_at)} />
          <MetaRow label="Updated" value={formatDate(provider.updated_at)} />
        </dl>
      </section>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Lifecycle actions
        </h2>
        <div className="mt-2">
          <LifecycleActions
            providerId={provider.id}
            currentState={provider.lifecycle_state}
          />
        </div>
      </section>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Environments
        </h2>
        <div className="mt-2">
          <EnvironmentTable environments={environments} />
        </div>
      </section>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Catalog versions
        </h2>
        <div className="mt-2">
          {versionsError ? (
            <SectionError
              message={`Could not load catalog versions: ${versionsError}`}
            />
          ) : (
            <CatalogVersionsTable
              versions={catalogVersions}
              providerId={id}
            />
          )}
        </div>
      </section>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Subscriptions
        </h2>
        <div className="mt-2">
          {subscriptionsError ? (
            <SectionError
              message={`Could not load subscriptions: ${subscriptionsError}`}
            />
          ) : (
            <SubscriptionsTable subscriptions={subscriptions} />
          )}
        </div>
      </section>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Customers
        </h2>
        <div className="mt-2">
          {customersError ? (
            <SectionError
              message={`Could not load customers: ${customersError}`}
            />
          ) : (
            <CustomersTable customers={customers} />
          )}
        </div>
      </section>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Usage Events
        </h2>
        <div className="mt-2">
          {usageEventsError ? (
            <SectionError
              message={`Could not load usage events: ${usageEventsError}`}
            />
          ) : (
            <UsageEventsTable events={usageEvents} />
          )}
        </div>
      </section>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Invoices
        </h2>
        <div className="mt-2">
          {invoicesError ? (
            <SectionError
              message={`Could not load invoices: ${invoicesError}`}
            />
          ) : (
            <InvoicesTable invoices={invoices} />
          )}
        </div>
      </section>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Live Capabilities
        </h2>
        <div className="mt-2">
          {capabilitiesError ? (
            <SectionError
              message={`Could not load capabilities: ${capabilitiesError}`}
            />
          ) : (
            <CapabilitiesTable capabilities={capabilities} />
          )}
        </div>
      </section>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Webhooks
        </h2>
        <div className="mt-2">
          {webhooksError ? (
            <SectionError
              message={`Could not load webhooks: ${webhooksError}`}
            />
          ) : (
            <WebhooksTable webhooks={webhooks} />
          )}
        </div>
      </section>

      <section>
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Webhook Deliveries
        </h2>
        <div className="mt-2">
          {webhookDeliveriesError ? (
            <SectionError
              message={`Could not load webhook deliveries: ${webhookDeliveriesError}`}
            />
          ) : (
            <WebhookDeliveriesTable deliveries={webhookDeliveries} />
          )}
        </div>
      </section>
    </div>
  );
}

function CapabilityStatusBadge({ status }: { status: string }) {
  const styles =
    status === "granted"
      ? "bg-emerald-100 text-emerald-800 ring-emerald-300"
      : status === "revoked"
        ? "bg-rose-100 text-rose-800 ring-rose-300"
        : "bg-zinc-100 text-zinc-700 ring-zinc-300";
  return <span className={badgeClass(styles)}>{status}</span>;
}

function CapabilitiesTable({ capabilities }: { capabilities: Capability[] }) {
  if (capabilities.length === 0) {
    return <p className="py-3 text-sm text-zinc-500">No capabilities granted.</p>;
  }
  return (
    <div className="overflow-x-auto rounded-md border border-zinc-200 bg-white">
      <table className="min-w-full divide-y divide-zinc-200 text-sm">
        <thead className="bg-zinc-50">
          <tr>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">
              Capability
            </th>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">
              Status
            </th>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">
              Granted By
            </th>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">
              Reason
            </th>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">
              Granted At
            </th>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">
              Revoked At
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-zinc-100">
          {capabilities.map((cap) => (
            <tr key={cap.id}>
              <td className="px-3 py-2 font-mono text-zinc-900">{cap.capability}</td>
              <td className="px-3 py-2">
                <CapabilityStatusBadge status={cap.status} />
              </td>
              <td className="px-3 py-2 text-zinc-700">{cap.granted_by || "—"}</td>
              <td className="px-3 py-2 text-zinc-700">{cap.reason || "—"}</td>
              <td className="px-3 py-2 text-zinc-500">{formatDate(cap.granted_at)}</td>
              <td className="px-3 py-2 text-zinc-500">{formatDate(cap.revoked_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function WebhooksTable({ webhooks }: { webhooks: WebhookEndpoint[] }) {
  if (webhooks.length === 0) {
    return <p className="py-3 text-sm text-zinc-500">No webhook endpoints configured.</p>;
  }
  return (
    <div className="overflow-x-auto rounded-md border border-zinc-200 bg-white">
      <table className="min-w-full divide-y divide-zinc-200 text-sm">
        <thead className="bg-zinc-50">
          <tr>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">URL</th>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">Enabled</th>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">Events</th>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">Created</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-zinc-100">
          {webhooks.map((wh) => (
            <tr key={wh.id}>
              <td className="px-3 py-2 font-mono text-xs text-zinc-900">{wh.url}</td>
              <td className="px-3 py-2">
                <span className={badgeClass(wh.enabled ? "bg-emerald-100 text-emerald-800 ring-emerald-300" : "bg-zinc-100 text-zinc-700 ring-zinc-300")}>
                  {wh.enabled ? "enabled" : "disabled"}
                </span>
              </td>
              <td className="px-3 py-2 text-zinc-700">{wh.events.length > 0 ? wh.events.join(", ") : "all"}</td>
              <td className="px-3 py-2 text-zinc-500">{formatDate(wh.created_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function WebhookDeliveriesTable({ deliveries }: { deliveries: WebhookDelivery[] }) {
  if (deliveries.length === 0) {
    return <p className="py-3 text-sm text-zinc-500">No webhook deliveries.</p>;
  }
  const statusStyles: Record<string, string> = {
    pending: "bg-amber-100 text-amber-800 ring-amber-300",
    delivered: "bg-emerald-100 text-emerald-800 ring-emerald-300",
    failed: "bg-rose-100 text-rose-800 ring-rose-300",
    dead_letter: "bg-red-100 text-red-800 ring-red-300",
  };
  return (
    <div className="overflow-x-auto rounded-md border border-zinc-200 bg-white">
      <table className="min-w-full divide-y divide-zinc-200 text-sm">
        <thead className="bg-zinc-50">
          <tr>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">Status</th>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">Attempts</th>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">Response</th>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">Event ID</th>
            <th className="px-3 py-2 text-left text-xs font-semibold uppercase text-zinc-500">Delivered At</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-zinc-100">
          {deliveries.map((d) => (
            <tr key={d.id}>
              <td className="px-3 py-2">
                <span className={badgeClass(statusStyles[d.status] ?? "bg-zinc-100 text-zinc-700 ring-zinc-300")}>
                  {d.status}
                </span>
              </td>
              <td className="px-3 py-2 text-zinc-700">{d.attempts}</td>
              <td className="px-3 py-2 text-zinc-700">{d.response_status ?? "—"}</td>
              <td className="px-3 py-2 font-mono text-xs text-zinc-500">{d.outbox_event_id.slice(0, 8)}…</td>
              <td className="px-3 py-2 text-zinc-500">{formatDate(d.delivered_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
