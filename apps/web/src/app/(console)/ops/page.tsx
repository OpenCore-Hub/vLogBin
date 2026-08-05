import Link from "next/link";
import { requireRole } from "@/lib/auth/rbac";
import { listProviders, listRegions } from "@/lib/api/operator";
import type { Provider, Region } from "@/lib/api/operator";
import { LifecycleBadge } from "@/components/ui/badge";
import { EmptyState, ErrorState } from "@/components/ui/feedback";
import { LinkButton } from "@/components/ui/button";
import { BoxIcon, PlusIcon, ZapIcon } from "@/components/ui/icons";
import { formatDate } from "@/lib/format";

export const dynamic = "force-dynamic";

export default async function OpsPage() {
  await requireRole("operator");

  let providers: Provider[] = [];
  let regions: Region[] = [];
  let error: string | null = null;
  try {
    providers = await listProviders();
    // 区域列表仅用于把 home_region_id 翻译成 code，失败不影响主列表。
    regions = await listRegions().catch(() => [] as Region[]);
  } catch (err) {
    error = err instanceof Error ? err.message : "无法加载 Provider 列表，请稍后重试";
  }
  const regionByID = new Map(regions.map((r) => [r.id, r.code]));

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Providers</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            管理服务提供商及其生命周期状态
          </p>
        </div>
        {providers.length > 0 && (
          <LinkButton href="/ops/new" variant="primary" prefetch={false}>
            <PlusIcon size={16} aria-hidden="true" />
            新建 Provider
          </LinkButton>
        )}
      </header>

      {error ? (
        <ErrorState
          title="加载失败"
          description={error}
          action={
            <LinkButton href="/ops" variant="outline" size="sm" prefetch={false}>
              重试
            </LinkButton>
          }
        />
      ) : providers.length === 0 ? (
        <EmptyState
          icon={<BoxIcon size={20} aria-hidden="true" />}
          title="还没有 Provider"
          description="创建第一个 Provider，开始接入订阅、计费与用量管理能力。"
          action={
            <LinkButton href="/ops/new" variant="primary" prefetch={false}>
              <PlusIcon size={16} aria-hidden="true" />
              新建 Provider
            </LinkButton>
          }
        />
      ) : (
        <div className="overflow-hidden rounded-xl border border-border bg-surface-1">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-surface-2 text-left text-xs font-medium text-muted-foreground">
                <th className="px-4 py-3">Provider</th>
                <th className="px-4 py-3">生命周期</th>
                <th className="px-4 py-3">Home Region</th>
                <th className="px-4 py-3">创建时间</th>
                <th className="px-4 py-3 text-right">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {providers.map((p) => (
                <tr key={p.id} className="transition-colors hover:bg-surface-2/60">
                  <td className="px-4 py-3">
                    {/* R31：列表页禁止 prefetch —— 大列表下每个详情页 RSC
                        会并行打 10 个 API 请求，全量预取会形成请求风暴并
                        触发自身 endpoint 限流（429）。点击时再加载。 */}
                    <Link
                      href={`/ops/${p.id}`}
                      prefetch={false}
                      className="inline-flex flex-col leading-tight"
                    >
                      <span className="font-medium hover:text-brand-700 dark:hover:text-brand-400">
                        {p.name}
                      </span>
                      <span className="text-xs text-muted-foreground">@{p.slug}</span>
                    </Link>
                  </td>
                  <td className="px-4 py-3">
                    <span className="inline-flex items-center gap-2">
                      <LifecycleBadge state={p.lifecycle_state} />
                      {p.lifecycle_state === "REGISTERED" && (
                        <span className="text-xs font-medium text-warning">
                          待激活
                        </span>
                      )}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    {p.home_region_id
                      ? regionByID.get(p.home_region_id) ?? "—"
                      : "—"}
                  </td>
                  <td className="px-4 py-3 text-muted-foreground">
                    {formatDate(p.created_at)}
                  </td>
                  <td className="px-4 py-3 text-right">
                    {p.lifecycle_state === "REGISTERED" ? (
                      <Link
                        href={`/ops/${p.id}`}
                        prefetch={false}
                        className="inline-flex items-center gap-1 text-sm font-medium text-brand-700 hover:underline dark:text-brand-400"
                      >
                        <ZapIcon size={14} aria-hidden="true" />
                        去激活
                      </Link>
                    ) : (
                      <Link
                        href={`/ops/${p.id}`}
                        prefetch={false}
                        className="text-sm font-medium text-brand-700 hover:underline dark:text-brand-400"
                      >
                        查看详情
                      </Link>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
