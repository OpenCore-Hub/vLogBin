import { requireAuth } from "@/lib/auth/rbac";
import {
  getQueueOverview,
  type QueueOverview,
} from "@/lib/api/operator";
import { QueuesClient } from "./queues-client";

export const dynamic = "force-dynamic";

export default async function QueuesPage() {
  await requireAuth();

  let overview: QueueOverview | null = null;
  let loadError: string | null = null;
  try {
    overview = await getQueueOverview();
  } catch (err) {
    loadError = err instanceof Error ? err.message : "队列数据加载失败";
  }

  return <QueuesClient overview={overview} loadError={loadError} />;
}
