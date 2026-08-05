"use client";

import Link from "next/link";
import type { InvoiceDetail } from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { formatDate, formatMoney } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { ErrorState, EmptyState } from "@/components/ui/feedback";
import { ArrowLeftIcon, CreditCardIcon } from "@/components/ui/icons";

const INVOICE_STATUS: Record<string, "success" | "neutral" | "warning" | "danger"> = {
  finalized: "success",
  voided: "neutral",
  draft: "warning",
  pending: "warning",
  failed: "danger",
};

export function InvoiceDetailClient({
  providerId,
  env,
  invoiceId,
  detail,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  invoiceId: string;
  detail: InvoiceDetail | null;
  loadError: string | null;
}) {
  if (!providerId || !detail) {
    return (
      <div className="space-y-6">
        <Link
          href="/console/billing/invoices"
          prefetch={false}
          className="inline-flex items-center gap-1.5 text-sm font-medium text-brand-700 hover:underline dark:text-brand-400"
        >
          <ArrowLeftIcon size={15} aria-hidden="true" />
          返回账单列表
        </Link>
        <ErrorState
          title="账单详情加载失败"
          description={
            loadError ?? `找不到账单 ${invoiceId}，可能已不在当前环境。`
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

  const inv = detail.invoice;
  return (
    <div className="space-y-6">
      <Link
        href="/console/billing/invoices"
        prefetch={false}
        className="inline-flex items-center gap-1.5 text-sm font-medium text-brand-700 hover:underline dark:text-brand-400"
      >
        <ArrowLeftIcon size={15} aria-hidden="true" />
        返回账单列表
      </Link>

      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="font-mono text-2xl font-semibold tracking-tight">
              {inv.number}
            </h1>
            <Badge variant={INVOICE_STATUS[inv.status] ?? "neutral"}>
              {inv.status}
            </Badge>
            <Badge variant="info">{inv.payment_status}</Badge>
            <EnvBadge env={env} />
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            客户 <span className="font-mono">{inv.customer_external_id}</span> ·{" "}
            开票日期 {formatDate(inv.issuing_date)}
          </p>
        </div>
        <div className="text-right">
          <p className="text-xs text-muted-foreground">合计</p>
          <p className="font-mono text-2xl font-semibold tabular-nums">
            {formatMoney(inv.total_amount_cents, inv.currency)}
          </p>
        </div>
      </header>

      <div className="rounded-xl border border-border bg-surface-1">
        {detail.lines.length === 0 ? (
          <div className="p-4">
            <EmptyState
              icon={<CreditCardIcon size={20} aria-hidden="true" />}
              title="暂无行明细"
              description="账单同步后，明细行会显示在这里。"
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border bg-surface-2 text-left text-xs font-medium text-muted-foreground">
                  <th className="px-4 py-3 font-medium">项目</th>
                  <th className="px-4 py-3 font-medium">指标</th>
                  <th className="px-4 py-3 font-medium">数量</th>
                  <th className="px-4 py-3 font-medium">单价</th>
                  <th className="px-4 py-3 font-medium">金额</th>
                  <th className="px-4 py-3 font-medium">税额</th>
                  <th className="px-4 py-3 font-medium">合计</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {detail.lines.map((line) => (
                  <tr key={line.id}>
                    <td className="px-4 py-3 font-medium">{line.item_name}</td>
                    <td className="px-4 py-3 font-mono text-xs">
                      {line.metric_code || "—"}
                    </td>
                    <td className="px-4 py-3 font-mono text-xs tabular-nums">
                      {line.units}
                    </td>
                    <td className="px-4 py-3 font-mono text-xs tabular-nums">
                      {line.precise_unit_amount}
                    </td>
                    <td className="px-4 py-3 font-mono text-xs tabular-nums">
                      {formatMoney(line.amount_cents, line.currency)}
                    </td>
                    <td className="px-4 py-3 font-mono text-xs tabular-nums">
                      {formatMoney(line.taxes_amount_cents, line.currency)}
                    </td>
                    <td className="px-4 py-3 font-mono font-semibold tabular-nums">
                      {formatMoney(line.total_amount_cents, line.currency)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
