import Link from "next/link";
import { requireAuth } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import {
  getOverviewStats,
  listCatalogPlans,
  listCustomers,
  listInvoices,
  listProviders,
  type Invoice,
  type OverviewStats,
  type PlanCollection,
} from "@/lib/api/operator";
import { formatDate, formatMoney } from "@/lib/format";
import { Card } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { EmptyState, ErrorState } from "@/components/ui/feedback";
import { AreaChart } from "@/components/charts/area-chart";
import { Sparkline } from "@/components/charts/sparkline";
import {
  CreditCardIcon,
  LayersIcon,
  UsersIcon,
} from "@/components/ui/icons";

export const dynamic = "force-dynamic";

const EMPTY_STATS: OverviewStats = {
  published_versions: 0,
  active_subscriptions: 0,
  customers: 0,
  revenue_cents: 0,
  trends: { revenue: [], usage_events: [] },
};

const EMPTY_PLANS: PlanCollection = { plans: [], metrics: [] };

const INVOICE_STATUS: Record<string, "success" | "neutral" | "warning" | "danger"> = {
  finalized: "success",
  voided: "neutral",
  draft: "warning",
  pending: "warning",
  failed: "danger",
};

async function safeOverview(): Promise<OverviewStats> {
  try {
    return await getOverviewStats();
  } catch {
    return EMPTY_STATS;
  }
}

export default async function BillingDashboardPage() {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  const providers = await listProviders().catch(() => []);
  const provider = providers[0] ?? null;

  let overview = EMPTY_STATS;
  let invoices: Invoice[] = [];
  let customerCount = 0;
  let planCount = 0;
  let loadError: string | null = null;
  if (provider) {
    try {
      const [stats, customerRows, plans, invoiceRows] = await Promise.all([
        safeOverview(),
        listCustomers(provider.id, env).catch(() => []),
        listCatalogPlans(provider.id, env).catch(() => EMPTY_PLANS),
        listInvoices(provider.id, env).catch(() => []),
      ]);
      overview = stats;
      invoices = invoiceRows;
      customerCount = customerRows.length;
      planCount = plans.plans.length;
    } catch (err) {
      loadError = err instanceof Error ? err.message : "计费数据加载失败";
    }
  }

  const pendingCents = invoices
    .filter((i) => i.status !== "voided" && i.payment_status !== "succeeded")
    .reduce((sum, i) => sum + i.total_amount_cents, 0);
  const recent = [...invoices]
    .sort((a, b) => b.issuing_date.localeCompare(a.issuing_date))
    .slice(0, 6);

  const stats = [
    {
      label: "账单收入",
      value: formatMoney(overview.revenue_cents),
      icon: <CreditCardIcon size={18} aria-hidden="true" />,
      spark: overview.trends.revenue,
      sparkFormat: "money" as const,
    },
    {
      label: "活跃订阅",
      value: String(overview.active_subscriptions),
      icon: <LayersIcon size={18} aria-hidden="true" />,
      spark: overview.trends.usage_events,
      sparkFormat: "count" as const,
    },
    {
      label: "客户",
      value: String(customerCount),
      icon: <UsersIcon size={18} aria-hidden="true" />,
      spark: undefined,
      sparkFormat: "count" as const,
    },
    {
      label: "待收账单",
      value: formatMoney(pendingCents),
      icon: <CreditCardIcon size={18} aria-hidden="true" />,
      spark: undefined,
      sparkFormat: "money" as const,
    },
  ];

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Billing Dashboard</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          当前 {env === "test" ? "测试环境" : "生产环境"} 的收入、订阅与待收账单概览。
        </p>
      </header>

      {loadError ? (
        <ErrorState title="计费数据加载失败" description={loadError} />
      ) : !provider ? (
        <EmptyState
          icon={<CreditCardIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，再查看计费指标。"
        />
      ) : (
        <>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {stats.map((s) => (
              <Card key={s.label} className="p-4">
                <div className="flex items-center justify-between text-muted-foreground">
                  <span className="text-xs font-medium">{s.label}</span>
                  {s.icon}
                </div>
                <p className="mt-2 text-2xl font-semibold tabular-nums">
                  {s.value}
                </p>
                {s.spark ? (
                  <div className="mt-2">
                    <Sparkline data={s.spark} height={36} format={s.sparkFormat} />
                  </div>
                ) : null}
              </Card>
            ))}
          </div>

          <Card className="p-5">
            <div className="mb-3 flex items-center justify-between">
              <h2 className="text-sm font-semibold">近 30 天收入趋势</h2>
              <span className="text-xs text-muted-foreground">
                {planCount} 个套餐
              </span>
            </div>
            <AreaChart data={overview.trends.revenue} format="money" />
          </Card>

          <Card className="overflow-hidden">
            <div className="border-b border-border px-4 py-3">
              <h2 className="text-sm font-semibold">近期账单</h2>
            </div>
            {recent.length === 0 ? (
              <EmptyState
                icon={<CreditCardIcon size={20} aria-hidden="true" />}
                title="暂无账单"
                description="账单由计费引擎同步后显示在这里。"
                className="border-0 shadow-none"
              />
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="bg-surface-2/70 text-left text-xs font-medium text-muted-foreground">
                    <tr>
                      <th className="px-4 py-3 font-medium">账单号</th>
                      <th className="px-4 py-3 font-medium">客户</th>
                      <th className="px-4 py-3 font-medium">开票日期</th>
                      <th className="px-4 py-3 font-medium">状态</th>
                      <th className="px-4 py-3 text-right font-medium">金额</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border">
                    {recent.map((invoice) => (
                      <tr
                        key={invoice.id}
                        className="transition-colors hover:bg-surface-2/60"
                      >
                        <td className="px-4 py-3">
                          <Link
                            href={`/console/billing/invoices/${invoice.id}`}
                            prefetch={false}
                            className="font-mono text-xs text-brand-600 hover:underline dark:text-brand-400"
                          >
                            {invoice.number}
                          </Link>
                        </td>
                        <td className="px-4 py-3 text-xs text-muted-foreground">
                          {invoice.customer_external_id}
                        </td>
                        <td className="px-4 py-3 text-xs text-muted-foreground tabular-nums">
                          {formatDate(invoice.issuing_date)}
                        </td>
                        <td className="px-4 py-3">
                          <Badge variant={INVOICE_STATUS[invoice.status] ?? "neutral"}>
                            {invoice.status}
                          </Badge>
                        </td>
                        <td className="px-4 py-3 text-right font-mono tabular-nums">
                          {formatMoney(invoice.total_amount_cents, invoice.currency)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        </>
      )}
    </div>
  );
}
