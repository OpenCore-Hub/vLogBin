import Link from "next/link";
import {
  getCatalogVersionDetail,
  type EntitlementGrant,
  type Metric,
  type Plan,
  type Price,
} from "@/lib/api";

export const dynamic = "force-dynamic";

function formatDate(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString("en-US");
}

function badgeClass(styles: string): string {
  return `inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${styles}`;
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

function ChargeModelBadge({ model }: { model: string }) {
  const styles =
    model === "fixed"
      ? "bg-sky-100 text-sky-800 ring-sky-300"
      : model === "per_unit"
        ? "bg-violet-100 text-violet-800 ring-violet-300"
        : model === "tiered"
          ? "bg-amber-100 text-amber-800 ring-amber-300"
          : "bg-zinc-100 text-zinc-700 ring-zinc-300";
  return <span className={badgeClass(styles)}>{model || "unknown"}</span>;
}

function MetaRow({ label, value }: { label: string; value?: string }) {
  return (
    <div className="flex gap-4 py-2 text-sm">
      <dt className="w-40 shrink-0 text-zinc-500">{label}</dt>
      <dd className="font-mono text-zinc-900">{value || "—"}</dd>
    </div>
  );
}

function EmptyTable({ message }: { message: string }) {
  return (
    <p className="rounded-md border border-zinc-200 bg-white p-4 text-sm text-zinc-500">
      {message}
    </p>
  );
}

function MetricsTable({ metrics }: { metrics: Metric[] }) {
  if (metrics.length === 0) {
    return <EmptyTable message="No metrics in this catalog version." />;
  }
  return (
    <div className="overflow-hidden rounded-md border border-zinc-200 bg-white">
      <table className="w-full text-left">
        <thead className="bg-zinc-50 text-xs uppercase tracking-wide text-zinc-500">
          <tr>
            <th className="px-4 py-2 font-medium">Code</th>
            <th className="px-4 py-2 font-medium">Name</th>
            <th className="px-4 py-2 font-medium">Aggregation</th>
            <th className="px-4 py-2 font-medium">Field</th>
            <th className="px-4 py-2 font-medium">Billable</th>
          </tr>
        </thead>
        <tbody>
          {metrics.map((m) => (
            <tr key={m.id} className="border-t border-zinc-200">
              <td className="px-4 py-3 font-mono text-sm text-zinc-900">
                {m.code || "—"}
              </td>
              <td className="px-4 py-3 text-sm">{m.name || "—"}</td>
              <td className="px-4 py-3 text-sm text-zinc-600">
                {m.aggregation_type || "—"}
              </td>
              <td className="px-4 py-3 font-mono text-sm text-zinc-600">
                {m.field_name ?? "—"}
              </td>
              <td className="px-4 py-3 text-sm text-zinc-600">
                {m.billable ? "Yes" : "No"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function PlansTable({ plans }: { plans: Plan[] }) {
  if (plans.length === 0) {
    return <EmptyTable message="No plans in this catalog version." />;
  }
  return (
    <div className="overflow-hidden rounded-md border border-zinc-200 bg-white">
      <table className="w-full text-left">
        <thead className="bg-zinc-50 text-xs uppercase tracking-wide text-zinc-500">
          <tr>
            <th className="px-4 py-2 font-medium">Code</th>
            <th className="px-4 py-2 font-medium">Name</th>
            <th className="px-4 py-2 font-medium">Interval</th>
            <th className="px-4 py-2 font-medium">Currency</th>
          </tr>
        </thead>
        <tbody>
          {plans.map((p) => (
            <tr key={p.id} className="border-t border-zinc-200">
              <td className="px-4 py-3 font-mono text-sm text-zinc-900">
                {p.code || "—"}
              </td>
              <td className="px-4 py-3 text-sm">{p.name || "—"}</td>
              <td className="px-4 py-3 text-sm text-zinc-600">
                {p.interval || "—"}
              </td>
              <td className="px-4 py-3 font-mono text-sm text-zinc-600">
                {p.currency || "—"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function formatPriceProperties(value: unknown): string {
  if (value === undefined || value === null) return "—";
  if (typeof value === "string") return value;
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function PricesTable({ prices }: { prices: Price[] }) {
  if (prices.length === 0) {
    return <EmptyTable message="No prices in this catalog version." />;
  }
  return (
    <div className="overflow-hidden rounded-md border border-zinc-200 bg-white">
      <table className="w-full text-left">
        <thead className="bg-zinc-50 text-xs uppercase tracking-wide text-zinc-500">
          <tr>
            <th className="px-4 py-2 font-medium">Charge Model</th>
            <th className="px-4 py-2 font-medium">Metric</th>
            <th className="px-4 py-2 font-medium">Properties</th>
          </tr>
        </thead>
        <tbody>
          {prices.map((p) => (
            <tr key={p.id} className="border-t border-zinc-200">
              <td className="px-4 py-3">
                <ChargeModelBadge model={p.charge_model} />
              </td>
              <td className="px-4 py-3 font-mono text-sm text-zinc-600">
                {p.metric_code ?? "—"}
              </td>
              <td className="px-4 py-3 font-mono text-xs text-zinc-600">
                {formatPriceProperties(p.properties)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function formatGrantValue(value: unknown): string {
  if (value === undefined || value === null) return "—";
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}

function EntitlementGrantsTable({
  grants,
}: {
  grants: EntitlementGrant[];
}) {
  if (grants.length === 0) {
    return (
      <EmptyTable message="No entitlement grants in this catalog version." />
    );
  }
  return (
    <div className="overflow-hidden rounded-md border border-zinc-200 bg-white">
      <table className="w-full text-left">
        <thead className="bg-zinc-50 text-xs uppercase tracking-wide text-zinc-500">
          <tr>
            <th className="px-4 py-2 font-medium">Key</th>
            <th className="px-4 py-2 font-medium">Value Type</th>
            <th className="px-4 py-2 font-medium">Value</th>
          </tr>
        </thead>
        <tbody>
          {grants.map((g) => (
            <tr key={g.id} className="border-t border-zinc-200">
              <td className="px-4 py-3 font-mono text-sm text-zinc-900">
                {g.key || "—"}
              </td>
              <td className="px-4 py-3 text-sm text-zinc-600">
                {g.value_type || "—"}
              </td>
              <td className="px-4 py-3 font-mono text-sm text-zinc-600">
                {formatGrantValue(g.value)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
      {children}
    </h2>
  );
}

export default async function CatalogVersionDetailPage({
  params,
}: {
  params: Promise<{ id: string; versionId: string }>;
}) {
  const { id, versionId } = await params;

  let detail = null;
  let loadError: string | null = null;
  try {
    detail = await getCatalogVersionDetail(id, versionId);
  } catch (err) {
    loadError =
      err instanceof Error ? err.message : "Failed to load catalog version";
  }

  const backHref = `/providers/${id}`;

  if (loadError) {
    return (
      <div className="flex flex-col gap-4">
        <Link
          href={backHref}
          className="text-sm text-sky-700 hover:underline"
        >
          &larr; Back to provider
        </Link>
        <div
          role="alert"
          className="rounded-md border border-red-300 bg-red-50 p-4 text-sm text-red-800"
        >
          Could not load catalog version: {loadError}
        </div>
      </div>
    );
  }

  if (!detail) {
    return (
      <div className="flex flex-col gap-4">
        <Link
          href={backHref}
          className="text-sm text-sky-700 hover:underline"
        >
          &larr; Back to provider
        </Link>
        <div
          role="alert"
          className="rounded-md border border-red-300 bg-red-50 p-4 text-sm text-red-800"
        >
          Catalog version not found.
        </div>
      </div>
    );
  }

  const { version, metrics, plans, prices, entitlement_grants } = detail;

  return (
    <div className="flex flex-col gap-8">
      <div>
        <Link href={backHref} className="text-sm text-sky-700 hover:underline">
          &larr; Back to provider
        </Link>
        <div className="mt-2 flex items-center gap-3">
          <h1 className="text-xl font-semibold tracking-tight">
            Catalog v{version.version}
          </h1>
          <CatalogStateBadge state={version.state} />
        </div>
      </div>

      <section>
        <SectionTitle>Metadata</SectionTitle>
        <dl className="mt-2 divide-y divide-zinc-100 rounded-md border border-zinc-200 bg-white px-4">
          <MetaRow label="ID" value={version.id} />
          <MetaRow label="Provider ID" value={version.provider_id} />
          <MetaRow label="Environment" value={version.environment_id} />
          <MetaRow label="Version" value={`v${version.version}`} />
          <MetaRow label="Created" value={formatDate(version.created_at)} />
          <MetaRow
            label="Validated"
            value={formatDate(version.validated_at)}
          />
          <MetaRow
            label="Published"
            value={formatDate(version.published_at)}
          />
          <MetaRow label="Retired" value={formatDate(version.retired_at)} />
        </dl>
      </section>

      <section>
        <SectionTitle>Metrics</SectionTitle>
        <div className="mt-2">
          <MetricsTable metrics={metrics} />
        </div>
      </section>

      <section>
        <SectionTitle>Plans</SectionTitle>
        <div className="mt-2">
          <PlansTable plans={plans} />
        </div>
      </section>

      <section>
        <SectionTitle>Prices</SectionTitle>
        <div className="mt-2">
          <PricesTable prices={prices} />
        </div>
      </section>

      <section>
        <SectionTitle>Entitlement Grants</SectionTitle>
        <div className="mt-2">
          <EntitlementGrantsTable grants={entitlement_grants} />
        </div>
      </section>
    </div>
  );
}
