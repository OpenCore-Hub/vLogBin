"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import type { AnalyticsDashboard } from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { formatMoney } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { EmptyState, ErrorState } from "@/components/ui/feedback";
import { Card } from "@/components/ui/card";
import { useEnv } from "@/components/console/env-provider";
import {
  ActivityIcon,
  ArrowRightIcon,
} from "@/components/ui/icons";

export function AnalyticsClient({
  providerId,
  env,
  dashboard,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  dashboard: AnalyticsDashboard;
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

  const revenue = dashboard.revenue.reduce(
    (sum, r) => sum + r.total_revenue_cents,
    0,
  );
  const mau = dashboard.mau.reduce(
    (sum, r) => sum + r.active_customers,
    0,
  );
  const anomalies = dashboard.anomalies.filter((a) => a.is_anomaly).length;
  const churn = dashboard.churn.reduce(
    (sum, r) => sum + r.churned_subscriptions,
    0,
  );

  const summary = [
    { label: "收入", value: formatMoney(revenue) },
    { label: "活跃客户", value: String(mau) },
    { label: "用量异常", value: String(anomalies) },
    { label: "流失订阅", value: String(churn) },
  ];

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Analytics</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          收入、MAU、转化、流失与用量异常汇总。当前环境：{env === "test" ? "测试" : "生产"}。
        </p>
      </header>

      {loadError ? (
        <ErrorState
          title="分析数据加载失败"
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
          description="先创建并激活 Provider，再查看分析数据。"
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

          {dashboard.revenue.length === 0 &&
          dashboard.mau.length === 0 &&
          dashboard.conversion.length === 0 &&
          dashboard.churn.length === 0 &&
          dashboard.anomalies.length === 0 ? (
            <EmptyState
              icon={<ActivityIcon size={20} aria-hidden="true" />}
              title="暂无分析数据"
              description="分析平面生成月度汇总后，这里会显示收入、MAU、转化与流失趋势。"
            />
          ) : (
            <>
              <AnalyticsTable
                title="月度收入"
                head={["月份", "发票数", "订阅数", "收入"]}
                rows={dashboard.revenue.map((r) => [
                  r.month,
                  String(r.invoice_count),
                  String(r.subscription_count),
                  formatMoney(r.total_revenue_cents),
                ])}
              />
              <AnalyticsTable
                title="MAU"
                head={["月份", "活跃客户", "指标数", "用量事件"]}
                rows={dashboard.mau.map((r) => [
                  r.month,
                  String(r.active_customers),
                  String(r.unique_metrics),
                  String(r.total_usage_events),
                ])}
              />
              <AnalyticsTable
                title="转化"
                head={["月份", "新客户", "有订阅客户", "活跃订阅"]}
                rows={dashboard.conversion.map((r) => [
                  r.signup_month,
                  String(r.new_customers),
                  String(r.customers_with_subscription),
                  String(r.active_subscriptions),
                ])}
              />
              <AnalyticsTable
                title="流失"
                head={["月份", "流失订阅", "留存订阅"]}
                rows={dashboard.churn.map((r) => [
                  r.churn_month,
                  String(r.churned_subscriptions),
                  String(r.retained_subscriptions),
                ])}
              />
              <AnalyticsTable
                title="用量异常"
                head={["指标", "日期", "事件数", "7 日均值", "状态"]}
                rows={dashboard.anomalies.map((r) => [
                  r.metric_code,
                  r.day,
                  String(r.event_count),
                  String(r.avg_7d),
                  r.is_anomaly ? "异常" : "正常",
                ])}
              />
            </>
          )}
        </>
      )}
    </div>
  );
}

function AnalyticsTable({
  title,
  head,
  rows,
}: {
  title: string;
  head: string[];
  rows: string[][];
}) {
  if (rows.length === 0) return null;
  return (
    <Card className="overflow-hidden">
      <div className="border-b border-border px-4 py-3">
        <h2 className="text-sm font-semibold">{title}</h2>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead className="bg-surface-2/70 text-left text-xs font-medium text-muted-foreground">
            <tr>
              {head.map((h) => (
                <th key={h} className="px-4 py-3 font-medium">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {rows.map((row, i) => (
              <tr key={i} className="transition-colors hover:bg-surface-2/60">
                {row.map((cell, j) => (
                  <td key={j} className="px-4 py-3 font-mono text-xs">
                    {cell}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  );
}
