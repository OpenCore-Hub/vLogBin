"use client";

import { useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import type {
  CatalogVersion,
  CatalogVersionDetail,
  PlanCollection,
} from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { formatDateTime } from "@/lib/format";
import { LinkButton } from "@/components/ui/button";
import { EmptyState, ErrorState } from "@/components/ui/feedback";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { Select } from "@/components/ui/field";
import { useEnv } from "@/components/console/env-provider";
import {
  ArrowRightIcon,
  LayersIcon,
} from "@/components/ui/icons";

const VERSION_STATE: Record<string, "success" | "info" | "warning" | "neutral"> = {
  draft: "warning",
  validated: "info",
  published: "success",
  retired: "neutral",
};

export function CatalogClient({
  providerId,
  env,
  versions,
  selectedVersionId,
  detail,
  currentPlans,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  versions: CatalogVersion[];
  selectedVersionId: string | null;
  detail: CatalogVersionDetail | null;
  currentPlans: PlanCollection;
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
        <h1 className="text-2xl font-semibold tracking-tight">Catalog</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          查看目录版本、指标、套餐价格与权益组成。版本选择写入 URL，可分享可回退。
        </p>
      </header>

      {loadError ? (
        <ErrorState title="目录数据加载失败" description={loadError} />
      ) : !providerId ? (
        <EmptyState
          icon={<LayersIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，再查看目录版本。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : versions.length === 0 ? (
        <EmptyState
          icon={<LayersIcon size={20} aria-hidden="true" />}
          title="暂无目录版本"
          description="在 Plans 页面创建套餐并发布后，目录版本会显示在这里。"
          action={
            <LinkButton
              href="/console/billing/plans"
              variant="primary"
              prefetch={false}
            >
              前往套餐
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-3">
            <Select
              aria-label="选择目录版本"
              value={selectedVersionId ?? ""}
              onChange={(e) =>
                router.push(`/console/catalog?version=${e.target.value}`)
              }
              className="h-9 w-72 text-sm"
            >
              {versions.map((v) => (
                <option key={v.id} value={v.id}>
                  v{v.version} · {v.state}
                </option>
              ))}
            </Select>
            <EnvBadge env={env} />
            <span className="text-xs text-muted-foreground">
              当前 draft：{currentPlans.plans.length} 个套餐 · {currentPlans.metrics.length} 个指标
            </span>
          </div>

          {detail && <VersionDetail detail={detail} />}
        </>
      )}
    </div>
  );
}

function VersionDetail({ detail }: { detail: CatalogVersionDetail }) {
  const version = detail.version;
  const meta = [
    { label: "版本", value: `v${version.version}` },
    {
      label: "状态",
      value: (
        <Badge variant={VERSION_STATE[version.state] ?? "neutral"}>
          {version.state}
        </Badge>
      ),
    },
    { label: "创建时间", value: formatDateTime(version.created_at) },
    { label: "指标", value: String(detail.metrics.length) },
    { label: "套餐", value: String(detail.plans.length) },
  ];

  return (
    <>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-5">
        {meta.map((m) => (
          <Card key={m.label} className="p-4">
            <p className="text-xs text-muted-foreground">{m.label}</p>
            <div className="mt-1.5 text-sm font-semibold">{m.value}</div>
          </Card>
        ))}
      </div>

      <Card className="overflow-hidden">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold">指标</h2>
        </div>
        {detail.metrics.length === 0 ? (
          <EmptyState
            title="暂无指标"
            className="border-0 shadow-none"
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="bg-surface-2/70 text-left text-xs font-medium text-muted-foreground">
                <tr>
                  <th className="px-4 py-3 font-medium">代码</th>
                  <th className="px-4 py-3 font-medium">名称</th>
                  <th className="px-4 py-3 font-medium">聚合</th>
                  <th className="px-4 py-3 font-medium">计费</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {detail.metrics.map((m) => (
                  <tr key={m.id} className="transition-colors hover:bg-surface-2/60">
                    <td className="px-4 py-3">
                      <code className="font-mono text-xs">{m.code}</code>
                    </td>
                    <td className="px-4 py-3 text-sm">{m.name}</td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">
                      {m.aggregation_type}
                    </td>
                    <td className="px-4 py-3">
                      <Badge variant={m.billable ? "success" : "neutral"}>
                        {m.billable ? "计费" : "不计费"}
                      </Badge>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Card className="overflow-hidden">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold">套餐</h2>
        </div>
        {detail.plans.length === 0 ? (
          <EmptyState title="暂无套餐" className="border-0 shadow-none" />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="bg-surface-2/70 text-left text-xs font-medium text-muted-foreground">
                <tr>
                  <th className="px-4 py-3 font-medium">代码</th>
                  <th className="px-4 py-3 font-medium">名称</th>
                  <th className="px-4 py-3 font-medium">周期</th>
                  <th className="px-4 py-3 font-medium">货币</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {detail.plans.map((p) => (
                  <tr key={p.id} className="transition-colors hover:bg-surface-2/60">
                    <td className="px-4 py-3">
                      <code className="font-mono text-xs">{p.code}</code>
                    </td>
                    <td className="px-4 py-3 text-sm">{p.name}</td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">
                      {p.interval}
                    </td>
                    <td className="px-4 py-3 font-mono text-xs">{p.currency}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Card className="overflow-hidden">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold">价格</h2>
        </div>
        {detail.prices.length === 0 ? (
          <EmptyState title="暂无价格" className="border-0 shadow-none" />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="bg-surface-2/70 text-left text-xs font-medium text-muted-foreground">
                <tr>
                  <th className="px-4 py-3 font-medium">计费模型</th>
                  <th className="px-4 py-3 font-medium">指标</th>
                  <th className="px-4 py-3 font-medium">属性</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {detail.prices.map((price) => (
                  <tr key={price.id} className="transition-colors hover:bg-surface-2/60">
                    <td className="px-4 py-3">
                      <Badge variant="info">{price.charge_model}</Badge>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs">
                      {price.metric_code ?? "—"}
                    </td>
                    <td className="px-4 py-3">
                      <pre className="max-w-md overflow-x-auto whitespace-pre-wrap font-mono text-[11px] text-muted-foreground">
                        {JSON.stringify(price.properties ?? {})}
                      </pre>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Card className="overflow-hidden">
        <div className="border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold">权益授权</h2>
        </div>
        {detail.entitlement_grants.length === 0 ? (
          <EmptyState title="暂无权益" className="border-0 shadow-none" />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="bg-surface-2/70 text-left text-xs font-medium text-muted-foreground">
                <tr>
                  <th className="px-4 py-3 font-medium">Key</th>
                  <th className="px-4 py-3 font-medium">类型</th>
                  <th className="px-4 py-3 font-medium">值</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {detail.entitlement_grants.map((g) => (
                  <tr key={g.id} className="transition-colors hover:bg-surface-2/60">
                    <td className="px-4 py-3">
                      <code className="font-mono text-xs">{g.key}</code>
                    </td>
                    <td className="px-4 py-3 text-xs text-muted-foreground">
                      {g.value_type}
                    </td>
                    <td className="px-4 py-3">
                      <code className="font-mono text-xs">
                        {JSON.stringify(g.value)}
                      </code>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </>
  );
}
