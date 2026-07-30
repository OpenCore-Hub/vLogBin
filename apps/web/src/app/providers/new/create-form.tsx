"use client";

import Link from "next/link";
import { useActionState } from "react";
import {
  createProviderAction,
  initialActionState,
} from "@/app/actions";
import { ApiKeyCallout } from "@/components/api-key-callout";
import type { Region } from "@/lib/api";

const inputClass =
  "w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm text-zinc-900 placeholder-zinc-400 focus:border-zinc-500 focus:outline-none";

export function CreateProviderForm({ regions }: { regions: Region[] }) {
  const [state, formAction, pending] = useActionState(
    createProviderAction,
    initialActionState,
  );

  if (state.ok) {
    return (
      <div className="flex flex-col gap-4">
        <div className="rounded-md border border-emerald-300 bg-emerald-50 p-4 text-sm text-emerald-800">
          Provider created successfully.
        </div>
        {state.apiKey ? (
          <ApiKeyCallout apiKey={state.apiKey} title="Test API key" />
        ) : (
          <div className="rounded-md border border-zinc-300 bg-zinc-50 p-4 text-sm text-zinc-700">
            The API did not return a test API key in the response.
          </div>
        )}
        <div className="flex gap-3">
          {state.providerId && (
            <Link
              href={`/providers/${state.providerId}`}
              className="rounded-md bg-zinc-900 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-700"
            >
              Go to provider
            </Link>
          )}
          <Link
            href="/"
            className="rounded-md border border-zinc-300 bg-white px-4 py-2 text-sm font-medium text-zinc-700 hover:bg-zinc-50"
          >
            Back to dashboard
          </Link>
        </div>
      </div>
    );
  }

  return (
    <form action={formAction} className="flex flex-col gap-4">
      {state.error && (
        <div
          role="alert"
          className="rounded-md border border-red-300 bg-red-50 p-4 text-sm text-red-800"
        >
          {state.error}
        </div>
      )}
      <div className="flex flex-col gap-1.5">
        <label htmlFor="slug" className="text-sm font-medium text-zinc-800">
          Slug
        </label>
        <input
          id="slug"
          name="slug"
          type="text"
          required
          placeholder="acme-logs"
          pattern="[a-z0-9-]+"
          title="Lowercase letters, digits and hyphens"
          className={inputClass}
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <label htmlFor="name" className="text-sm font-medium text-zinc-800">
          Name
        </label>
        <input
          id="name"
          name="name"
          type="text"
          required
          placeholder="Acme Logs Inc."
          className={inputClass}
        />
      </div>
      <div className="flex flex-col gap-1.5">
        <label
          htmlFor="home_region_code"
          className="text-sm font-medium text-zinc-800"
        >
          Home region
        </label>
        <select
          id="home_region_code"
          name="home_region_code"
          required
          defaultValue=""
          className={inputClass}
        >
          <option value="" disabled>
            Select a region…
          </option>
          {regions.map((r) => (
            <option key={r.id || r.code} value={r.code}>
              {r.code}
              {r.jurisdiction ? ` — ${r.jurisdiction}` : ""}
            </option>
          ))}
        </select>
      </div>
      <div className="flex gap-3 pt-2">
        <button
          type="submit"
          disabled={pending}
          className="rounded-md bg-zinc-900 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-700 disabled:opacity-50"
        >
          {pending ? "Creating…" : "Create Provider"}
        </button>
        <Link
          href="/"
          className="rounded-md border border-zinc-300 bg-white px-4 py-2 text-sm font-medium text-zinc-700 hover:bg-zinc-50"
        >
          Cancel
        </Link>
      </div>
    </form>
  );
}
