import Link from "next/link";
import { listProviders, type Provider } from "@/lib/api";
import { LifecycleBadge } from "@/components/lifecycle-badge";

export const dynamic = "force-dynamic";

function formatDate(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString("en-US");
}

function ProviderRow({ provider }: { provider: Provider }) {
  return (
    <tr className="border-t border-zinc-200 hover:bg-zinc-50">
      <td className="px-4 py-3 font-mono text-sm">
        <Link
          href={`/providers/${provider.id}`}
          className="text-sky-700 hover:underline"
        >
          {provider.slug}
        </Link>
      </td>
      <td className="px-4 py-3 text-sm">{provider.name}</td>
      <td className="px-4 py-3">
        <LifecycleBadge state={provider.lifecycle_state} />
      </td>
      <td className="px-4 py-3 text-sm text-zinc-600">
        {provider.sla_tier ?? "—"}
      </td>
      <td className="px-4 py-3 text-sm text-zinc-600">
        {formatDate(provider.created_at)}
      </td>
    </tr>
  );
}

export default async function DashboardPage() {
  let providers: Provider[] = [];
  let loadError: string | null = null;
  try {
    providers = await listProviders();
  } catch (err) {
    loadError = err instanceof Error ? err.message : "Failed to load providers";
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Providers</h1>
          <p className="mt-1 text-sm text-zinc-600">
            All providers onboarded to the platform.
          </p>
        </div>
        <Link
          href="/providers/new"
          className="rounded-md bg-zinc-900 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-700"
        >
          New Provider
        </Link>
      </div>

      {loadError ? (
        <div
          role="alert"
          className="rounded-md border border-red-300 bg-red-50 p-4 text-sm text-red-800"
        >
          Could not load providers: {loadError}
        </div>
      ) : providers.length === 0 ? (
        <div className="rounded-md border border-zinc-200 bg-white p-8 text-center text-sm text-zinc-500">
          No providers yet. Create the first one.
        </div>
      ) : (
        <div className="overflow-hidden rounded-md border border-zinc-200 bg-white">
          <table className="w-full text-left">
            <thead className="bg-zinc-50 text-xs uppercase tracking-wide text-zinc-500">
              <tr>
                <th className="px-4 py-2 font-medium">Slug</th>
                <th className="px-4 py-2 font-medium">Name</th>
                <th className="px-4 py-2 font-medium">Lifecycle</th>
                <th className="px-4 py-2 font-medium">SLA tier</th>
                <th className="px-4 py-2 font-medium">Created</th>
              </tr>
            </thead>
            <tbody>
              {providers.map((p) => (
                <ProviderRow key={p.id || p.slug} provider={p} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
