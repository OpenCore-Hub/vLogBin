"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import type { UsageEvent } from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { formatDateTime } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { EmptyState, ErrorState } from "@/components/ui/feedback";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import { useEnv } from "@/components/console/env-provider";
import {
  ActivityIcon,
  ArrowRightIcon,
} from "@/components/ui/icons";

export function UsageClient({
  providerId,
  env,
  events,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  events: UsageEvent[];
  loadError: string | null;
}) {
  const router = useRouter();
  const { env: activeEnv } = useEnv();
  const prevEnv = useRef(env);

  useEffect(() => {
    if (prevEnv.current !== activeEnv) {
      prevEnv.current = activeEnv;
      router.refresh();
    }
  }, [activeEnv, router]);

  const customers = new Set(events.map((e) => e.customer_external_id));
  const metrics = new Set(events.map((e) => e.metric_code));
  const reversals = events.filter((e) => e.kind === "reversal").length;

  const summary = [
    { label: "事件数", value: String(events.length) },
    { label: "客户数", value: String(customers.size) },
    { label: "指标数", value: String(metrics.size) },
    { label: "撤销事件", value: String(reversals) },
  ];

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">用量</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          查看当前 workspace 的用量事件流，按事务、客户与指标检索。
        </p>
      </header>

      {loadError ? (
        <ErrorState
          title="用量数据加载失败"
          description={loadError}
          action={
            <Button variant="outline" onClick={() => router.refresh()}>
              重试
            </Button>
          }
        />
      ) : !providerId ? (
        <EmptyState
          icon={<ActivityIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，再查看用量事件。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : (
        <>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {summary.map((s) => (
              <Card key={s.label} className="p-4">
                <p className="text-xs text-muted-foreground">{s.label}</p>
                <p className="mt-2 font-mono text-xl font-semibold tabular-nums">
                  {s.value}
                </p>
              </Card>
            ))}
          </div>

          {events.length === 0 ? (
            <EmptyState
              icon={<ActivityIcon size={20} aria-hidden="true" />}
              title="暂无用量事件"
              description="客户消费产生计量后，事件会显示在这里。"
            />
          ) : (
            <UsageTable events={events} />
          )}
        </>
      )}
    </div>
  );
}

function UsageTable({ events }: { events: UsageEvent[] }) {
  const columns: DataTableColumn<UsageEvent>[] = [
    {
      key: "transaction_id",
      header: "事务 ID",
      sortable: true,
      sortValue: (e) => e.transaction_id,
      cell: (e) => (
        <code className="font-mono text-xs text-foreground">
          {e.transaction_id}
        </code>
      ),
    },
    {
      key: "customer",
      header: "客户",
      cell: (e) => (
        <span className="font-mono text-xs text-muted-foreground">
          {e.customer_external_id}
        </span>
      ),
    },
    {
      key: "metric",
      header: "指标",
      cell: (e) => (
        <code className="font-mono text-xs">{e.metric_code}</code>
      ),
    },
    {
      key: "kind",
      header: "类型",
      cell: (e) => (
        <Badge variant={e.kind === "reversal" ? "warning" : "success"}>
          {e.kind}
        </Badge>
      ),
    },
    {
      key: "environment",
      header: "环境",
      cell: (e) => <EnvBadge env={e.environment_kind} />,
    },
    {
      key: "event_timestamp",
      header: "发生时间",
      sortable: true,
      sortValue: (e) => e.event_timestamp ?? "",
      cell: (e) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatDateTime(e.event_timestamp)}
        </span>
      ),
    },
    {
      key: "created_at",
      header: "入库时间",
      sortable: true,
      sortValue: (e) => e.created_at ?? "",
      cell: (e) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatDateTime(e.created_at)}
        </span>
      ),
    },
  ];

  return (
    <DataTable
      data={events}
      columns={columns}
      rowKey={(e) => e.id}
      searchKeys={(e) => [
        e.transaction_id,
        e.customer_external_id,
        e.metric_code,
      ]}
      defaultSort={{ key: "event_timestamp", dir: "desc" }}
      emptyLabel="暂无用量"
    />
  );
}
