import { listRegions } from "@/lib/api";
import { CreateProviderForm } from "./create-form";

export const dynamic = "force-dynamic";

export default async function NewProviderPage() {
  let regions: Awaited<ReturnType<typeof listRegions>> = [];
  let regionsError: string | null = null;
  try {
    regions = await listRegions();
  } catch (err) {
    regionsError =
      err instanceof Error ? err.message : "Failed to load regions";
  }

  return (
    <div className="flex max-w-xl flex-col gap-6">
      <div>
        <h1 className="text-xl font-semibold tracking-tight">New Provider</h1>
        <p className="mt-1 text-sm text-zinc-600">
          Onboard a provider. A test environment and API key are created
          automatically.
        </p>
      </div>
      {regionsError && (
        <div
          role="alert"
          className="rounded-md border border-red-300 bg-red-50 p-4 text-sm text-red-800"
        >
          Could not load regions: {regionsError}
        </div>
      )}
      <CreateProviderForm regions={regions} />
    </div>
  );
}
