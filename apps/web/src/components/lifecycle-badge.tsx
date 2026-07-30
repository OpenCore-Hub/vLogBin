const STATE_STYLES: Record<string, string> = {
  DRAFT: "bg-zinc-100 text-zinc-700 ring-zinc-300",
  ONBOARDING: "bg-sky-100 text-sky-800 ring-sky-300",
  TEST: "bg-sky-100 text-sky-800 ring-sky-300",
  LIVE_REVIEW: "bg-amber-100 text-amber-800 ring-amber-300",
  LIVE_ACTIVE: "bg-emerald-100 text-emerald-800 ring-emerald-300",
  RESTRICTED: "bg-orange-100 text-orange-800 ring-orange-300",
  SUSPENDED: "bg-red-100 text-red-800 ring-red-300",
};

export function LifecycleBadge({ state }: { state: string }) {
  const styles =
    STATE_STYLES[state] ?? "bg-zinc-100 text-zinc-700 ring-zinc-300";
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${styles}`}
    >
      {state || "UNKNOWN"}
    </span>
  );
}
