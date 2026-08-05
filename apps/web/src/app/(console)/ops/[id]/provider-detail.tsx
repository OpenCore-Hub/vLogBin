"use client";

import { useActionState, useEffect, useId, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { cn } from "@/lib/utils";
import type {
  Provider,
  Environment,
  Capability,
  CatalogVersion,
  CatalogVersionDetail,
  Subscription,
  Customer,
  UsageEvent,
  Invoice,
  Region,
  AuditEvent,
  Credential,
  WebhookDelivery,
} from "@/lib/api/operator";
import { Tabs, TabPanel } from "@/components/ui/tabs";
import { Badge, EnvBadge, LifecycleBadge, type BadgeVariant } from "@/components/ui/badge";
import { Alert, EmptyState } from "@/components/ui/feedback";
import { Button } from "@/components/ui/button";
import { formatDate, formatDateTime, formatMoney } from "@/lib/format";
import { LifecycleActions } from "./lifecycle-actions";
import { replayWebhookDeliveryAction, revokeCredentialAction } from "../actions";
import {
  ActivityIcon,
  CreditCardIcon,
  KeyIcon,
  LayersIcon,
  ShieldIcon,
  UsersIcon,
  WebhookIcon,
} from "@/components/ui/icons";

/* ================= 本地表格辅助 ================= */
function Table({ head, children }: { head: ReactNode; children: ReactNode }) {
  return (
    <div className="overflow-x-auto rounded-xl border border-border bg-surface-1">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border bg-surface-2 text-left text-xs font-medium text-muted-foreground">
            {head}
          </tr>
        </thead>
        <tbody className="divide-y divide-border">{children}</tbody>
      </table>
    </div>
  );
}

function Th({ children, className }: { children?: ReactNode; className?: string }) {
  return <th className={cn("px-4 py-3 font-medium", className)}>{children}</th>;
}

function Td({ children, className }: { children?: ReactNode; className?: string }) {
  return <td className={cn("px-4 py-3 align-top", className)}>{children}</td>;
}

function CountBadge({ n }: { n: number }) {
  return (
    <span className="ml-1 rounded-full bg-surface-3 px-1.5 py-0.5 text-[10px] font-semibold tabular-nums text-muted-foreground">
      {n}
    </span>
  );
}

const CATALOG_STATE: Record<string, "draft" | "info" | "active" | "neutral"> = {
  draft: "draft",
  validated: "info",
  published: "active",
  retired: "neutral",
};

const SUB_STATUS: Record<string, "success" | "neutral"> = {
  active: "success",
  terminated: "neutral",
};

const INVOICE_STATUS: Record<string, "neutral" | "success" | "warning" | "danger"> = {
  draft: "neutral",
  finalized: "success",
  voided: "neutral",
  pending: "warning",
  failed: "danger",
};

const CAP_STATUS: Record<string, "success" | "warning" | "neutral"> = {
  granted: "success",
  pending: "warning",
  revoked: "neutral",
};

/* ================= 主组件 ================= */
export function ProviderDetail({
  provider,
  environments,
  capabilities,
  versions,
  detail,
  subscriptions,
  customers,
  usageEvents,
  invoices,
  regions,
  auditEvents,
  credentials,
  deliveries,
}: {
  provider: Provider;
  environments: Environment[];
  capabilities: Capability[];
  versions: CatalogVersion[];
  detail: CatalogVersionDetail | null;
  subscriptions: Subscription[];
  customers: Customer[];
  usageEvents: UsageEvent[];
  regions: Region[];
  invoices: Invoice[];
  auditEvents: AuditEvent[];
  credentials: Credential[];
  deliveries: WebhookDelivery[];
}) {
  const id = useId();
  const [tab, setTab] = useState("overview");

  const items = [
    { value: "overview", label: "概览" },
    { value: "catalog", label: "目录", badge: <CountBadge n={versions.length} /> },
    { value: "subscriptions", label: "订阅", badge: <CountBadge n={subscriptions.length} /> },
    { value: "customers", label: "客户", badge: <CountBadge n={customers.length} /> },
    { value: "usage", label: "用量", badge: <CountBadge n={usageEvents.length} /> },
    { value: "invoices", label: "账单", badge: <CountBadge n={invoices.length} /> },
    { value: "credentials", label: "密钥", badge: <CountBadge n={credentials.length} /> },
    { value: "deliveries", label: "投递", badge: <CountBadge n={deliveries.length} /> },
    { value: "audit", label: "审计", badge: <CountBadge n={auditEvents.length} /> },
  ];

  return (
    <div className="space-y-6">
      <Tabs items={items} value={tab} onChange={setTab} />

      {!["TEST_ACTIVE", "LIVE_REVIEW", "LIVE_ACTIVE", "RESTRICTED"].includes(
        provider.lifecycle_state,
      ) ? (
        <Alert variant="warning" title="只读模式">
          Provider 当前状态为 <strong>{provider.lifecycle_state}</strong>，所有写操作
          （目录、订阅、用量、密钥、Webhook、SCIM）已被服务端禁用（409），仅允许只读访问。
          请在生命周期中恢复 Provider 后继续。
        </Alert>
      ) : null}

      <TabPanel id={id} value="overview" selected={tab === "overview"}>
        <OverviewTab
          provider={provider}
          environments={environments}
          capabilities={capabilities}
          regions={regions}
        />
      </TabPanel>

      <TabPanel id={id} value="catalog" selected={tab === "catalog"}>
        <CatalogTab
          versions={versions}
          detail={detail}
          providerState={provider.lifecycle_state}
        />
      </TabPanel>

      <TabPanel id={id} value="subscriptions" selected={tab === "subscriptions"}>
        <SubscriptionsTab subscriptions={subscriptions} />
      </TabPanel>

      <TabPanel id={id} value="customers" selected={tab === "customers"}>
        <CustomersTab customers={customers} />
      </TabPanel>

      <TabPanel id={id} value="usage" selected={tab === "usage"}>
        <UsageTab events={usageEvents} />
      </TabPanel>

      <TabPanel id={id} value="invoices" selected={tab === "invoices"}>
        <InvoicesTab invoices={invoices} />
      </TabPanel>

      <TabPanel id={id} value="credentials" selected={tab === "credentials"}>
        <CredentialsTab credentials={credentials} providerId={provider.id} />
      </TabPanel>

      <TabPanel id={id} value="deliveries" selected={tab === "deliveries"}>
        <DeliveriesTab deliveries={deliveries} providerId={provider.id} />
      </TabPanel>

      <TabPanel id={id} value="audit" selected={tab === "audit"}>
        <AuditTab events={auditEvents} />
      </TabPanel>
    </div>
  );
}

/* ================= Webhook 投递 ================= */
const DELIVERY_STATUS: Record<string, BadgeVariant> = {
  pending: "warning",
  in_progress: "info",
  delivered: "success",
  failed: "danger",
  dead_letter: "danger",
};

const DELIVERY_STATUS_LABEL: Record<string, string> = {
  pending: "待投递",
  in_progress: "投递中",
  delivered: "已投递",
  failed: "失败",
  dead_letter: "死信",
};

function DeliveriesTab({
  deliveries,
  providerId,
}: {
  deliveries: WebhookDelivery[];
  providerId: string;
}) {
  const [state, formAction, pending] = useActionState(replayWebhookDeliveryAction, {
    ok: false,
  });
  const router = useRouter();

  useEffect(() => {
    if (state.ok) router.refresh();
  }, [state.ok, router]);

  if (deliveries.length === 0) {
    return (
      <EmptyState
        icon={<WebhookIcon size={20} aria-hidden="true" />}
        title="暂无 Webhook 投递记录"
        description="用量事件经 outbox 发布后会自动生成投递记录，投递失败会按指数退避重试。"
      />
    );
  }

  return (
    <div className="space-y-6">
      {state.error ? (
        <Alert variant="danger" title="重放失败">
          {state.error}
        </Alert>
      ) : null}

      <Table
        head={
          <>
            <Th>时间</Th>
            <Th>状态</Th>
            <Th>事件</Th>
            <Th>Endpoint</Th>
            <Th>尝试</Th>
            <Th>响应码</Th>
            <Th>投递完成</Th>
            <Th></Th>
          </>
        }
      >
        {deliveries.map((d) => (
          <tr key={d.id}>
            <Td className="whitespace-nowrap text-muted-foreground">
              {d.created_at ? formatDateTime(d.created_at) : "—"}
            </Td>
            <Td>
              <Badge variant={DELIVERY_STATUS[d.status] ?? "neutral"} title={d.status}>
                {DELIVERY_STATUS_LABEL[d.status] ?? d.status}
              </Badge>
            </Td>
            <Td className="font-mono text-xs text-muted-foreground">
              {d.outbox_event_id.slice(0, 8)}…
            </Td>
            <Td className="font-mono text-xs text-muted-foreground">
              {d.endpoint_id.slice(0, 8)}…
            </Td>
            <Td className="tabular-nums">{d.attempts}</Td>
            <Td className="font-mono text-xs text-muted-foreground">
              {d.response_status ?? "—"}
            </Td>
            <Td className="whitespace-nowrap text-muted-foreground">
              {d.delivered_at ? formatDateTime(d.delivered_at) : "—"}
            </Td>
            <Td>
              {d.status === "dead_letter" || d.status === "failed" ? (
                <form
                  action={formAction}
                  onSubmit={(e) => {
                    if (!confirm("确认重放该投递？将立即重新入队并投递。")) {
                      e.preventDefault();
                    }
                  }}
                >
                  <input type="hidden" name="provider_id" value={providerId} />
                  <input type="hidden" name="delivery_id" value={d.id} />
                  <Button type="submit" variant="outline" size="sm" disabled={pending}>
                    重放
                  </Button>
                </form>
              ) : null}
            </Td>
          </tr>
        ))}
      </Table>

      <p className="text-xs text-muted-foreground">
        死信（dead_letter）与失败（failed）投递可手动重放；挂起期间暂停投递，恢复后自动补发积压。
      </p>
    </div>
  );
}

/* ================= 概览 ================= */
function OverviewTab({
  provider,
  environments,
  capabilities,
  regions,
}: {
  provider: Provider;
  environments: Environment[];
  capabilities: Capability[];
  regions: Region[];
}) {
  const rows: Array<{ label: string; value: ReactNode }> = [
    { label: "Slug", value: <span className="font-mono text-[13px]">@{provider.slug}</span> },
    { label: "生命周期", value: <LifecycleBadge state={provider.lifecycle_state} /> },
    { label: "Home Region", value: provider.home_region_id || "—" },
    { label: "SLA 等级", value: provider.sla_tier || "—" },
    { label: "创建时间", value: formatDateTime(provider.created_at) },
    { label: "更新时间", value: formatDateTime(provider.updated_at) },
    { label: "Provider ID", value: <span className="font-mono text-xs">{provider.id}</span> },
  ];

  return (
    <div className="grid gap-6 lg:grid-cols-3">
      <section className="space-y-4 lg:col-span-2">
        <div className="rounded-xl border border-border bg-surface-1">
          <dl className="grid grid-cols-1 gap-x-6 gap-y-4 p-5 sm:grid-cols-2">
            {rows.map((r) => (
              <div key={r.label} className="min-w-0">
                <dt className="text-xs font-medium text-muted-foreground">{r.label}</dt>
                <dd className="mt-1 truncate text-sm">{r.value}</dd>
              </div>
            ))}
          </dl>
        </div>

        <section className="rounded-xl border border-border bg-surface-1 p-5">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="flex items-center gap-2 text-sm font-semibold">
              <WebhookIcon size={15} aria-hidden="true" />
              能力
            </h3>
          </div>
          {capabilities.length === 0 ? (
            <p className="text-sm text-muted-foreground">暂无授予的能力。</p>
          ) : (
            <ul className="divide-y divide-border">
              {capabilities.map((c) => (
                <li key={c.id} className="flex items-center justify-between gap-3 py-2.5">
                  <div className="min-w-0">
                    <p className="text-sm font-medium">{c.capability}</p>
                    <p className="text-xs text-muted-foreground">
                      授予于 {formatDate(c.granted_at)}
                    </p>
                  </div>
                  <Badge variant={CAP_STATUS[c.status] ?? "neutral"}>{c.status}</Badge>
                </li>
              ))}
            </ul>
          )}
        </section>
      </section>

      <section className="space-y-4">
        <div className="rounded-xl border border-border bg-surface-1 p-5">
          <h3 className="mb-3 text-sm font-semibold">生命周期操作</h3>
          <LifecycleActions
            providerId={provider.id}
            currentState={provider.lifecycle_state}
            regions={regions}
          />
        </div>

        <div className="rounded-xl border border-border bg-surface-1 p-5">
          <h3 className="mb-3 text-sm font-semibold">环境</h3>
          {environments.length === 0 ? (
            <p className="text-sm text-muted-foreground">暂无环境。</p>
          ) : (
            <ul className="space-y-2">
              {environments.map((e) => (
                <li key={e.id} className="flex items-center justify-between gap-3 text-sm">
                  <span className="font-medium">{e.kind === "live" ? "生产环境" : "测试环境"}</span>
                  <span className="flex items-center gap-2">
                    <Badge variant={e.status === "active" ? "success" : "neutral"}>
                      {e.status}
                    </Badge>
                    <EnvBadge env={e.kind} />
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </section>
    </div>
  );
}

/* ================= 目录 ================= */
function CatalogTab({
  versions,
  detail,
  providerState,
}: {
  versions: CatalogVersion[];
  detail: CatalogVersionDetail | null;
  providerState: string;
}) {
  // 发布目录版本是一种对外产品承诺，只有已激活且在服务中的 Provider
  // （TEST_ACTIVE / LIVE_REVIEW / LIVE_ACTIVE / RESTRICTED）才被允许；
  // 服务端会以 409 拒绝其它状态的发布请求。
  const canPublish = [
    "TEST_ACTIVE",
    "LIVE_REVIEW",
    "LIVE_ACTIVE",
    "RESTRICTED",
  ].includes(providerState);

  return (
    <div className="space-y-6">
      {!canPublish ? (
        <Alert variant="warning" title="无法发布目录版本">
          当前 Provider 状态为 <strong>{providerState}</strong>，未激活、已暂停或
          下线中的 Provider 不能发布目录版本。请在生命周期中恢复 Provider 后重试。
        </Alert>
      ) : null}
      {detail ? (
        <section className="rounded-xl border border-border bg-surface-1 p-5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="flex items-center gap-2 text-sm font-semibold">
              <LayersIcon size={15} aria-hidden="true" />
              已发布版本 v{detail.version.version}
            </h3>
            <Badge variant="active">published</Badge>
          </div>

          <div className="mt-4 grid gap-6 lg:grid-cols-2">
            <div>
              <h4 className="mb-2 text-xs font-semibold text-muted-foreground">指标 Metrics</h4>
              {detail.metrics.length === 0 ? (
                <p className="text-sm text-muted-foreground">暂无指标。</p>
              ) : (
                <Table
                  head={
                    <>
                      <Th>Code</Th>
                      <Th>名称</Th>
                      <Th>聚合</Th>
                      <Th>计费</Th>
                    </>
                  }
                >
                  {detail.metrics.map((m) => (
                    <tr key={m.id}>
                      <Td className="font-mono text-xs">{m.code}</Td>
                      <Td>{m.name}</Td>
                      <Td className="text-muted-foreground">{m.aggregation_type}</Td>
                      <Td>
                        <Badge variant={m.billable ? "brand" : "neutral"}>
                          {m.billable ? "billable" : "free"}
                        </Badge>
                      </Td>
                    </tr>
                  ))}
                </Table>
              )}
            </div>

            <div>
              <h4 className="mb-2 text-xs font-semibold text-muted-foreground">套餐 Plans</h4>
              {detail.plans.length === 0 ? (
                <p className="text-sm text-muted-foreground">暂无套餐。</p>
              ) : (
                <Table head={<><Th>Code</Th><Th>名称</Th><Th>周期</Th><Th>币种</Th></>}>
                  {detail.plans.map((p) => (
                    <tr key={p.id}>
                      <Td className="font-mono text-xs">{p.code}</Td>
                      <Td>{p.name}</Td>
                      <Td className="text-muted-foreground">{p.interval}</Td>
                      <Td className="text-muted-foreground">{p.currency}</Td>
                    </tr>
                  ))}
                </Table>
              )}
            </div>
          </div>
        </section>
      ) : null}

      <section>
        <h3 className="mb-3 text-sm font-semibold">版本历史</h3>
        {versions.length === 0 ? (
          <EmptyState
            icon={<LayersIcon size={20} aria-hidden="true" />}
            title="暂无目录版本"
            description="发布首个目录版本后，订阅与计费才会开始工作。"
          />
        ) : (
          <Table head={<><Th>版本</Th><Th>状态</Th><Th>环境</Th><Th>指标 / 套餐</Th><Th>发布时间</Th></>}>
            {versions.map((v) => (
              <tr key={v.id}>
                <Td className="font-mono">v{v.version}</Td>
                <Td>
                  <Badge variant={CATALOG_STATE[v.state] ?? "neutral"}>{v.state}</Badge>
                </Td>
                <Td><EnvBadge env={v.environment_kind} /></Td>
                <Td className="tabular-nums text-muted-foreground">
                  {v.metrics_count} / {v.plans_count}
                </Td>
                <Td className="text-muted-foreground">{formatDate(v.published_at)}</Td>
              </tr>
            ))}
          </Table>
        )}
      </section>
    </div>
  );
}

/* ================= 订阅 ================= */
function SubscriptionsTab({ subscriptions }: { subscriptions: Subscription[] }) {
  return subscriptions.length === 0 ? (
    <EmptyState
      icon={<LayersIcon size={20} aria-hidden="true" />}
      title="暂无订阅"
      description="客户订阅目录中的套餐后，会显示在这里。"
    />
  ) : (
    <Table
      head={
        <>
          <Th>订阅 ID</Th>
          <Th>客户</Th>
          <Th>套餐</Th>
          <Th>状态</Th>
          <Th>环境</Th>
          <Th>开始时间</Th>
        </>
      }
    >
      {subscriptions.map((s) => (
        <tr key={s.id}>
          <Td className="font-mono text-xs">{s.external_id}</Td>
          <Td className="font-mono text-xs">{s.customer_external_id}</Td>
          <Td className="font-mono text-xs">{s.plan_code}</Td>
          <Td>
            <Badge variant={SUB_STATUS[s.status] ?? "neutral"}>{s.status}</Badge>
          </Td>
          <Td><EnvBadge env={s.environment_kind} /></Td>
          <Td className="text-muted-foreground">{formatDate(s.started_at)}</Td>
        </tr>
      ))}
    </Table>
  );
}

/* ================= 客户 ================= */
function CustomersTab({ customers }: { customers: Customer[] }) {
  return customers.length === 0 ? (
    <EmptyState
      icon={<UsersIcon size={20} aria-hidden="true" />}
      title="暂无客户"
      description="客户在入驻（Onboarding）后显示在这里。"
    />
  ) : (
    <Table
      head={
        <>
          <Th>客户</Th>
          <Th>外部 ID</Th>
          <Th>类型</Th>
          <Th>环境</Th>
          <Th>创建时间</Th>
        </>
      }
    >
      {customers.map((c) => (
        <tr key={c.id}>
          <Td className="font-medium">{c.display_name}</Td>
          <Td className="font-mono text-xs">{c.external_id}</Td>
          <Td className="text-muted-foreground">{c.account_type}</Td>
          <Td><EnvBadge env={c.environment_kind} /></Td>
          <Td className="text-muted-foreground">{formatDate(c.created_at)}</Td>
        </tr>
      ))}
    </Table>
  );
}

/* ================= 用量 ================= */
function UsageTab({ events }: { events: UsageEvent[] }) {
  const recent = events.slice(0, 50);
  return events.length === 0 ? (
    <EmptyState
      icon={<ActivityIcon size={20} aria-hidden="true" />}
      title="暂无用量事件"
      description="通过 API 上报的用量事件会显示在这里（最多展示最近 50 条）。"
    />
  ) : (
    <Table
      head={
        <>
          <Th>事件</Th>
          <Th>类型</Th>
          <Th>指标</Th>
          <Th>客户</Th>
          <Th>环境</Th>
          <Th>事件时间</Th>
        </>
      }
    >
      {recent.map((e) => (
        <tr key={e.id}>
          <Td className="font-mono text-xs">{e.transaction_id}</Td>
          <Td className="text-muted-foreground">{e.kind}</Td>
          <Td className="font-mono text-xs">{e.metric_code}</Td>
          <Td className="font-mono text-xs">{e.customer_external_id}</Td>
          <Td><EnvBadge env={e.environment_kind} /></Td>
          <Td className="text-muted-foreground">{formatDateTime(e.event_timestamp)}</Td>
        </tr>
      ))}
    </Table>
  );
}

/* ================= 审计 ================= */
const ACTOR_STYLE: Record<string, "brand" | "info" | "neutral" | "warning"> = {
  system: "neutral",
  operator: "brand",
  provider: "info",
  customer: "warning",
};

/* ================= 密钥 ================= */
type CredentialState = "active" | "revoked" | "expired";

function credentialState(c: Credential): CredentialState {
  if (c.revoked_at) return "revoked";
  if (c.expires_at && new Date(c.expires_at).getTime() <= Date.now()) return "expired";
  return "active";
}

function CredentialsTab({
  credentials,
  providerId,
}: {
  credentials: Credential[];
  providerId: string;
}) {
  const [state, formAction, pending] = useActionState(revokeCredentialAction, { ok: false });
  const router = useRouter();

  useEffect(() => {
    if (state.ok) router.refresh();
  }, [state.ok, router]);

  const active = credentials.filter((c) => credentialState(c) === "active");
  const inactive = credentials.filter((c) => credentialState(c) !== "active");

  if (credentials.length === 0) {
    return (
      <EmptyState
        icon={<KeyIcon size={20} aria-hidden="true" />}
        title="暂无 API 密钥"
        description="激活测试或生产环境时会自动签发密钥，密钥会显示在这里。"
      />
    );
  }

  return (
    <div className="space-y-6">
      <section>
        <h3 className="mb-3 text-sm font-semibold">活跃密钥</h3>
        {active.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无活跃密钥。</p>
        ) : (
          <Table
            head={
              <>
                <Th>名称</Th>
                <Th>密钥</Th>
                <Th>环境</Th>
                <Th>权限</Th>
                <Th>最近使用</Th>
                <Th>过期时间</Th>
                <Th></Th>
              </>
            }
          >
            {active.map((c) => (
              <tr key={c.id}>
                <Td className="font-medium">{c.name}</Td>
                <Td>
                  <span className="font-mono text-xs text-muted-foreground">
                    {c.key_prefix}…
                  </span>
                </Td>
                <Td>
                  <EnvBadge env={c.environment_kind} />
                </Td>
                <Td className="max-w-[240px]">
                  {c.scopes.length === 0 ? (
                    <span className="text-muted-foreground">—</span>
                  ) : (
                    <span className="font-mono text-xs text-muted-foreground">
                      {c.scopes.join(", ")}
                    </span>
                  )}
                </Td>
                <Td className="whitespace-nowrap text-muted-foreground">
                  {c.last_used_at ? formatDateTime(c.last_used_at) : "从未使用"}
                </Td>
                <Td className="whitespace-nowrap text-muted-foreground">
                  {c.expires_at ? formatDate(c.expires_at) : "永不过期"}
                </Td>
                <Td>
                  <form action={formAction}>
                    <input type="hidden" name="provider_id" value={providerId} />
                    <input type="hidden" name="credential_id" value={c.id} />
                    <Button
                      type="submit"
                      variant="danger-outline"
                      size="sm"
                      disabled={pending}
                      onClick={(e) => {
                        if (
                          !window.confirm(
                            `确定吊销密钥 "${c.name}"？吊销后该密钥立即失效，且不可恢复。`,
                          )
                        ) {
                          e.preventDefault();
                        }
                      }}
                    >
                      吊销
                    </Button>
                  </form>
                </Td>
              </tr>
            ))}
          </Table>
        )}
      </section>

      {inactive.length > 0 ? (
        <section>
          <h3 className="mb-3 text-sm font-semibold">已吊销 / 已过期</h3>
          <Table
            head={
              <>
                <Th>名称</Th>
                <Th>密钥</Th>
                <Th>环境</Th>
                <Th>状态</Th>
                <Th>吊销时间</Th>
              </>
            }
          >
            {inactive.map((c) => (
              <tr key={c.id}>
                <Td className="font-medium text-muted-foreground">{c.name}</Td>
                <Td>
                  <span className="font-mono text-xs text-muted-foreground">
                    {c.key_prefix}…
                  </span>
                </Td>
                <Td>
                  <EnvBadge env={c.environment_kind} />
                </Td>
                <Td>
                  <Badge
                    variant={credentialState(c) === "revoked" ? "danger" : "neutral"}
                  >
                    {credentialState(c)}
                  </Badge>
                </Td>
                <Td className="whitespace-nowrap text-muted-foreground">
                  {c.revoked_at ? formatDateTime(c.revoked_at) : "—"}
                </Td>
              </tr>
            ))}
          </Table>
        </section>
      ) : null}

      {state.error ? (
        <Alert variant="danger" title="吊销失败">
          {state.error}
        </Alert>
      ) : null}
    </div>
  );
}

function AuditTab({ events }: { events: AuditEvent[] }) {
  const recent = events.slice(0, 200);
  return events.length === 0 ? (
    <EmptyState
      icon={<ShieldIcon size={20} aria-hidden="true" />}
      title="暂无审计记录"
      description="激活、生命周期转移等关键操作会记录在这里。"
    />
  ) : (
    <div className="overflow-x-auto rounded-xl border border-border bg-surface-1">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border bg-surface-2 text-left text-xs font-medium text-muted-foreground">
            <Th>时间</Th>
            <Th>动作</Th>
            <Th>执行者</Th>
            <Th>目标</Th>
            <Th>元数据</Th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {recent.map((e) => (
            <tr key={e.id}>
              <Td className="whitespace-nowrap text-muted-foreground">
                {formatDateTime(e.created_at)}
              </Td>
              <Td>
                <span className="font-mono text-xs">{e.action}</span>
              </Td>
              <Td>
                <div className="flex items-center gap-2">
                  <Badge variant={ACTOR_STYLE[e.actor_type] ?? "neutral"}>
                    {e.actor_type}
                  </Badge>
                  <span className="font-mono text-xs">{e.actor_id}</span>
                </div>
              </Td>
              <Td className="text-muted-foreground">
                {e.target_type ? (
                  <span>
                    {e.target_type}
                    {e.target_id ? (
                      <span className="ml-1 font-mono text-xs">{e.target_id}</span>
                    ) : null}
                  </span>
                ) : (
                  "—"
                )}
              </Td>
              <Td>
                {e.metadata !== undefined && e.metadata !== null ? (
                  <pre className="max-w-[320px] overflow-x-auto whitespace-pre-wrap font-mono text-[11px] text-muted-foreground">
                    {JSON.stringify(e.metadata)}
                  </pre>
                ) : (
                  "—"
                )}
              </Td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/* ================= 账单 ================= */
function InvoicesTab({ invoices }: { invoices: Invoice[] }) {
  return invoices.length === 0 ? (
    <EmptyState
      icon={<CreditCardIcon size={20} aria-hidden="true" />}
      title="暂无账单"
      description="订阅产生结算后，账单会显示在这里。"
    />
  ) : (
    <Table
      head={
        <>
          <Th>账单号</Th>
          <Th>客户</Th>
          <Th>状态</Th>
          <Th>支付状态</Th>
          <Th>金额</Th>
          <Th>开票日期</Th>
        </>
      }
    >
      {invoices.map((inv) => (
        <tr key={inv.id}>
          <Td className="font-mono text-xs">{inv.number}</Td>
          <Td className="font-mono text-xs">{inv.customer_external_id}</Td>
          <Td>
            <Badge variant={INVOICE_STATUS[inv.status] ?? "neutral"}>{inv.status}</Badge>
          </Td>
          <Td className="text-muted-foreground">{inv.payment_status}</Td>
          <Td className="font-semibold tabular-nums">
            {formatMoney(inv.total_amount_cents, inv.currency)}
          </Td>
          <Td className="text-muted-foreground">{formatDate(inv.issuing_date)}</Td>
        </tr>
      ))}
    </Table>
  );
}
