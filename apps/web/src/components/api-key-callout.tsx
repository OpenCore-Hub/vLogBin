"use client";

import { useState } from "react";

/**
 * Displays a one-time plaintext API key with a copy button.
 * The key is only ever returned once by the API — warn the operator to store it.
 */
export function ApiKeyCallout({
  apiKey,
  title = "API key",
}: {
  apiKey: string;
  title?: string;
}) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(apiKey);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard unavailable (e.g. non-secure context) — leave the key selectable.
    }
  }

  return (
    <div
      role="alert"
      className="rounded-md border border-amber-300 bg-amber-50 p-4"
    >
      <p className="text-sm font-semibold text-amber-900">{title}</p>
      <p className="mt-1 text-sm text-amber-800">
        This key is shown once. Copy and store it now — it cannot be retrieved
        again.
      </p>
      <div className="mt-3 flex items-center gap-2">
        <code className="flex-1 overflow-x-auto rounded border border-amber-200 bg-white px-3 py-2 font-mono text-sm text-zinc-900 select-all">
          {apiKey}
        </code>
        <button
          type="button"
          onClick={copy}
          className="shrink-0 rounded-md border border-amber-300 bg-white px-3 py-2 text-sm font-medium text-amber-900 hover:bg-amber-100"
        >
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
    </div>
  );
}
