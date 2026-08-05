"use client";

import { useState } from "react";
import Link from "next/link";
import type {
  CustomerDetail,
  Invoice,
  Subscription,
  UsageEvent,
} from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { formatDate, formatDateTime, formatMoney } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { TabPanel, Tabs } from "@/components/ui/tabs";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { EmptyState, ErrorState } from "@/components/ui/feedback";
import {
  ActivityIcon,
  ArrowLeftIcon,
  CreditCardIcon,
  LayersIcon,
} from "@/components/ui/icons";

const SUB_STATUS: Record<string, "active" | "neutral" | "warning"> = {
  active: "active",
  terminated: "neutral",
};

const INVOICE_STATUS: Record<string, "success" | "neutral" | "warning" | "danger"> = {
  finalized: "success",
  voided: "neutral",
  draft: "warning",
  pending: "warning",
  failed: "danger",
};

function CountBadge({ n }: { n: number }) {
  return (
    <span className="rounded-full bg-surface-2 px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground tabular-nums">
      {n}
    </span>
  );
}

export function CustomerDetailClient({
  providerId,
  env,
  externalId,
  detail,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  externalId: string;
  detail: CustomerDetail | null;
  loadError: string | null;
}) {
  const [tab, setTab] = useState<"subscriptions" | "usage" | "invoices">(
    "subscriptions",
  );

  if (!providerId || !detail) {
    return (
      <div className="space-y-6">
        <Link
          href="/console/billing/customers"
          prefetch={false}
          className="inline-flex items-center gap-1.5 text-sm font-medium text-brand-700 hover:underline dark:text-brand-400"
        >
          <ArrowLeftIcon size={15} aria-hidden="true" />
          返回客户列表
        </Link>
        <ErrorState
          title="客户详情加载失败"
          description={
            loadError ??
            `找不到客户 ${externalId}，可能已不在当前环境。`
          }
          action={
            <Button variant="outline" onClick={() => window.location.reload()}>
              重试
            </Button>
          }
        />
      </div>
    );
  }

  const customer = detail.customer;
  return (
    <div className="space-y-6">
      <Link
        href="/console/billing/customers"
        prefetch={false}
        className="inline-flex items-center gap-1.5 text-sm font-medium text-brand-700 hover:underline dark:text-brand-400"
      >
        <ArrowLeftIcon size={15} aria-hidden="true" />
        返回客户列表
      </Link>

      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              {customer.display_name}
            </h1>
            <Badge variant={customer.account_type === "business" ? "brand" : "info"}>
              {customer.account_type === "business" ? "企业" : "个人"}
            </Badge>
            <EnvBadge env={env} />
          </div>
          <p className="mt-1 font-mono text-sm text-muted-foreground">
            {customer.external_id}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            创建于 {formatDate(customer.created_at)} ·{" "}
            {env === "test" ? "测试环境（沙箱）" : "生产环境"}
          </p>
        </div>
      </header>

      <div className="rounded-xl border border-border bg-surface-1 p-4">
        <Tabs
          value={tab}
          onChange={(v) => setTab(v as typeof tab)}
          items={[
            {
              value: "subscriptions",
              label: "订阅",
              badge: <CountBadge n={detail.subscriptions.length} />,
            },
            {
              value: "usage",
              label: "用量",
              badge: <CountBadge n={detail.usage_events.length} />,
            },
            {
              value: "invoices",
              label: "账单",
              badge: <CountBadge n={detail.invoices.length} />,
            },
          ]}
        />
        <div className="mt-4">
          <TabPanel id="customer-tabs" value="subscriptions" selected={tab === "subscriptions"}>
            <SubscriptionsTab subscriptions={detail.subscriptions} />
          </TabPanel>
          <TabPanel id="customer-tabs" value="usage" selected={tab === "usage"}>
            <UsageTab events={detail.usage_events} />
          </TabPanel>
          <TabPanel id="customer-tabs" value="invoices" selected={tab === "invoices"}>
            <InvoicesTab invoices={detail.invoices} />
          </TabPanel>
        </div>
      </div>
    </div>
  );
}

/* ---------------- 订阅 ---------------- */
function SubscriptionsTab({ subscriptions }: { subscriptions: Subscription[] }) {
  return subscriptions.length === 0 ? (
    <EmptyState
      icon={<LayersIcon size={20} aria-hidden="true" />}
      title="暂无订阅"
      description="客户订阅套餐后，会显示在这里。"
    />
  ) : (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border text-left text-xs font-medium text-muted-foreground">
            <th className="px-3 py-2 font-medium">订阅 ID</th>
            <th className="px-3 py-2 font-medium">套餐</th>
            <th className="px-3 py-2 font-medium">状态</th>
            <th className="px-3 py-2 font-medium">开始时间</th>
            <th className="px-3 py-2 font-medium">结束时间</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {subscriptions.map((s) => (
            <tr key={s.id}>
              <td className="px-3 py-2.5 font-mono text-xs">{s.external_id}</td>
              <td className="px-3 py-2.5 font-mono text-xs">{s.plan_code}</td>
              <td className="px-3 py-2.5">
                <Badge variant={SUB_STATUS[s.status] ?? "neutral"}>{s.status}</Badge>
              </td>
              <td className="px-3 py-2.5 text-xs text-muted-foreground">
                {formatDate(s.started_at)}
              </td>
              <td className="px-3 py-2.5 text-xs text-muted-foreground">
                {s.terminated_at ? formatDate(s.terminated_at) : "—"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/* ---------------- 用量 ---------------- */
function UsageTab({ events }: { events: UsageEvent[] }) {
  return events.length === 0 ? (
    <EmptyState
      icon={<ActivityIcon size={20} aria-hidden="true" />}
      title="暂无用量事件"
      description="通过 API 上报的用量事件会显示在这里。"
    />
  ) : (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border text-left text-xs font-medium text-muted-foreground">
            <th className="px-3 py-2 font-medium">事件</th>
            <th className="px-3 py-2 font-medium">类型</th>
            <th className="px-3 py-2 font-medium">指标</th>
            <th className="px-3 py-2 font-medium">事件时间</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {events.map((e) => (
            <tr key={e.id}>
              <td className="px-3 py-2.5 font-mono text-xs">{e.transaction_id}</td>
              <td className="px-3 py-2.5 text-xs text-muted-foreground">{e.kind}</td>
              <td className="px-3 py-2.5 font-mono text-xs">{e.metric_code}</td>
              <td className="px-3 py-2.5 text-xs text-muted-foreground">
                {formatDateTime(e.event_timestamp)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/* ---------------- 账单 ---------------- */
function InvoicesTab({ invoices }: { invoices: Invoice[] }) {
  return invoices.length === 0 ? (
    <EmptyState
      icon={<CreditCardIcon size={20} aria-hidden="true" />}
      title="暂无账单"
      description="订阅产生结算后，账单会显示在这里。"
    />
  ) : (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border text-left text-xs font-medium text-muted-foreground">
            <th className="px-3 py-2 font-medium">账单号</th>
            <th className="px-3 py-2 font-medium">状态</th>
            <th className="px-3 py-2 font-medium">支付状态</th>
            <th className="px-3 py-2 font-medium">金额</th>
            <th className="px-3 py-2 font-medium">开票日期</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {invoices.map((inv) => (
            <tr key={inv.id}>
              <td className="px-3 py-2.5 font-mono text-xs">{inv.number}</td>
              <td className="px-3 py-2.5">
                <Badge variant={INVOICE_STATUS[inv.status] ?? "neutral"}>
                  {inv.status}
                </Badge>
              </td>
              <td className="px-3 py-2.5 text-xs text-muted-foreground">
                {inv.payment_status}
              </td>
              <td className="px-3 py-2.5 font-semibold tabular-nums">
                {formatMoney(inv.total_amount_cents, inv.currency)}
              </td>
              <td className="px-3 py-2.5 text-xs text-muted-foreground">
                {formatDate(inv.issuing_date)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
