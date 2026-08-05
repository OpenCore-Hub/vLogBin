import { Skeleton } from "@/components/ui/feedback";

export default function Loading() {
  return (
    <div className="space-y-4" role="status" aria-label="加载中…">
      <Skeleton className="h-7 w-48" />
      <Skeleton className="h-64 rounded-lg" />
    </div>
  );
}
