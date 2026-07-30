export default function Loading() {
  return (
    <div className="flex flex-col gap-4">
      <div className="h-7 w-64 animate-pulse rounded bg-zinc-200" />
      <div className="h-40 animate-pulse rounded-md border border-zinc-200 bg-zinc-100" />
      <div className="h-40 animate-pulse rounded-md border border-zinc-200 bg-zinc-100" />
    </div>
  );
}
