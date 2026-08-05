"use client";

import { useEffect, useRef } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import type { Invoice } from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { formatDate, formatMoney } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { EmptyState, ErrorState } from "@/components/ui/feedback";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import { useEnv } from "@/components/console/env-provider";
import {
  ArrowRightIcon,
  CreditCardIcon,
} from "@/components/ui/icons";

const INVOICE_STATUS: Record<string, "success" | "neutral" | "warning" | "danger"> = {
  finalized: "success",
  voided: "neutral",
  draft: "warning",
  pending: "warning",
  failed: "danger",
};

export function InvoicesClient({
  providerId,
  env,
  invoices,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  invoices: Invoice[];
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

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">账单</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          查看订阅结算生成的发票与明细。金额以分存储，统一右对齐展示。
          当前环境为 {env === "test" ? "测试环境（沙箱）" : "生产环境（真实账单）"}。
        </p>
      </header>

      {loadError ? (
        <ErrorState
          title="账单列表加载失败"
          description={loadError}
          action={
            <Button variant="outline" onClick={() => router.refresh()}>
              重试
            </Button>
          }
        />
      ) : !providerId ? (
        <EmptyState
          icon={<CreditCardIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，获得测试环境后即可查看账单。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : invoices.length === 0 ? (
        <EmptyState
          icon={<CreditCardIcon size={20} aria-hidden="true" />}
          title="暂无账单"
          description="客户订阅套餐并产生结算后，账单会显示在这里。"
        />
      ) : (
        <InvoiceTable invoices={invoices} env={env} />
      )}
    </div>
  );
}

/* ---------------- 账单列表（DataTable + URL 筛选） ---------------- */
function InvoiceTable({
  invoices,
  env,
}: {
  invoices: Invoice[];
  env: Env;
}) {
  const columns: DataTableColumn<Invoice>[] = [
    {
      key: "number",
      header: "账单号",
      sortable: true,
      sortValue: (i) => i.number,
      cell: (inv) => (
        <Link
          href={`/console/billing/invoices/${inv.id}`}
          prefetch={false}
          className="group flex items-center gap-2"
        >
          <span className="font-mono text-xs text-foreground group-hover:text-brand-700 dark:group-hover:text-brand-400">
            {inv.number}
          </span>
          <ArrowRightIcon
            size={14}
            className="text-muted-foreground transition-transform group-hover:translate-x-0.5"
            aria-hidden="true"
          />
        </Link>
      ),
    },
    {
      key: "customer",
      header: "客户",
      cell: (inv) => (
        <span className="font-mono text-xs text-muted-foreground">
          {inv.customer_external_id}
        </span>
      ),
    },
    {
      key: "status",
      header: "状态",
      cell: (inv) => (
        <Badge variant={INVOICE_STATUS[inv.status] ?? "neutral"}>
          {inv.status}
        </Badge>
      ),
    },
    {
      key: "payment_status",
      header: "支付状态",
      cell: (inv) => (
        <span className="text-xs text-muted-foreground">
          {inv.payment_status}
        </span>
      ),
    },
    {
      key: "amount",
      header: "金额",
      sortable: true,
      sortValue: (i) => i.total_amount_cents,
      numeric: true,
      cell: (inv) => (
        <span className="font-semibold">
          {formatMoney(inv.total_amount_cents, inv.currency)}
        </span>
      ),
    },
    {
      key: "issuing_date",
      header: "开票日期",
      sortable: true,
      sortValue: (i) => i.issuing_date,
      cell: (inv) => (
        <span className="text-xs text-muted-foreground">
          {formatDate(inv.issuing_date)}
        </span>
      ),
    },
    {
      key: "environment",
      header: "环境",
      cell: () => <EnvBadge env={env} />,
    },
  ];

  return (
    <DataTable
      data={invoices}
      columns={columns}
      rowKey={(i) => i.id}
      searchKeys={(i) => [i.number, i.lago_id, i.customer_external_id]}
      filters={[
        {
          key: "status",
          label: "状态",
          options: [
            { value: "draft", label: "草稿" },
            { value: "finalized", label: "已出账" },
            { value: "voided", label: "已作废" },
            { value: "pending", label: "待处理" },
            { value: "failed", label: "失败" },
          ],
          predicate: (i, value) => i.status === value,
        },
      ]}
      defaultSort={{ key: "issuing_date", dir: "desc" }}
      emptyLabel="暂无账单"
    />
  );
}
