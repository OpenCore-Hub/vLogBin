import Link from "next/link";
import { requireAuth } from "@/lib/auth/rbac";
import { listProviders, getOverviewStats } from "@/lib/api/operator";
import type { OverviewStats, Provider } from "@/lib/api/operator";
import { getOnboardingState, type OnboardingState } from "@/lib/onboarding";
import { formatMoney, formatDate } from "@/lib/format";
import { LifecycleBadge } from "@/components/ui/badge";
import { LinkButton } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { AreaChart } from "@/components/charts/area-chart";
import { BarChart } from "@/components/charts/bar-chart";
import {
  ArrowRightIcon,
  BoxIcon,
  CheckIcon,
  CreditCardIcon,
  LayersIcon,
  LogoMark,
  PlusIcon,
  UsersIcon,
} from "@/components/ui/icons";

export const dynamic = "force-dynamic";

async function safeList<T>(fn: () => Promise<T[]>): Promise<T[]> {
  try {
    return await fn();
  } catch {
    return [];
  }
}

const EMPTY_STATS: OverviewStats = {
  published_versions: 0,
  active_subscriptions: 0,
  customers: 0,
  revenue_cents: 0,
  trends: { revenue: [], usage_events: [] },
};

async function safeGetOverviewStats() {
  try {
    return await getOverviewStats();
  } catch {
    return EMPTY_STATS;
  }
}

export default async function ConsolePage() {
  const session = await requireAuth();
  const providers = await safeList(() => listProviders());

  let publishedVersions = 0;
  let activeSubscriptions = 0;
  let customers = 0;
  let revenueCents = 0;

  // R29：概览统计改为单请求聚合接口（API 端 SQL 聚合），
  // 彻底消除 web 端 N+1 扇出（一次 200 请求打爆 credential 限流桶的历史问题）。
  const overview = await safeGetOverviewStats();
  publishedVersions = overview.published_versions;
  activeSubscriptions = overview.active_subscriptions;
  customers = overview.customers;
  revenueCents = overview.revenue_cents;

  const liveActiveCount = providers.filter(
    (p) => p.lifecycle_state === "LIVE_ACTIVE",
  ).length;
  // 已激活（TEST_ACTIVE 及以上有效状态）即视为完成第一步；
  // 终态 OFFBOARDING 不计入，避免完成度随下架回退。
  const ACTIVATED_STATES = [
    "TEST_ACTIVE",
    "LIVE_REVIEW",
    "LIVE_ACTIVE",
    "RESTRICTED",
    "SUSPENDED",
  ];
  const activatedProviderCount = providers.filter((p) =>
    ACTIVATED_STATES.includes(p.lifecycle_state),
  ).length;

  const onboarding = getOnboardingState({
    providerCount: providers.length,
    activatedProviderCount,
    publishedVersionCount: publishedVersions,
    liveActiveCount,
  });

  const stats = [
    { label: "Providers", value: String(providers.length), icon: <BoxIcon size={18} aria-hidden="true" /> },
    { label: "活跃订阅", value: String(activeSubscriptions), icon: <LayersIcon size={18} aria-hidden="true" /> },
    { label: "客户", value: String(customers), icon: <UsersIcon size={18} aria-hidden="true" /> },
    { label: "账单收入", value: formatMoney(revenueCents), icon: <CreditCardIcon size={18} aria-hidden="true" /> },
  ];

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">概览</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          你好，{session.name || "用户"}。这里是你的 vLogBin 控制台。
        </p>
      </header>

      {providers.length === 0 ? (
        <FirstRunPanel onboarding={onboarding} />
      ) : (
        <>
          {!onboarding.completed && (
            <OnboardingStrip onboarding={onboarding} />
          )}

          <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
            {stats.map((s) => (
              <div
                key={s.label}
                className="rounded-xl border border-border bg-surface-1 p-4"
              >
                <div className="flex items-center justify-between text-muted-foreground">
                  <span className="text-xs font-medium">{s.label}</span>
                  {s.icon}
                </div>
                <p className="mt-2 text-2xl font-semibold tabular-nums">{s.value}</p>
              </div>
            ))}
          </div>

          {/* M2：自绘 SVG 趋势图（§7.5）——收入按出票日汇总 finalized 发票，
              用量事件按入库日计数 ingestion 事件；后端已补零为连续 30 天日轴。 */}
          <TrendSection overview={overview} />

          <RecentProviders providers={providers} />
        </>
      )}
    </div>
  );
}

/* ---------------- First-Run ---------------- */
function FirstRunPanel({ onboarding }: { onboarding: OnboardingState }) {
  return (
    <div className="rounded-2xl border border-border bg-surface-1 p-6 sm:p-10">
      <div className="mx-auto flex max-w-lg flex-col items-center text-center">
        <div className="flex h-12 w-12 items-center justify-center rounded-xl bg-brand-50 text-brand-700 dark:bg-brand-500/15 dark:text-brand-400">
          <LogoMark size={26} />
        </div>
        <h2 className="mt-4 text-xl font-semibold tracking-tight">欢迎来到 vLogBin</h2>
        <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
          三步开启首个计费闭环：创建 Provider、发布目录版本、上线生产环境。
          全程沙箱先行，随时可在测试与生产环境间切换。
        </p>

        <ol className="mt-8 w-full space-y-3 text-left">
          {onboarding.steps.map((step, i) => (
            <li key={step.id}>
              <Link
                href={step.href}
                prefetch={false}
                className="group flex items-start gap-3 rounded-lg border border-border bg-surface-2/60 px-4 py-3 transition-colors hover:border-brand-300 hover:bg-surface-2 dark:hover:border-brand-700"
              >
                <span className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-brand-600 text-xs font-semibold text-white">
                  {i + 1}
                </span>
                <span className="min-w-0">
                  <span className="block text-sm font-medium">{step.title}</span>
                  <span className="block text-xs leading-relaxed text-muted-foreground">
                    {step.description}
                  </span>
                </span>
                <ArrowRightIcon
                  size={16}
                  className="ml-auto mt-1 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5"
                  aria-hidden="true"
                />
              </Link>
            </li>
          ))}
        </ol>

        <LinkButton href="/ops/new" variant="primary" className="mt-8" size="lg" prefetch={false}>
          <PlusIcon size={16} aria-hidden="true" />
          开始创建 Provider
        </LinkButton>
      </div>
    </div>
  );
}

/* ---------------- 未完成引导条 ---------------- */
function OnboardingStrip({ onboarding }: { onboarding: OnboardingState }) {
  const doneCount = onboarding.steps.filter((s) => s.done).length;
  return (
    <div className="rounded-xl border border-border bg-surface-1 p-4">
      <div className="flex flex-wrap items-center gap-4">
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-4">
            <p className="text-sm font-medium">完成度 {onboarding.progress}%</p>
            <p className="text-xs text-muted-foreground">
              已完成 {doneCount}/{onboarding.steps.length} 步
            </p>
          </div>
          <Progress value={onboarding.progress} className="mt-2" />
        </div>
        <div className="flex items-center gap-2">
          {onboarding.steps.map((s) => (
            <span
              key={s.id}
              title={s.title}
              className="flex h-7 w-7 items-center justify-center rounded-full border text-xs"
              aria-hidden="true"
            >
              {s.done ? (
                <CheckIcon size={13} className="text-success" />
              ) : (
                <span className="text-muted-foreground">·</span>
              )}
            </span>
          ))}
        </div>
        <LinkButton href="/ops" variant="outline" size="sm" prefetch={false}>
          继续配置
        </LinkButton>
      </div>
    </div>
  );
}

/* ---------------- 近 30 天趋势（M2） ---------------- */
function TrendSection({ overview }: { overview: OverviewStats }) {
  return (
    <section>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-semibold">近 30 天趋势</h2>
      </div>
      <div className="grid gap-4 lg:grid-cols-3">
        <div className="rounded-xl border border-border bg-surface-1 p-4 lg:col-span-2">
          <div className="mb-1 flex items-baseline justify-between gap-2">
            <h3 className="text-xs font-medium text-muted-foreground">收入趋势</h3>
            <span className="text-[10px] text-muted-foreground">
              已确认发票金额（finalized）
            </span>
          </div>
          <AreaChart
            data={overview.trends.revenue}
            height={220}
            format="money"
          />
        </div>
        <div className="rounded-xl border border-border bg-surface-1 p-4">
          <div className="mb-1 flex items-baseline justify-between gap-2">
            <h3 className="text-xs font-medium text-muted-foreground">用量事件</h3>
            <span className="text-[10px] text-muted-foreground">
              ingestion 事件数
            </span>
          </div>
          <BarChart
            data={overview.trends.usage_events}
            height={220}
            format="count"
          />
        </div>
      </div>
    </section>
  );
}

/* ---------------- 最近 Provider ---------------- */
function RecentProviders({ providers }: { providers: Provider[] }) {
  const recent = providers.slice(0, 5);
  return (
    <section>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-semibold">最近 Provider</h2>
        <Link
          href="/ops"
          prefetch={false}
          className="text-sm font-medium text-brand-700 hover:underline dark:text-brand-400"
        >
          查看全部
        </Link>
      </div>
      <div className="overflow-hidden rounded-xl border border-border bg-surface-1">
        <ul className="divide-y divide-border">
          {recent.map((p) => (
            <li key={p.id}>
              <Link
                href={`/ops/${p.id}`}
                prefetch={false}
                className="flex items-center justify-between gap-4 px-4 py-3 transition-colors hover:bg-surface-2/60"
              >
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">{p.name}</span>
                  <span className="block text-xs text-muted-foreground">@{p.slug}</span>
                </span>
                <span className="flex shrink-0 items-center gap-2">
                  {p.lifecycle_state === "REGISTERED" && (
                    <span className="flex items-center gap-1 text-xs font-medium text-warning">
                      待激活
                      <ArrowRightIcon size={12} aria-hidden="true" />
                    </span>
                  )}
                  <LifecycleBadge state={p.lifecycle_state} />
                  <span className="hidden text-xs text-muted-foreground sm:inline">
                    {formatDate(p.created_at)}
                  </span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}
