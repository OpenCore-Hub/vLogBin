import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/feedback";

export default function ConsoleLoading() {
  return (
    <div className="space-y-6" role="status" aria-label="控制台加载中…">
      <Skeleton className="h-8 w-48" />
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Card key={i} className="p-4">
            <Skeleton className="h-4 w-24" />
            <Skeleton className="mt-3 h-8 w-20" />
          </Card>
        ))}
      </div>
      <Card className="p-5">
        <Skeleton className="h-5 w-40" />
        <Skeleton className="mt-4 h-48 w-full" />
      </Card>
    </div>
  );
}
