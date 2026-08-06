"use client";

import { useState } from "react";
import type { PortalDashboard } from "@/lib/api/types";
import { formatDate, formatMoney } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { EmptyState, ErrorState, InfoNote } from "@/components/ui/feedback";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import { Card } from "@/components/ui/card";
import { Tabs, TabPanel } from "@/components/ui/tabs";
import {
  CreditCardIcon,
  ActivityIcon,
  LogoutIcon,
} from "@/components/ui/icons";
import { logoutPortalAction } from "./actions";

const INVOICE_STATUS: Record<string, "success" | "neutral" | "warning" | "danger"> = {
  finalized: "success",
  voided: "neutral",
  draft: "warning",
  pending: "warning",
  failed: "danger",
};

const PAYMENT_STATUS: Record<string, "success" | "warning" | "danger" | "neutral"> = {
  succeeded: "success",
  pending: "warning",
  failed: "danger",
};

export function PortalClient({
  dashboard,
  loadError,
}: {
  dashboard: PortalDashboard | null;
  loadError: string | null;
}) {
  const [tab, setTab] = useState("invoices");

  if (loadError || !dashboard) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-canvas px-4">
        <ErrorState
          title="门户加载失败"
          description={loadError ?? "未找到客户数据"}
          action={
            <form action={logoutPortalAction}>
              <Button type="submit" variant="outline">
                返回登录
              </Button>
            </form>
          }
        />
      </main>
    );
  }

  return (
    <main className="min-h-screen bg-canvas">
      <header className="sticky top-0 z-30 flex h-14 items-center justify-between border-b border-border bg-canvas/85 px-4 backdrop-blur sm:px-6">
        <div className="flex items-center gap-2">
          <span className="size-2 rounded-full bg-brand-600" aria-hidden="true" />
          <span className="text-sm font-semibold">{dashboard.provider_name}</span>
          <span className="font-mono text-xs text-muted-foreground">
            @{dashboard.provider_slug}
          </span>
        </div>
        <form action={logoutPortalAction}>
          <Button variant="ghost" size="sm" type="submit">
            <LogoutIcon size={14} aria-hidden="true" />
            退出
          </Button>
        </form>
      </header>

      <div className="mx-auto max-w-5xl space-y-6 px-4 py-8 sm:px-6">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {dashboard.customer.display_name}
          </h1>
          <p className="mt-1 font-mono text-sm text-muted-foreground">
            {dashboard.customer.external_id}
          </p>
        </div>

        <Tabs
          value={tab}
          onChange={setTab}
          items={[
            { value: "invoices", label: "账单" },
            { value: "usage", label: "用量" },
            { value: "payment", label: "支付" },
          ]}
        />

        <TabPanel id="portal" value="invoices" selected={tab === "invoices"}>
          <InvoiceTab dashboard={dashboard} />
        </TabPanel>
        <TabPanel id="portal" value="usage" selected={tab === "usage"}>
          <UsageTab dashboard={dashboard} />
        </TabPanel>
        <TabPanel id="portal" value="payment" selected={tab === "payment"}>
          <PaymentTab dashboard={dashboard} />
        </TabPanel>
      </div>
    </main>
  );
}

function InvoiceTab({ dashboard }: { dashboard: PortalDashboard }) {
  const columns: DataTableColumn<(typeof dashboard.invoices)[number]>[] = [
    {
      key: "number",
      header: "账单号",
      sortable: true,
      sortValue: (i) => i.number,
      cell: (i) => <code className="font-mono text-xs text-foreground">{i.number}</code>,
    },
    {
      key: "issuing_date",
      header: "开票日期",
      sortable: true,
      sortValue: (i) => i.issuing_date,
      cell: (i) => <span className="text-xs text-muted-foreground">{formatDate(i.issuing_date)}</span>,
    },
    {
      key: "status",
      header: "状态",
      cell: (i) => <Badge variant={INVOICE_STATUS[i.status] ?? "neutral"}>{i.status}</Badge>,
    },
    {
      key: "amount",
      header: "金额",
      numeric: true,
      sortable: true,
      sortValue: (i) => i.total_amount_cents,
      cell: (i) => <span className="font-semibold">{formatMoney(i.total_amount_cents, i.currency)}</span>,
    },
  ];

  return dashboard.invoices.length === 0 ? (
    <EmptyState
      icon={<CreditCardIcon size={20} aria-hidden="true" />}
      title="暂无账单"
      description="订阅结算生成后，账单会显示在这里。"
    />
  ) : (
    <DataTable
      data={dashboard.invoices}
      columns={columns}
      rowKey={(i) => i.id}
      searchKeys={(i) => [i.number, i.status]}
      defaultSort={{ key: "issuing_date", dir: "desc" }}
      emptyLabel="暂无账单"
    />
  );
}

function UsageTab({ dashboard }: { dashboard: PortalDashboard }) {
  const columns: DataTableColumn<(typeof dashboard.usage_events)[number]>[] = [
    {
      key: "metric",
      header: "指标",
      cell: (e) => <code className="font-mono text-xs text-foreground">{e.metric_code}</code>,
    },
    {
      key: "transaction",
      header: "事务 ID",
      cell: (e) => <code className="font-mono text-[11px] text-muted-foreground">{e.transaction_id}</code>,
    },
    {
      key: "timestamp",
      header: "发生时间",
      sortable: true,
      sortValue: (e) => e.event_timestamp ?? "",
      cell: (e) => <span className="text-xs text-muted-foreground">{formatDate(e.event_timestamp)}</span>,
    },
  ];

  return dashboard.usage_events.length === 0 ? (
    <EmptyState
      icon={<ActivityIcon size={20} aria-hidden="true" />}
      title="暂无用量事件"
      description="使用产品产生计量后，事件会显示在这里。"
    />
  ) : (
    <DataTable
      data={dashboard.usage_events}
      columns={columns}
      rowKey={(e) => e.id}
      searchKeys={(e) => [e.metric_code, e.transaction_id]}
      defaultSort={{ key: "timestamp", dir: "desc" }}
      emptyLabel="暂无用量"
    />
  );
}

function PaymentTab({ dashboard }: { dashboard: PortalDashboard }) {
  const invoices = dashboard.invoices;
  const succeeded = invoices
    .filter((i) => i.payment_status === "succeeded")
    .reduce((sum, i) => sum + i.total_amount_cents, 0);
  const pending = invoices
    .filter((i) => i.status !== "voided" && i.payment_status === "pending")
    .reduce((sum, i) => sum + i.total_amount_cents, 0);
  const failed = invoices
    .filter((i) => i.status !== "voided" && i.payment_status === "failed")
    .reduce((sum, i) => sum + i.total_amount_cents, 0);

  const summary = [
    { label: "已支付", value: formatMoney(succeeded) },
    { label: "待支付", value: formatMoney(pending) },
    { label: "支付失败", value: formatMoney(failed) },
  ];

  const columns: DataTableColumn<(typeof invoices)[number]>[] = [
    {
      key: "number",
      header: "账单号",
      sortable: true,
      sortValue: (i) => i.number,
      cell: (i) => (
        <code className="font-mono text-xs text-foreground">{i.number}</code>
      ),
    },
    {
      key: "issuing_date",
      header: "开票日期",
      sortable: true,
      sortValue: (i) => i.issuing_date,
      cell: (i) => (
        <span className="text-xs text-muted-foreground">
          {formatDate(i.issuing_date)}
        </span>
      ),
    },
    {
      key: "payment_status",
      header: "支付状态",
      cell: (i) => (
        <Badge variant={PAYMENT_STATUS[i.payment_status] ?? "neutral"}>
          {i.payment_status}
        </Badge>
      ),
    },
    {
      key: "amount",
      header: "金额",
      numeric: true,
      sortable: true,
      sortValue: (i) => i.total_amount_cents,
      cell: (i) => (
        <span className="font-semibold">
          {formatMoney(i.total_amount_cents, i.currency)}
        </span>
      ),
    },
  ];

  return (
    <div className="space-y-4">
      {invoices.length === 0 ? (
        <EmptyState
          icon={<CreditCardIcon size={20} aria-hidden="true" />}
          title="暂无支付记录"
          description="订阅结算生成后，支付状态会显示在这里。"
        />
      ) : (
        <>
          <div className="grid gap-4 sm:grid-cols-3">
            {summary.map((s) => (
              <Card key={s.label} className="p-4">
                <p className="text-xs text-muted-foreground">{s.label}</p>
                <p className="mt-2 font-mono text-xl font-semibold tabular-nums">
                  {s.value}
                </p>
              </Card>
            ))}
          </div>
          <DataTable
            data={invoices}
            columns={columns}
            rowKey={(i) => i.id}
            searchKeys={(i) => [i.number, i.payment_status]}
            defaultSort={{ key: "issuing_date", dir: "desc" }}
            emptyLabel="暂无支付记录"
          />
        </>
      )}
      <InfoNote>
        该 workspace 尚未接入支付渠道；配置后此处可管理支付方式与历史。
      </InfoNote>
    </div>
  );
}
