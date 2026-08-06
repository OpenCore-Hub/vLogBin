"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import type { Subscription } from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { formatDate } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { EmptyState, ErrorState } from "@/components/ui/feedback";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import { useEnv } from "@/components/console/env-provider";
import {
  ArrowRightIcon,
  LayersIcon,
} from "@/components/ui/icons";

export function SubscriptionsClient({
  providerId,
  env,
  subscriptions,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  subscriptions: Subscription[];
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

  const active = subscriptions.filter((s) => s.status === "active").length;
  const terminated = subscriptions.length - active;
  const customers = new Set(subscriptions.map((s) => s.customer_external_id));

  const summary = [
    { label: "订阅数", value: String(subscriptions.length) },
    { label: "活跃", value: String(active) },
    { label: "已终止", value: String(terminated) },
    { label: "客户数", value: String(customers.size) },
  ];

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">订阅</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          查看当前 workspace 的订阅状态与计划归属。
          当前环境为 {env === "test" ? "测试环境（沙箱）" : "生产环境"}。
        </p>
      </header>

      {loadError ? (
        <ErrorState
          title="订阅列表加载失败"
          description={loadError}
          action={
            <Button variant="outline" onClick={() => router.refresh()}>
              重试
            </Button>
          }
        />
      ) : !providerId ? (
        <EmptyState
          icon={<LayersIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，再查看订阅。"
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

          {subscriptions.length === 0 ? (
            <EmptyState
              icon={<LayersIcon size={20} aria-hidden="true" />}
              title="暂无订阅"
              description="客户订阅套餐后，订阅会显示在这里。"
            />
          ) : (
            <SubscriptionTable subscriptions={subscriptions} env={env} />
          )}
        </>
      )}
    </div>
  );
}

function SubscriptionTable({
  subscriptions,
  env,
}: {
  subscriptions: Subscription[];
  env: Env;
}) {
  const columns: DataTableColumn<Subscription>[] = [
    {
      key: "external_id",
      header: "订阅 ID",
      sortable: true,
      sortValue: (s) => s.external_id,
      cell: (s) => (
        <code className="font-mono text-xs text-foreground">
          {s.external_id}
        </code>
      ),
    },
    {
      key: "customer",
      header: "客户",
      cell: (s) => (
        <span className="font-mono text-xs text-muted-foreground">
          {s.customer_external_id}
        </span>
      ),
    },
    {
      key: "plan",
      header: "套餐",
      cell: (s) => (
        <code className="font-mono text-xs">{s.plan_code}</code>
      ),
    },
    {
      key: "status",
      header: "状态",
      cell: (s) => (
        <Badge variant={s.status === "active" ? "success" : "neutral"}>
          {s.status}
        </Badge>
      ),
    },
    {
      key: "environment",
      header: "环境",
      cell: () => <EnvBadge env={env} />,
    },
    {
      key: "started_at",
      header: "开始时间",
      sortable: true,
      sortValue: (s) => s.started_at ?? "",
      cell: (s) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatDate(s.started_at)}
        </span>
      ),
    },
    {
      key: "terminated_at",
      header: "终止时间",
      sortable: true,
      sortValue: (s) => s.terminated_at ?? "",
      cell: (s) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatDate(s.terminated_at)}
        </span>
      ),
    },
  ];

  return (
    <DataTable
      data={subscriptions}
      columns={columns}
      rowKey={(s) => s.id}
      searchKeys={(s) => [
        s.external_id,
        s.customer_external_id,
        s.plan_code,
      ]}
      filters={[
        {
          key: "status",
          label: "状态",
          options: [
            { value: "active", label: "活跃" },
            { value: "terminated", label: "已终止" },
          ],
          predicate: (s, value) => s.status === value,
        },
      ]}
      defaultSort={{ key: "started_at", dir: "desc" }}
      emptyLabel="暂无订阅"
    />
  );
}
