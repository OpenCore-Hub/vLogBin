"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import type { Env } from "@/lib/env-shared";
import type {
  QuotaLimitUsage,
  QuotaReservation,
  Subscription,
} from "@/lib/api/operator";
import { formatDate } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import { Dialog, ConfirmDialog } from "@/components/ui/overlay";
import {
  EmptyState,
  ErrorState,
  Alert,
} from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import { useEnv } from "@/components/console/env-provider";
import { useActionFeedback } from "@/hooks/use-action-feedback";
import {
  ArrowRightIcon,
  EditIcon,
  LayersIcon,
  PlusIcon,
  TrashIcon,
} from "@/components/ui/icons";
import {
  deleteQuotaLimitAction,
  setQuotaLimitAction,
  type QuotaActionState,
} from "./quota-actions";

const initialState: QuotaActionState = { ok: false };

const PERIOD_LABEL: Record<string, string> = {
  daily: "每日",
  monthly: "每月",
  total: "总量",
};

export function QuotaClient({
  providerId,
  env,
  subscriptions,
  selectedSubscription,
  quotaLimits,
  reservations,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  subscriptions: Subscription[];
  selectedSubscription: Subscription | null;
  quotaLimits: QuotaLimitUsage[];
  reservations: QuotaReservation[];
  loadError: string | null;
}) {
  const router = useRouter();
  const { env: activeEnv } = useEnv();
  const prevEnv = useRef(env);
  const [limitDialog, setLimitDialog] = useState<{
    mode: "create" | "edit";
    limit?: QuotaLimitUsage;
  } | null>(null);
  const [deleteLimit, setDeleteLimit] = useState<QuotaLimitUsage | null>(null);

  useEffect(() => {
    if (prevEnv.current !== activeEnv) {
      prevEnv.current = activeEnv;
      router.refresh();
    }
  }, [activeEnv, router]);

  const committed = quotaLimits.reduce((sum, q) => sum + q.committed, 0);
  const reserved = quotaLimits.reduce((sum, q) => sum + q.reserved, 0);
  const summary = [
    { label: "额度上限", value: String(quotaLimits.length) },
    { label: "已用", value: committed.toLocaleString("en-US") },
    { label: "预占", value: reserved.toLocaleString("en-US") },
    { label: "订阅数", value: String(subscriptions.length) },
  ];

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">额度</h1>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
            查看订阅的硬额度上限与持久化预占账本。
            当前环境为 {env === "test" ? "测试环境（沙箱）" : "生产环境"}。
          </p>
        </div>
        {providerId && subscriptions.length > 0 && (
          <Button onClick={() => setLimitDialog({ mode: "create" })}>
            <PlusIcon size={16} aria-hidden="true" />
            新建额度
          </Button>
        )}
      </header>

      {loadError ? (
        <ErrorState
          title="额度数据加载失败"
          description={loadError}
          action={
            <Button variant="outline" onClick={() => router.refresh()}>
              重试
            </Button>
          }
        />
      ) : !providerId ? (
        <EmptyState
          icon={<LayersIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，再配置订阅额度。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : subscriptions.length === 0 ? (
        <EmptyState
          icon={<LayersIcon size={20} aria-hidden="true" />}
          title="暂无订阅"
          description="客户订阅套餐后，可在这里配置硬额度。"
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

          <section className="space-y-4">
            <div className="flex flex-wrap items-center gap-3">
              <Field label="订阅" className="min-w-[260px]">
                <Select
                  aria-label="订阅"
                  value={selectedSubscription?.id ?? ""}
                  onChange={(e) => {
                    if (e.target.value) {
                      router.push(
                        `/console/billing/quota?env=${env}&subscription=${e.target.value}`,
                      );
                    }
                  }}
                >
                  {subscriptions.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.external_id} · {s.plan_code}
                    </option>
                  ))}
                </Select>
              </Field>
              {selectedSubscription && (
                <span className="text-sm text-muted-foreground">
                  {selectedSubscription.customer_external_id}
                </span>
              )}
            </div>

            <QuotaLimitsTable
              limits={quotaLimits}
              onEdit={(limit) => setLimitDialog({ mode: "edit", limit })}
              onDelete={setDeleteLimit}
            />
          </section>

          <section className="space-y-4">
            <div>
              <h2 className="text-sm font-semibold">预占账本</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                持久化 reservation ledger，记录 reserve / commit / release / expire。
              </p>
            </div>
            <QuotaReservationsTable reservations={reservations} />
          </section>
        </>
      )}

      {limitDialog && selectedSubscription && (
        <QuotaLimitDialog
          key={limitDialog.mode + (limitDialog.limit?.quota_key ?? "")}
          open={!!limitDialog}
          onOpenChange={(open) => {
            if (!open) setLimitDialog(null);
          }}
          providerId={providerId}
          subscriptionId={selectedSubscription.id}
          env={env}
          limit={limitDialog.limit}
          onSaved={() => router.refresh()}
        />
      )}

      {deleteLimit && selectedSubscription && (
        <DeleteQuotaLimitDialog
          key={deleteLimit.quota_key}
          open={!!deleteLimit}
          onOpenChange={(open) => {
            if (!open) setDeleteLimit(null);
          }}
          providerId={providerId}
          subscriptionId={selectedSubscription.id}
          env={env}
          limit={deleteLimit}
          onSaved={() => router.refresh()}
        />
      )}
    </div>
  );
}

function QuotaLimitsTable({
  limits,
  onEdit,
  onDelete,
}: {
  limits: QuotaLimitUsage[];
  onEdit: (limit: QuotaLimitUsage) => void;
  onDelete: (limit: QuotaLimitUsage) => void;
}) {
  const columns: DataTableColumn<QuotaLimitUsage>[] = [
    {
      key: "quota_key",
      header: "额度键",
      sortable: true,
      sortValue: (q) => q.quota_key,
      cell: (q) => <code className="font-mono text-xs">{q.quota_key}</code>,
    },
    {
      key: "period_type",
      header: "周期",
      cell: (q) => (
        <Badge variant="info">{PERIOD_LABEL[q.period_type] ?? q.period_type}</Badge>
      ),
    },
    {
      key: "limit_value",
      header: "上限",
      numeric: true,
      sortable: true,
      sortValue: (q) => q.limit_value,
      cell: (q) => (
        <span className="tabular-nums">{q.limit_value.toLocaleString("en-US")}</span>
      ),
    },
    {
      key: "committed",
      header: "已用",
      numeric: true,
      sortable: true,
      sortValue: (q) => q.committed,
      cell: (q) => (
        <span className="tabular-nums">{q.committed.toLocaleString("en-US")}</span>
      ),
    },
    {
      key: "reserved",
      header: "预占",
      numeric: true,
      sortable: true,
      sortValue: (q) => q.reserved,
      cell: (q) => (
        <span className="tabular-nums">{q.reserved.toLocaleString("en-US")}</span>
      ),
    },
    {
      key: "usage_pct",
      header: "占用率",
      numeric: true,
      cell: (q) => {
        const used = q.committed + q.reserved;
        const pct = q.limit_value > 0 ? Math.round((used / q.limit_value) * 100) : 0;
        return <span className="tabular-nums">{pct}%</span>;
      },
    },
    {
      key: "actions",
      header: <span className="sr-only">操作</span>,
      className: "text-right",
      cell: (q) => (
        <div className="flex items-center justify-end gap-1">
          <Button variant="ghost" size="sm" onClick={() => onEdit(q)}>
            <EditIcon size={14} aria-hidden="true" />
            编辑
          </Button>
          <Button variant="ghost" size="sm" onClick={() => onDelete(q)}>
            <TrashIcon size={14} aria-hidden="true" />
            删除
          </Button>
        </div>
      ),
    },
  ];

  return (
    <DataTable
      data={limits}
      columns={columns}
      rowKey={(q) => q.id}
      searchKeys={(q) => [q.quota_key, q.period_type]}
      defaultSort={{ key: "quota_key", dir: "asc" }}
      emptyLabel="暂无额度上限"
    />
  );
}

function QuotaReservationsTable({
  reservations,
}: {
  reservations: QuotaReservation[];
}) {
  const columns: DataTableColumn<QuotaReservation>[] = [
    {
      key: "quota_key",
      header: "额度键",
      cell: (r) => <code className="font-mono text-xs">{r.quota_key}</code>,
    },
    {
      key: "amount",
      header: "数量",
      numeric: true,
      sortable: true,
      sortValue: (r) => r.amount,
      cell: (r) => <span className="tabular-nums">{r.amount.toLocaleString("en-US")}</span>,
    },
    {
      key: "status",
      header: "状态",
      sortable: true,
      sortValue: (r) => r.status,
      cell: (r) => (
        <Badge
          variant={
            r.status === "committed"
              ? "success"
              : r.status === "reserved"
                ? "warning"
                : "neutral"
          }
        >
          {r.status}
        </Badge>
      ),
    },
    {
      key: "reservation_id",
      header: "幂等键",
      cell: (r) => (
        <span className="font-mono text-[11px] text-muted-foreground">
          {r.reservation_id}
        </span>
      ),
    },
    {
      key: "expires_at",
      header: "过期时间",
      sortable: true,
      sortValue: (r) => r.expires_at ?? "",
      cell: (r) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatDate(r.expires_at)}
        </span>
      ),
    },
    {
      key: "created_at",
      header: "创建时间",
      sortable: true,
      sortValue: (r) => r.created_at ?? "",
      cell: (r) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatDate(r.created_at)}
        </span>
      ),
    },
  ];

  return (
    <DataTable
      data={reservations}
      columns={columns}
      rowKey={(r) => r.id}
      searchKeys={(r) => [r.quota_key, r.status, r.reservation_id]}
      defaultSort={{ key: "created_at", dir: "desc" }}
      emptyLabel="暂无预占记录"
    />
  );
}

function QuotaLimitDialog({
  open,
  onOpenChange,
  providerId,
  subscriptionId,
  env,
  limit,
  onSaved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerId: string | null;
  subscriptionId: string;
  env: Env;
  limit?: QuotaLimitUsage;
  onSaved: () => void;
}) {
  const { state, formAction, pending } = useActionFeedback<QuotaActionState>({
    action: setQuotaLimitAction,
    initialState,
    onSuccess: () => {
      onOpenChange(false);
      onSaved();
    },
    successTitle: limit ? "额度已更新" : "额度已创建",
    errorTitle: "操作失败",
  });

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={limit ? `编辑 ${limit.quota_key}` : "新建额度"}
      description="硬额度由持久化预占账本强制执行，超出即拒绝。"
      size="md"
    >
      {state.ok ? (
        <div className="space-y-4">
          <p className="text-sm text-muted-foreground">
            {limit ? "额度已更新。" : "额度已创建。"}
          </p>
          <div className="flex justify-end">
            <Button onClick={() => onOpenChange(false)}>完成</Button>
          </div>
        </div>
      ) : (
        <form action={formAction} className="space-y-4">
          <input type="hidden" name="provider_id" value={providerId ?? ""} />
          <input type="hidden" name="subscription_id" value={subscriptionId} />
          <input type="hidden" name="env" value={env} />
          {limit && <input type="hidden" name="quota_key" value={limit.quota_key} />}
          <Field label="额度键" htmlFor="quota_key" hint="如 api_calls / storage_gb">
            <Input
              id="quota_key"
              name="quota_key"
              defaultValue={limit?.quota_key}
              readOnly={!!limit}
              required
              placeholder="api_calls"
            />
          </Field>
          <Field label="上限" htmlFor="limit_value" hint="0 表示禁止任何预占">
            <Input
              id="limit_value"
              name="limit_value"
              type="number"
              min={0}
              step={1}
              defaultValue={limit?.limit_value ?? 1000}
              required
            />
          </Field>
          <Field label="周期" htmlFor="period_type">
            <Select
              id="period_type"
              name="period_type"
              defaultValue={limit?.period_type ?? "monthly"}
            >
              <option value="daily">每日</option>
              <option value="monthly">每月</option>
              <option value="total">总量</option>
            </Select>
          </Field>
          {state.error && <Alert title="操作失败">{state.error}</Alert>}
          <div className="flex justify-end gap-2">
            <Button
              variant="ghost"
              type="button"
              onClick={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button type="submit" loading={pending}>
              {limit ? "保存额度" : "创建额度"}
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

function DeleteQuotaLimitDialog({
  open,
  onOpenChange,
  providerId,
  subscriptionId,
  env,
  limit,
  onSaved,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerId: string | null;
  subscriptionId: string;
  env: Env;
  limit: QuotaLimitUsage;
  onSaved: () => void;
}) {
  const { state, formAction, pending } = useActionFeedback<QuotaActionState>({
    action: deleteQuotaLimitAction,
    initialState,
    onSuccess: () => {
      onOpenChange(false);
      onSaved();
    },
    successTitle: "额度已删除",
    errorTitle: "删除失败",
  });

  function confirm() {
    const fd = new FormData();
    fd.set("provider_id", providerId ?? "");
    fd.set("subscription_id", subscriptionId);
    fd.set("env", env);
    fd.set("quota_key", limit.quota_key);
    formAction(fd);
  }

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title="删除额度上限"
      description={
        <div className="space-y-2">
          <p>
            删除 {limit.quota_key} 后，新的预占会被拒绝；已提交的预占不受影响。
          </p>
          {state.error && <Alert title="删除失败">{state.error}</Alert>}
        </div>
      }
      confirmText={limit.quota_key}
      pending={pending}
      onConfirm={confirm}
      confirmLabel="删除额度"
    />
  );
}
