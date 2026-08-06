import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/feedback";

export default function PortalLoading() {
  return (
    <main className="min-h-screen bg-canvas px-4 py-8">
      <div className="mx-auto max-w-5xl space-y-6" role="status" aria-label="门户加载中…">
        <Skeleton className="h-8 w-48" />
        <Card className="p-5">
          <Skeleton className="h-5 w-32" />
          <Skeleton className="mt-4 h-64 w-full" />
        </Card>
      </div>
    </main>
  );
}
