"use client";

import { startTransition, useState } from "react";
import { useRouter } from "next/navigation";
import type {
  AuditChainState,
  AuditChainVerifyResult,
  AuditEvent,
  AuditStats,
} from "@/lib/api/operator";
import { formatDateTime } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import { Alert, EmptyState, ErrorState } from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import {
  ArrowRightIcon,
  ShieldIcon,
  RefreshIcon,
} from "@/components/ui/icons";
import {
  fetchAuditPageAction,
  verifyAuditChainAction,
} from "./audit-actions";

const ACTOR_STYLE: Record<string, "success" | "info" | "warning" | "neutral"> = {
  operator: "success",
  credential: "info",
  customer: "warning",
};

export function AuditClient({
  providerId,
  initialEvents,
  nextCursor,
  stats,
  chain,
  defaultFrom,
  defaultTo,
  loadError,
}: {
  providerId: string | null;
  initialEvents: AuditEvent[];
  nextCursor: number | null;
  stats: AuditStats | null;
  chain: AuditChainState | null;
  defaultFrom: string;
  defaultTo: string;
  loadError: string | null;
}) {
  const router = useRouter();
  const [events, setEvents] = useState<AuditEvent[]>(initialEvents);
  const [next, setNext] = useState<number | null>(nextCursor);
  const [pending, setPending] = useState(false);
  const [queryError, setQueryError] = useState<string | null>(null);
  const [filters, setFilters] = useState({
    action: "",
    actor_type: "",
    target_type: "",
    from: defaultFrom.slice(0, 10),
    to: defaultTo.slice(0, 10),
  });
  const [verify, setVerify] = useState<AuditChainVerifyResult | null>(null);
  const [verifyError, setVerifyError] = useState<string | null>(null);
  const [verifying, setVerifying] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);

  function runQuery(formData: FormData) {
    setPending(true);
    setQueryError(null);
    startTransition(async () => {
      const result = await fetchAuditPageAction(
        { ok: false, events: [], next_cursor: null },
        formData,
      );
      setEvents(result.events);
      setNext(result.next_cursor);
      setQueryError(result.error ?? null);
      setPending(false);
    });
  }

  function loadMore() {
    if (!providerId || next == null) return;
    setLoadingMore(true);
    const fd = new FormData();
    fd.set("provider_id", providerId);
    fd.set("cursor", String(next));
    fd.set("action", filters.action);
    fd.set("actor_type", filters.actor_type);
    fd.set("target_type", filters.target_type);
    fd.set("from", filters.from);
    fd.set("to", filters.to);
    startTransition(async () => {
      const result = await fetchAuditPageAction(
        { ok: false, events: [], next_cursor: null },
        fd,
      );
      if (result.ok) {
        setEvents((prev) => {
          const seen = new Set(prev.map((e) => e.id));
          return [...prev, ...result.events.filter((e) => !seen.has(e.id))];
        });
        setNext(result.next_cursor);
      }
      setLoadingMore(false);
    });
  }

  function runVerify() {
    setVerifying(true);
    setVerifyError(null);
    startTransition(async () => {
      const result = await verifyAuditChainAction();
      if (result.verify) setVerify(result.verify);
      else setVerifyError(result.error ?? "哈希链验证失败");
      setVerifying(false);
    });
  }

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">审计日志</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          检索 Provider 的不可变审计轨迹，并检查哈希链完整性。
        </p>
      </header>

      {loadError ? (
        <ErrorState title="审计日志加载失败" description={loadError} />
      ) : !providerId ? (
        <EmptyState
          icon={<ShieldIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，审计日志会记录关键操作。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : (
        <>
          {chain && <ChainPanel chain={chain} verify={verify} verifyError={verifyError} verifying={verifying} onVerify={runVerify} />}
          {stats && <StatsPanel stats={stats} />}

          <section className="rounded-2xl border border-border bg-surface-1 p-5 shadow-[var(--shadow-premium)]">
            <form
              onSubmit={(e) => {
                e.preventDefault();
                runQuery(new FormData(e.currentTarget));
              }}
              className="grid gap-3 md:grid-cols-6"
            >
              <input type="hidden" name="provider_id" value={providerId} />
              <Field label="动作" className="md:col-span-2">
                <Input
                  name="action"
                  value={filters.action}
                  onChange={(e) => setFilters((f) => ({ ...f, action: e.target.value }))}
                  placeholder="credential.create"
                />
              </Field>
              <Field label="执行者类型" className="md:col-span-2">
                <Select
                  name="actor_type"
                  value={filters.actor_type}
                  onChange={(e) => setFilters((f) => ({ ...f, actor_type: e.target.value }))}
                >
                  <option value="">全部</option>
                  <option value="operator">operator</option>
                  <option value="credential">credential</option>
                  <option value="customer">customer</option>
                </Select>
              </Field>
              <Field label="目标类型" className="md:col-span-2">
                <Input
                  name="target_type"
                  value={filters.target_type}
                  onChange={(e) => setFilters((f) => ({ ...f, target_type: e.target.value }))}
                  placeholder="credential"
                />
              </Field>
              <Field label="从" className="md:col-span-3">
                <Input
                  type="date"
                  name="from"
                  value={filters.from}
                  onChange={(e) => setFilters((f) => ({ ...f, from: e.target.value }))}
                />
              </Field>
              <Field label="到" className="md:col-span-3">
                <Input
                  type="date"
                  name="to"
                  value={filters.to}
                  onChange={(e) => setFilters((f) => ({ ...f, to: e.target.value }))}
                />
              </Field>
              <div className="flex items-end gap-2 md:col-span-6">
                <Button type="submit" loading={pending}>查询</Button>
                <Button type="button" variant="ghost" onClick={() => router.refresh()}>
                  重置
                </Button>
              </div>
            </form>
            {queryError && <Alert title="查询失败" className="mt-4">{queryError}</Alert>}
          </section>

          <AuditTable events={events} />

          {next != null && (
            <div className="flex justify-center">
              <Button variant="outline" onClick={loadMore} loading={loadingMore || pending}>
                加载更多
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function ChainPanel({
  chain,
  verify,
  verifyError,
  verifying,
  onVerify,
}: {
  chain: AuditChainState;
  verify: AuditChainVerifyResult | null;
  verifyError: string | null;
  verifying: boolean;
  onVerify: () => void;
}) {
  return (
    <section className="surface-premium rounded-2xl p-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
            Tamper-evident chain
          </p>
          <h2 className="mt-2 text-base font-semibold">审计哈希链</h2>
          <div className="mt-4 grid gap-x-8 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-4">
            <div>
              <p className="text-xs text-muted-foreground">事件总数</p>
              <p className="mt-1 font-mono text-lg font-semibold tabular-nums">{chain.total_events}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">尾事件</p>
              <p className="mt-1 font-mono text-xs tabular-nums">{chain.tail_event_id ?? "—"}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">最近锚点</p>
              <p className="mt-1 font-mono text-xs tabular-nums">{chain.last_anchor_id || "—"}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">尾哈希</p>
              <code className="mt-1 block max-w-56 truncate font-mono text-xs text-muted-foreground">
                {chain.tail_hash ?? "—"}
              </code>
            </div>
          </div>
        </div>
        <Button variant="outline" onClick={onVerify} loading={verifying}>
          <RefreshIcon size={14} aria-hidden="true" />
          验证完整性
        </Button>
      </div>
      {verify && (
        <div className="mt-4 rounded-xl border border-border bg-surface-2 px-4 py-3 text-sm">
          <Badge variant={verify.ok ? "success" : "danger"}>
            {verify.ok ? "链完整" : "检测到篡改"}
          </Badge>
          <span className="ml-3 font-mono text-xs text-muted-foreground">
            验证 {verify.verified_count} 条 · {verify.verified_from} → {verify.verified_to}
          </span>
          {!verify.ok && verify.broken_at != null && (
            <p className="mt-2 text-danger">断点事件：{verify.broken_at}（{verify.reason ?? "hash mismatch"}）</p>
          )}
        </div>
      )}
      {verifyError && <Alert title="验证失败" className="mt-4">{verifyError}</Alert>}
    </section>
  );
}

function StatsPanel({ stats }: { stats: AuditStats }) {
  return (
    <section className="grid gap-4 sm:grid-cols-3">
      <div className="surface-premium rounded-2xl p-5">
        <p className="text-xs text-muted-foreground">近 7 天事件</p>
        <p className="mt-2 font-mono text-2xl font-semibold tabular-nums">{stats.total}</p>
      </div>
      <div className="surface-premium rounded-2xl p-5">
        <p className="text-xs text-muted-foreground">高频动作</p>
        <div className="mt-3 space-y-1.5">
          {stats.by_action.slice(0, 3).map((row) => (
            <div key={row.key} className="flex justify-between gap-3 font-mono text-xs">
              <span className="truncate text-muted-foreground">{row.key}</span>
              <span className="tabular-nums">{row.count}</span>
            </div>
          ))}
        </div>
      </div>
      <div className="surface-premium rounded-2xl p-5">
        <p className="text-xs text-muted-foreground">执行者类型</p>
        <div className="mt-3 space-y-1.5">
          {stats.by_actor_type.slice(0, 3).map((row) => (
            <div key={row.key} className="flex justify-between gap-3 font-mono text-xs">
              <span className="text-muted-foreground">{row.key}</span>
              <span className="tabular-nums">{row.count}</span>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function AuditTable({ events }: { events: AuditEvent[] }) {
  const columns: DataTableColumn<AuditEvent>[] = [
    {
      key: "created_at",
      header: "时间",
      sortable: true,
      sortValue: (e) => e.created_at ?? "",
      cell: (e) => (
        <span className="whitespace-nowrap text-xs text-muted-foreground tabular-nums">
          {formatDateTime(e.created_at)}
        </span>
      ),
    },
    {
      key: "actor",
      header: "执行者",
      cell: (e) => (
        <div className="flex items-center gap-2">
          <Badge variant={ACTOR_STYLE[e.actor_type] ?? "neutral"}>{e.actor_type}</Badge>
          <code className="font-mono text-xs text-muted-foreground">{e.actor_id}</code>
        </div>
      ),
    },
    {
      key: "action",
      header: "动作",
      sortable: true,
      sortValue: (e) => e.action,
      cell: (e) => <code className="font-mono text-xs text-foreground">{e.action}</code>,
    },
    {
      key: "target",
      header: "目标",
      cell: (e) =>
        e.target_type ? (
          <span className="text-sm text-muted-foreground">
            {e.target_type}
            {e.target_id ? <code className="ml-1 font-mono text-[11px]">{e.target_id}</code> : null}
          </span>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
    {
      key: "request_id",
      header: "请求 ID",
      cell: (e) => (
        <code className="font-mono text-[11px] text-muted-foreground">{e.request_id ?? "—"}</code>
      ),
    },
    {
      key: "metadata",
      header: "元数据",
      cell: (e) =>
        e.metadata != null ? (
          <pre className="max-w-60 overflow-x-auto whitespace-pre-wrap font-mono text-[11px] text-muted-foreground">
            {JSON.stringify(e.metadata)}
          </pre>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
  ];

  return events.length === 0 ? (
    <EmptyState
      icon={<ShieldIcon size={20} aria-hidden="true" />}
      title="暂无审计记录"
      description="激活、生命周期转移等关键操作会记录在这里。"
    />
  ) : (
    <DataTable
      data={events}
      columns={columns}
      rowKey={(e) => String(e.id)}
      searchKeys={(e) => [e.action, e.actor_type, e.actor_id, e.target_type ?? "", e.request_id ?? ""]}
      defaultSort={{ key: "created_at", dir: "desc" }}
      emptyLabel="暂无审计记录"
    />
  );
}
