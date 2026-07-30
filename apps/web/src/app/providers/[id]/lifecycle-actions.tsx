"use client";

import { useActionState } from "react";
import { lifecycleAction, initialActionState } from "@/app/actions";
import { ApiKeyCallout } from "@/components/api-key-callout";

const ACTIONS: { to: string; label: string; className: string }[] = [
  {
    to: "LIVE_REVIEW",
    label: "Request Live Review",
    className:
      "border-amber-300 bg-amber-50 text-amber-900 hover:bg-amber-100",
  },
  {
    to: "LIVE_ACTIVE",
    label: "Activate Live",
    className:
      "border-emerald-300 bg-emerald-50 text-emerald-900 hover:bg-emerald-100",
  },
  {
    to: "RESTRICTED",
    label: "Restrict",
    className:
      "border-orange-300 bg-orange-50 text-orange-900 hover:bg-orange-100",
  },
  {
    to: "SUSPENDED",
    label: "Suspend",
    className: "border-red-300 bg-red-50 text-red-900 hover:bg-red-100",
  },
];

export function LifecycleActions({
  providerId,
  currentState,
}: {
  providerId: string;
  currentState: string;
}) {
  const [state, formAction, pending] = useActionState(
    lifecycleAction,
    initialActionState,
  );

  return (
    <div className="flex flex-col gap-3">
      <form action={formAction} className="flex flex-wrap gap-2">
        <input type="hidden" name="provider_id" value={providerId} />
        {ACTIONS.map((a) => (
          <button
            key={a.to}
            type="submit"
            name="to"
            value={a.to}
            disabled={pending || currentState === a.to}
            className={`rounded-md border px-3 py-1.5 text-sm font-medium disabled:cursor-not-allowed disabled:opacity-40 ${a.className}`}
          >
            {pending ? "Working…" : a.label}
          </button>
        ))}
      </form>

      {state.error && (
        <div
          role="alert"
          className="rounded-md border border-red-300 bg-red-50 p-4 text-sm text-red-800"
        >
          {state.error}
        </div>
      )}
      {state.ok && !state.apiKey && (
        <div className="rounded-md border border-emerald-300 bg-emerald-50 p-3 text-sm text-emerald-800">
          Lifecycle transition applied.
        </div>
      )}
      {state.ok && state.apiKey && (
        <ApiKeyCallout apiKey={state.apiKey} title="Live API key" />
      )}
    </div>
  );
}
