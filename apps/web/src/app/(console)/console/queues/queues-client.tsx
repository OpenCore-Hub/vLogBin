"use client";

import { useRouter } from "next/navigation";
import type {
  PlatformEvent,
  QueueOverview,
} from "@/lib/api/operator";
import { formatDate } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { ErrorState, EmptyState } from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import { ActivityIcon } from "@/components/ui/icons";

const STATUS_VARIANT: Record<string, "success" | "warning" | "danger" | "neutral" | "info"> = {
  pending: "warning",
  failed: "danger",
  dead_letter: "danger",
  published: "success",
  delivered: "success",
};

const STATUS_LABEL: Record<string, string> = {
  pending: "待处理",
  failed: "失败",
  dead_letter: "死信",
  published: "已发布",
  delivered: "已投递",
};

function count(map: Record<string, number> | undefined, key: string): number {
  return map?.[key] ?? 0;
}

export function QueuesClient({
  overview,
  loadError,
}: {
  overview: QueueOverview | null;
  loadError: string | null;
}) {
  const router = useRouter();

  if (loadError || !overview) {
    return (
      <ErrorState
        title="队列看板加载失败"
        description={loadError ?? "暂无数据"}
        action={
          <Button variant="outline" onClick={() => router.refresh()}>
            重试
          </Button>
        }
      />
    );
  }

  const summary = [
    { label: "Outbox 待处理", value: count(overview.outbox, "pending") },
    { label: "Outbox 失败", value: count(overview.outbox, "failed") },
    { label: "Outbox 死信", value: count(overview.outbox, "dead_letter") },
    { label: "Outbox 已发布", value: count(overview.outbox, "published") },
    { label: "Webhook 待处理", value: count(overview.webhook_deliveries, "pending") },
    { label: "Webhook 失败", value: count(overview.webhook_deliveries, "failed") },
    { label: "Webhook 死信", value: count(overview.webhook_deliveries, "dead_letter") },
    { label: "Webhook 已投递", value: count(overview.webhook_deliveries, "delivered") },
  ];

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">队列</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          Outbox 与 Webhook 投递容量看板，用于定位积压与死信。
        </p>
      </header>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {summary.map((s) => (
          <Card key={s.label} className="p-4">
            <p className="text-xs text-muted-foreground">{s.label}</p>
            <p className="mt-2 font-mono text-xl font-semibold tabular-nums">
              {s.value.toLocaleString("en-US")}
            </p>
          </Card>
        ))}
      </div>

      <section className="space-y-4">
        <div>
          <h2 className="text-sm font-semibold">最近 Outbox 事件</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            跨全部 Provider 的最近投递事件，用于下钻排查。
          </p>
        </div>
        <RecentOutboxTable events={overview.recent_outbox} />
      </section>
    </div>
  );
}

function RecentOutboxTable({ events }: { events: PlatformEvent[] }) {
  const columns: DataTableColumn<PlatformEvent>[] = [
    {
      key: "status",
      header: "状态",
      sortable: true,
      sortValue: (e) => e.status,
      cell: (e) => (
        <Badge variant={STATUS_VARIANT[e.status] ?? "neutral"}>
          {STATUS_LABEL[e.status] ?? e.status}
        </Badge>
      ),
    },
    {
      key: "event_type",
      header: "事件类型",
      sortable: true,
      sortValue: (e) => e.event_type,
      cell: (e) => <code className="font-mono text-xs">{e.event_type}</code>,
    },
    {
      key: "provider",
      header: "Provider",
      cell: (e) => (
        <code className="font-mono text-[11px] text-muted-foreground">
          {e.provider_id.slice(0, 8)}
        </code>
      ),
    },
    {
      key: "aggregate_type",
      header: "聚合",
      cell: (e) => (
        <span className="font-mono text-[11px] text-muted-foreground">
          {e.aggregate_type}
        </span>
      ),
    },
    {
      key: "attempts",
      header: "尝试",
      numeric: true,
      sortable: true,
      sortValue: (e) => e.attempts,
      cell: (e) => <span className="tabular-nums">{e.attempts}</span>,
    },
    {
      key: "last_error",
      header: "最后错误",
      cell: (e) => (
        <span className="line-clamp-2 max-w-md text-xs text-muted-foreground">
          {e.last_error || "—"}
        </span>
      ),
    },
    {
      key: "created_at",
      header: "创建时间",
      sortable: true,
      sortValue: (e) => e.created_at ?? "",
      cell: (e) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatDate(e.created_at)}
        </span>
      ),
    },
  ];

  return events.length === 0 ? (
    <EmptyState
      icon={<ActivityIcon size={20} aria-hidden="true" />}
      title="暂无 Outbox 事件"
      description="业务写入产生的事件会显示在这里。"
    />
  ) : (
    <DataTable
      data={events}
      columns={columns}
      rowKey={(e) => e.id}
      searchKeys={(e) => [e.event_type, e.status, e.provider_id]}
      filters={[
        {
          key: "status",
          label: "状态",
          options: [
            { value: "pending", label: "待处理" },
            { value: "failed", label: "失败" },
            { value: "published", label: "已发布" },
            { value: "dead_letter", label: "死信" },
          ],
          predicate: (e, value) => e.status === value,
        },
      ]}
      defaultSort={{ key: "created_at", dir: "desc" }}
      defaultPageSize={10}
      emptyLabel="暂无 Outbox 事件"
    />
  );
}
