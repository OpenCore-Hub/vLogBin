"use client";

import { useEffect, useRef } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import type { Invoice } from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { formatDate, formatMoney } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { EmptyState, ErrorState, InfoNote } from "@/components/ui/feedback";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import { useEnv } from "@/components/console/env-provider";
import {
  ArrowRightIcon,
  CreditCardIcon,
} from "@/components/ui/icons";

const PAYMENT_STATUS: Record<string, "success" | "neutral" | "warning" | "danger"> = {
  succeeded: "success",
  pending: "warning",
  failed: "danger",
};

const INVOICE_STATUS: Record<string, "success" | "neutral" | "warning" | "danger"> = {
  finalized: "success",
  voided: "neutral",
  draft: "warning",
  pending: "warning",
  failed: "danger",
};

export function PaymentsClient({
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

  const sumBy = (predicate: (i: Invoice) => boolean) =>
    invoices
      .filter(predicate)
      .reduce((sum, i) => sum + i.total_amount_cents, 0);

  const succeeded = sumBy((i) => i.payment_status === "succeeded");
  const pending = sumBy(
    (i) => i.status !== "voided" && i.payment_status === "pending",
  );
  const failed = sumBy(
    (i) => i.status !== "voided" && i.payment_status === "failed",
  );

  const summary = [
    { label: "支付成功", value: formatMoney(succeeded) },
    { label: "待支付", value: formatMoney(pending) },
    { label: "支付失败", value: formatMoney(failed) },
    { label: "发票数", value: String(invoices.length) },
  ];

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">支付</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          按发票支付状态追踪收款：成功、待支付与失败金额。
          当前环境为 {env === "test" ? "测试环境（沙箱）" : "生产环境（真实支付）"}。
        </p>
      </header>

      {loadError ? (
        <ErrorState
          title="支付数据加载失败"
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
          description="先创建并激活 Provider，再查看支付与收款状态。"
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

          <InfoNote>
            本地不保存支付方式与支付渠道凭据；支付状态由 Lago 发票同步维护。
          </InfoNote>

          {invoices.length === 0 ? (
            <EmptyState
              icon={<CreditCardIcon size={20} aria-hidden="true" />}
              title="暂无账单"
              description="客户订阅并产生结算后，支付状态会显示在这里。"
            />
          ) : (
            <PaymentTable invoices={invoices} env={env} />
          )}
        </>
      )}
    </div>
  );
}

function PaymentTable({
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
      header: "账单状态",
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
        <Badge variant={PAYMENT_STATUS[inv.payment_status] ?? "neutral"}>
          {inv.payment_status}
        </Badge>
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
          key: "payment_status",
          label: "支付状态",
          options: [
            { value: "succeeded", label: "已支付" },
            { value: "pending", label: "待支付" },
            { value: "failed", label: "支付失败" },
          ],
          predicate: (i, value) => i.payment_status === value,
        },
      ]}
      defaultSort={{ key: "issuing_date", dir: "desc" }}
      emptyLabel="暂无账单"
    />
  );
}
