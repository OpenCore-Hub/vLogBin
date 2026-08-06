"use client";

import { useActionState, useState } from "react";
import { useActionFeedback } from "@/hooks/use-action-feedback";
import { usePathname, useRouter } from "next/navigation";
import Link from "next/link";
import type {
  Cell,
  CellFailover,
  CellMigration,
  Provider,
  Region,
  RiskReview,
  SupportSession,
} from "@/lib/api/operator";
import { formatDate } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import { Dialog } from "@/components/ui/overlay";
import { EmptyState, ErrorState, Alert, SuccessPanel } from "@/components/ui/feedback";
import { Badge, LifecycleBadge } from "@/components/ui/badge";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import { Tabs, TabPanel } from "@/components/ui/tabs";
import {
  BoxIcon,
  PlusIcon,
  ShieldIcon,
} from "@/components/ui/icons";
import {
  assignProviderCellAction,
  createCellAction,
  submitRiskReviewAction,
  updateCellStatusAction,
  type OpActionState,
} from "./actions";

const initialState: OpActionState = { ok: false };

const RISK_CHECKS: Array<{ key: string; label: string }> = [
  { key: "email_and_company_domain", label: "邮箱与企业域名归属" },
  { key: "tos_dpa", label: "服务条款与数据处理协议" },
  { key: "custom_domain_ownership", label: "自定义域所有权" },
  { key: "payment_tax_connection", label: "Payment / Tax Connection 有效" },
  { key: "webhook_destination", label: "Webhook 目的地验证" },
  { key: "initial_quota", label: "初始配额已分配" },
  { key: "security_contact", label: "安全联系人已登记" },
];

const CELL_STATUS: Record<string, "success" | "warning" | "neutral"> = {
  active: "success",
  draining: "warning",
  inactive: "neutral",
};

export function OpsClient({
  providers,
  regions,
  reviewRows,
  awaitingReviews,
  supportSessions,
  cells,
  failovers,
  migrations,
  error,
}: {
  providers: Provider[];
  regions: Region[];
  reviewRows: Array<{ provider: Provider; review: RiskReview }>;
  awaitingReviews: Provider[];
  supportSessions: SupportSession[];
  cells: Cell[];
  failovers: CellFailover[];
  migrations: CellMigration[];
  error: string | null;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const tab = pathname.endsWith("/reviews")
    ? "reviews"
    : pathname.endsWith("/cells")
      ? "cells"
      : "providers";
  const [reviewProvider, setReviewProvider] = useState<Provider | null>(null);
  const [createCellOpen, setCreateCellOpen] = useState(false);
  const [statusCell, setStatusCell] = useState<Cell | null>(null);

  const regionByID = new Map(regions.map((r) => [r.id, r.code]));

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Providers</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Provider 生命周期、准入审核队列与 Cell 拓扑运维。
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
        <ErrorState title="加载失败" description={error} />
      ) : (
        <>
          <Tabs
            value={tab}
            onChange={(value) =>
              router.push(
                value === "reviews"
                  ? "/ops/reviews"
                  : value === "cells"
                    ? "/ops/cells"
                    : "/ops",
              )
            }
            items={[
              { value: "providers", label: "Providers" },
              { value: "reviews", label: "审核" },
              { value: "cells", label: "Cell 运维" },
            ]}
          />

          <TabPanel id="ops" value="providers" selected={tab === "providers"}>
            <ProvidersTab providers={providers} regions={regions} />
          </TabPanel>

          <TabPanel id="ops" value="reviews" selected={tab === "reviews"}>
            <ReviewsTab
              reviewRows={reviewRows}
              awaitingReviews={awaitingReviews}
              supportSessions={supportSessions}
              onReview={setReviewProvider}
            />
          </TabPanel>

          <TabPanel id="ops" value="cells" selected={tab === "cells"}>
            <CellsTab
              cells={cells}
              regionByID={regionByID}
              providers={providers}
              failovers={failovers}
              migrations={migrations}
              onCreate={() => setCreateCellOpen(true)}
              onStatus={setStatusCell}
            />
          </TabPanel>
        </>
      )}

      {reviewProvider && (
        <RiskReviewDialog
          key={reviewProvider.id}
          open={!!reviewProvider}
          onOpenChange={(open) => !open && setReviewProvider(null)}
          provider={reviewProvider}
        />
      )}
      {createCellOpen && (
        <CreateCellDialog
          open={createCellOpen}
          onOpenChange={setCreateCellOpen}
          regions={regions}
          onSaved={() => router.refresh()}
        />
      )}
      {statusCell && (
        <CellStatusDialog
          key={statusCell.id}
          open={!!statusCell}
          onOpenChange={(open) => !open && setStatusCell(null)}
          cell={statusCell}
        />
      )}
    </div>
  );
}

function ProvidersTab({
  providers,
  regions,
}: {
  providers: Provider[];
  regions: Region[];
}) {
  const regionByID = new Map(regions.map((r) => [r.id, r.code]));
  return providers.length === 0 ? (
    <EmptyState
      icon={<BoxIcon size={20} aria-hidden="true" />}
      title="还没有 Provider"
      description="创建第一个 Provider，开始接入订阅、计费与用量管理能力。"
      action={
        <Link
          href="/ops/new"
          className="inline-flex h-9.5 items-center justify-center gap-2 rounded-md bg-brand-600 px-4 text-sm font-medium text-white shadow-sm"
        >
          <PlusIcon size={16} aria-hidden="true" />
          新建 Provider
        </Link>
      }
    />
  ) : (
    <div className="overflow-hidden rounded-xl border border-border bg-surface-1">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-border bg-surface-2 text-left text-xs font-medium text-muted-foreground">
            <th className="px-4 py-3 font-medium">Provider</th>
            <th className="px-4 py-3 font-medium">生命周期</th>
            <th className="px-4 py-3 font-medium">Home Region</th>
            <th className="px-4 py-3 font-medium">创建时间</th>
            <th className="px-4 py-3 font-medium text-right">操作</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {providers.map((p) => (
            <tr key={p.id} className="transition-colors hover:bg-surface-2/60">
              <td className="px-4 py-3">
                <Link href={`/ops/${p.id}`} prefetch={false} className="inline-flex flex-col leading-tight">
                  <span className="font-medium text-foreground hover:text-brand-700 dark:hover:text-brand-400">
                    {p.name}
                  </span>
                  <span className="text-xs text-muted-foreground">@{p.slug}</span>
                </Link>
              </td>
              <td className="px-4 py-3">
                <LifecycleBadge state={p.lifecycle_state} />
              </td>
              <td className="px-4 py-3 text-muted-foreground">
                {p.home_region_id ? regionByID.get(p.home_region_id) ?? "—" : "—"}
              </td>
              <td className="px-4 py-3 text-muted-foreground">{formatDate(p.created_at)}</td>
              <td className="px-4 py-3 text-right">
                <Link href={`/ops/${p.id}`} prefetch={false} className="text-sm font-medium text-brand-700 hover:underline dark:text-brand-400">
                  查看详情
                </Link>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ReviewsTab({
  reviewRows,
  awaitingReviews,
  supportSessions,
  onReview,
}: {
  reviewRows: Array<{ provider: Provider; review: RiskReview }>;
  awaitingReviews: Provider[];
  supportSessions: SupportSession[];
  onReview: (provider: Provider) => void;
}) {
  const columns: DataTableColumn<{ provider: Provider; review: RiskReview }>[] = [
    {
      key: "provider",
      header: "Provider",
      cell: (row) => (
        <Link href={`/ops/${row.provider.id}`} prefetch={false} className="font-medium text-foreground hover:text-brand-700">
          {row.provider.name}
        </Link>
      ),
    },
    {
      key: "risk_score",
      header: "风险评分",
      numeric: true,
      sortable: true,
      sortValue: (row) => row.review.risk_score,
      cell: (row) => <span className="tabular-nums">{row.review.risk_score}</span>,
    },
    {
      key: "decision",
      header: "结论",
      cell: (row) => (
        <Badge variant={row.review.decision === "approved" ? "success" : "danger"}>
          {row.review.decision === "approved" ? "已批准" : "已拒绝"}
        </Badge>
      ),
    },
    {
      key: "reviewed_by",
      header: "审核人",
      cell: (row) => <span className="text-sm text-muted-foreground">{row.review.reviewed_by}</span>,
    },
    {
      key: "reviewed_at",
      header: "审核时间",
      sortable: true,
      sortValue: (row) => row.review.reviewed_at ?? "",
      cell: (row) => <span className="text-xs text-muted-foreground tabular-nums">{formatDate(row.review.reviewed_at)}</span>,
    },
  ];

  return (
    <div className="space-y-8">
      <section className="space-y-4">
        <div>
          <h2 className="text-sm font-semibold">待审核</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            LIVE_REVIEW 状态且尚未获得批准的 Provider。
          </p>
        </div>
        {awaitingReviews.length === 0 ? (
          <EmptyState icon={<ShieldIcon size={20} aria-hidden="true" />} title="审核队列为空" />
        ) : (
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {awaitingReviews.map((p) => (
              <div key={p.id} className="rounded-xl border border-border bg-surface-1 p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate font-medium text-foreground">{p.name}</p>
                    <p className="font-mono text-xs text-muted-foreground">@{p.slug}</p>
                  </div>
                  <LifecycleBadge state={p.lifecycle_state} />
                </div>
                <Button size="sm" className="mt-4" onClick={() => onReview(p)}>
                  提交审核
                </Button>
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="space-y-4">
        <h2 className="text-sm font-semibold">风险审核记录</h2>
        <DataTable
          data={reviewRows}
          columns={columns}
          rowKey={(row) => row.review.id}
          searchKeys={(row) => [row.provider.name, row.provider.slug, row.review.reviewed_by]}
          defaultSort={{ key: "reviewed_at", dir: "desc" }}
          emptyLabel="暂无审核记录"
        />
      </section>

      <section className="space-y-4">
        <h2 className="text-sm font-semibold">支持会话</h2>
        <SupportSessionsTable sessions={supportSessions} />
      </section>
    </div>
  );
}

function SupportSessionsTable({ sessions }: { sessions: SupportSession[] }) {
  const columns: DataTableColumn<SupportSession>[] = [
    {
      key: "status",
      header: "状态",
      sortable: true,
      sortValue: (s) => s.status,
      cell: (s) => <Badge variant={s.status === "active" ? "success" : s.status === "requested" ? "warning" : "neutral"}>{s.status}</Badge>,
    },
    {
      key: "access_type",
      header: "类型",
      cell: (s) => <Badge variant={s.access_type === "emergency" ? "danger" : "info"}>{s.access_type}</Badge>,
    },
    {
      key: "requested_by",
      header: "申请人",
      cell: (s) => <span className="font-mono text-xs text-muted-foreground">{s.requested_by}</span>,
    },
    {
      key: "reason",
      header: "原因",
      cell: (s) => <span className="line-clamp-2 max-w-md text-sm text-muted-foreground">{s.reason}</span>,
    },
    {
      key: "expires_at",
      header: "过期时间",
      sortable: true,
      sortValue: (s) => s.expires_at ?? "",
      cell: (s) => <span className="text-xs text-muted-foreground tabular-nums">{formatDate(s.expires_at)}</span>,
    },
  ];
  return (
    <DataTable
      data={sessions}
      columns={columns}
      rowKey={(s) => s.id}
      searchKeys={(s) => [s.status, s.requested_by, s.reason]}
      defaultSort={{ key: "expires_at", dir: "desc" }}
      emptyLabel="暂无支持会话"
    />
  );
}

function CellsTab({
  cells,
  regionByID,
  providers,
  failovers,
  migrations,
  onCreate,
  onStatus,
}: {
  cells: Cell[];
  regionByID: Map<string, string>;
  providers: Provider[];
  failovers: CellFailover[];
  migrations: CellMigration[];
  onCreate: () => void;
  onStatus: (cell: Cell) => void;
}) {
  const cellColumns: DataTableColumn<Cell>[] = [
    {
      key: "code",
      header: "Code",
      sortable: true,
      sortValue: (c) => c.code,
      cell: (c) => <code className="font-mono text-xs text-foreground">{c.code}</code>,
    },
    {
      key: "region",
      header: "Region",
      cell: (c) => <span className="text-sm text-muted-foreground">{regionByID.get(c.region_id) ?? "—"}</span>,
    },
    {
      key: "cell_type",
      header: "类型",
      cell: (c) => <Badge variant={c.cell_type === "shared" ? "info" : "brand"}>{c.cell_type}</Badge>,
    },
    {
      key: "status",
      header: "状态",
      sortable: true,
      sortValue: (c) => c.status,
      cell: (c) => <Badge variant={CELL_STATUS[c.status] ?? "neutral"}>{c.status}</Badge>,
    },
    {
      key: "actions",
      header: <span className="sr-only">操作</span>,
      className: "text-right",
      cell: (c) => (
        <Button variant="ghost" size="sm" onClick={() => onStatus(c)}>
          改状态
        </Button>
      ),
    },
  ];

  return (
    <div className="space-y-8">
      <section className="space-y-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold">Cells</h2>
            <p className="mt-1 text-sm text-muted-foreground">区域共享/专用拓扑单元。</p>
          </div>
          <Button onClick={onCreate}>
            <PlusIcon size={16} aria-hidden="true" />
            新建 Cell
          </Button>
        </div>
        <DataTable
          data={cells}
          columns={cellColumns}
          rowKey={(c) => c.id}
          searchKeys={(c) => [c.code, c.cell_type, c.status]}
          defaultSort={{ key: "code", dir: "asc" }}
          emptyLabel="暂无 Cell"
        />
      </section>

      <AssignCellForm providers={providers} cells={cells} />

      <section className="space-y-4">
        <h2 className="text-sm font-semibold">故障切换</h2>
        <FailoversTable failovers={failovers} />
      </section>

      <section className="space-y-4">
        <h2 className="text-sm font-semibold">Cell 迁移</h2>
        <MigrationsTable migrations={migrations} />
      </section>
    </div>
  );
}

function AssignCellForm({ providers, cells }: { providers: Provider[]; cells: Cell[] }) {
  const [state, formAction, pending] = useActionState(assignProviderCellAction, initialState);
  return (
    <section className="rounded-xl border border-border bg-surface-1 p-4">
      <h2 className="text-sm font-semibold">分配 Provider</h2>
      <p className="mt-1 text-sm text-muted-foreground">将 Provider 绑定到指定 Cell。</p>
      <form action={formAction} className="mt-4 flex flex-wrap items-end gap-3">
        <Field label="Provider" className="min-w-[220px]">
          <Select name="provider_id" defaultValue="">
            <option value="" disabled>选择 Provider</option>
            {providers.map((p) => (
              <option key={p.id} value={p.id}>{p.name}</option>
            ))}
          </Select>
        </Field>
        <Field label="Cell" className="min-w-[200px]">
          <Select name="cell_id" defaultValue="">
            <option value="" disabled>选择 Cell</option>
            {cells.filter((c) => c.status === "active").map((c) => (
              <option key={c.id} value={c.id}>{c.code}</option>
            ))}
          </Select>
        </Field>
        <Button type="submit" loading={pending}>分配</Button>
      </form>
      {state.ok && <SuccessPanel title="已分配" className="mt-3" />}
      {state.error && <Alert title="分配失败" className="mt-3">{state.error}</Alert>}
    </section>
  );
}

function FailoversTable({ failovers }: { failovers: CellFailover[] }) {
  const columns: DataTableColumn<CellFailover>[] = [
    {
      key: "status",
      header: "状态",
      cell: (f) => <Badge variant={f.status === "completed" ? "success" : f.status === "aborted" ? "neutral" : "warning"}>{f.status}</Badge>,
    },
    {
      key: "provider",
      header: "Provider ID",
      cell: (f) => <code className="font-mono text-[11px] text-muted-foreground">{f.provider_id}</code>,
    },
    {
      key: "from_to",
      header: "From → To",
      cell: (f) => <code className="font-mono text-[11px] text-muted-foreground">{f.from_cell_id.slice(0, 8)} → {f.to_cell_id.slice(0, 8)}</code>,
    },
    {
      key: "started_at",
      header: "开始时间",
      cell: (f) => <span className="text-xs text-muted-foreground">{formatDate(f.started_at)}</span>,
    },
  ];
  return <DataTable data={failovers} columns={columns} rowKey={(f) => f.id} searchKeys={(f) => [f.status, f.provider_id]} emptyLabel="暂无故障切换" />;
}

function MigrationsTable({ migrations }: { migrations: CellMigration[] }) {
  const columns: DataTableColumn<CellMigration>[] = [
    {
      key: "status",
      header: "状态",
      cell: (m) => <Badge variant={m.status === "completed" ? "success" : m.status === "cancelled" || m.status === "failed" ? "neutral" : "warning"}>{m.status}</Badge>,
    },
    {
      key: "provider",
      header: "Provider ID",
      cell: (m) => <code className="font-mono text-[11px] text-muted-foreground">{m.provider_id}</code>,
    },
    {
      key: "precheck",
      header: "预检",
      cell: (m) => <Badge variant={m.precheck_passed ? "success" : "neutral"}>{m.precheck_passed ? "通过" : "未通过"}</Badge>,
    },
    {
      key: "records",
      header: "记录数",
      numeric: true,
      cell: (m) => <span className="tabular-nums">{m.record_count}</span>,
    },
    {
      key: "created_at",
      header: "创建时间",
      cell: (m) => <span className="text-xs text-muted-foreground">{formatDate(m.created_at)}</span>,
    },
  ];
  return <DataTable data={migrations} columns={columns} rowKey={(m) => m.id} searchKeys={(m) => [m.status, m.provider_id]} emptyLabel="暂无迁移任务" />;
}

function RiskReviewDialog({
  open,
  onOpenChange,
  provider,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  provider: Provider;
}) {
  const [state, formAction, pending] = useActionState(submitRiskReviewAction, initialState);
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="提交风险审核"
      description={`${provider.name}（@${provider.slug}）进入生产前审核。`}
      size="lg"
    >
      {state.ok ? (
        <div className="space-y-4">
          <SuccessPanel title="审核已提交" description="结果已写入风险审核记录与审计链。" />
          <div className="flex justify-end">
            <Button onClick={() => onOpenChange(false)}>完成</Button>
          </div>
        </div>
      ) : (
        <form action={formAction} className="space-y-4">
          <input type="hidden" name="provider_id" value={provider.id} />
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="风险评分" htmlFor="risk_score" hint="0=低风险，100=高风险">
              <Input id="risk_score" name="risk_score" type="number" min={0} max={100} defaultValue={20} required />
            </Field>
            <Field label="结论" htmlFor="decision">
              <Select id="decision" name="decision" defaultValue="approved">
                <option value="approved">批准</option>
                <option value="rejected">拒绝</option>
              </Select>
            </Field>
          </div>
          <fieldset>
            <legend className="mb-1.5 text-sm font-medium text-foreground">Go-live Checklist</legend>
            <div className="grid gap-2 rounded-lg border border-border bg-surface-1 p-3 sm:grid-cols-2">
              {RISK_CHECKS.map((check) => (
                <label key={check.key} className="flex items-center gap-2 text-sm">
                  <input type="checkbox" name={`check_${check.key}`} className="size-4 accent-[var(--brand-600)]" />
                  {check.label}
                </label>
              ))}
            </div>
          </fieldset>
          <Field label="审核人" htmlFor="reviewed_by">
            <Input id="reviewed_by" name="reviewed_by" defaultValue="operator" />
          </Field>
          <Field label="原因" htmlFor="reason">
            <Input id="reason" name="reason" placeholder="可选，写入审计" />
          </Field>
          {state.error && <Alert title="提交失败">{state.error}</Alert>}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" type="button" onClick={() => onOpenChange(false)}>取消</Button>
            <Button type="submit" loading={pending}>提交审核</Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

function CreateCellDialog({
  open,
  onOpenChange,
  regions,
  onSaved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  regions: Region[];
  onSaved: () => void;
}) {
  const { state, formAction, pending } = useActionFeedback<OpActionState>({
    action: createCellAction,
    initialState,
    successTitle: "Cell 已创建",
  });
  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="新建 Cell"
      description="创建区域共享或专用 Cell。"
      size="md"
    >
      {state.ok ? (
        <div className="space-y-4">
          <SuccessPanel title="Cell 已创建" />
          <div className="flex justify-end">
            <Button
              onClick={() => {
                onOpenChange(false);
                onSaved();
              }}
            >
              完成
            </Button>
          </div>
        </div>
      ) : (
        <form action={formAction} className="space-y-4">
          <Field label="Region" htmlFor="region_id">
            <Select id="region_id" name="region_id" defaultValue="">
              <option value="" disabled>选择 Region</option>
              {regions.map((r) => <option key={r.id} value={r.id}>{r.code} · {r.jurisdiction}</option>)}
            </Select>
          </Field>
          <Field label="Code" htmlFor="code">
            <Input id="code" name="code" required placeholder="cn-sh-01" />
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="类型" htmlFor="cell_type">
              <Select id="cell_type" name="cell_type" defaultValue="shared">
                <option value="shared">shared</option>
                <option value="dedicated">dedicated</option>
              </Select>
            </Field>
            <Field label="状态" htmlFor="cell_status">
              <Select id="cell_status" name="status" defaultValue="active">
                <option value="active">active</option>
                <option value="draining">draining</option>
                <option value="inactive">inactive</option>
              </Select>
            </Field>
          </div>
          {state.error && <Alert title="创建失败">{state.error}</Alert>}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" type="button" onClick={() => onOpenChange(false)}>取消</Button>
            <Button type="submit" loading={pending}>创建 Cell</Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

function CellStatusDialog({
  open,
  onOpenChange,
  cell,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  cell: Cell;
}) {
  const [state, formAction, pending] = useActionState(updateCellStatusAction, initialState);
  return (
    <Dialog open={open} onOpenChange={onOpenChange} title={`更新 ${cell.code}`} size="sm">
      <form action={formAction} className="space-y-4">
        <input type="hidden" name="cell_id" value={cell.id} />
        <Field label="状态" htmlFor="cell-status">
          <Select id="cell-status" name="status" defaultValue={cell.status}>
            <option value="active">active</option>
            <option value="draining">draining</option>
            <option value="inactive">inactive</option>
          </Select>
        </Field>
        {state.error && <Alert title="更新失败">{state.error}</Alert>}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" type="button" onClick={() => onOpenChange(false)}>取消</Button>
          <Button type="submit" loading={pending}>更新</Button>
        </div>
      </form>
    </Dialog>
  );
}
