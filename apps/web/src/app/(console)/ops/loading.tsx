import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/feedback";

export default function OpsLoading() {
  return (
    <div className="space-y-6" role="status" aria-label="运营商台加载中…">
      <Skeleton className="h-8 w-52" />
      <Card className="p-5">
        <Skeleton className="h-5 w-32" />
        <Skeleton className="mt-4 h-56 w-full" />
      </Card>
    </div>
  );
}
