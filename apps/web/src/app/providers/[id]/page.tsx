import Link from "next/link";
import { notFound } from "next/navigation";
import { getProvider, type Environment, type Provider } from "@/lib/api";
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
                <span
                  className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${
                    env.kind === "live"
                      ? "bg-emerald-100 text-emerald-800 ring-emerald-300"
                      : "bg-sky-100 text-sky-800 ring-sky-300"
                  }`}
                >
                  {env.kind}
                </span>
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

export default async function ProviderDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;

  let provider: Provider | null = null;
  let environments: Environment[] = [];
  let loadError: string | null = null;
  try {
    const data = await getProvider(id);
    provider = data.provider;
    environments = data.environments;
  } catch (err) {
    loadError = err instanceof Error ? err.message : "Failed to load provider";
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
    </div>
  );
}
