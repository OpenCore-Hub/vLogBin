"use client";

import { useEffect, useRef, useState } from "react";
import { useActionState } from "react";
import { useRouter } from "next/navigation";
import type {
  WebhookDelivery,
  WebhookEndpoint,
} from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { formatDate } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { Dialog, ConfirmDialog } from "@/components/ui/overlay";
import { CopyButton } from "@/components/ui/code-block";
import { EmptyState, ErrorState, Alert, SuccessPanel } from "@/components/ui/feedback";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import { Tabs, TabPanel } from "@/components/ui/tabs";
import { useEnv } from "@/components/console/env-provider";
import {
  ArrowRightIcon,
  PlusIcon,
  RefreshIcon,
  TrashIcon,
  WebhookIcon,
} from "@/components/ui/icons";
import { useActionFeedback } from "@/hooks/use-action-feedback";
import {
  createWebhookAction,
  deleteWebhookAction,
  replayWebhookDeliveryAction,
  type WebhookActionState,
} from "./webhooks-actions";

const initialState: WebhookActionState = { ok: false };

const EVENT_OPTIONS = [
  "customer.created",
  "subscription.created",
  "subscription.terminated",
  "usage.accepted",
  "usage.reversed",
  "invoice.synced",
  "credential.created",
  "credential.revoked",
  "webhook.endpoint_created",
  "webhook.endpoint_deleted",
];

const DELIVERY_STATUS: Record<string, "success" | "neutral" | "warning" | "danger"> = {
  delivered: "success",
  pending: "warning",
  failed: "warning",
  dead_letter: "danger",
};

export function WebhooksClient({
  providerId,
  env,
  endpoints,
  deliveries,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  endpoints: WebhookEndpoint[];
  deliveries: WebhookDelivery[];
  loadError: string | null;
}) {
  const router = useRouter();
  const { env: activeEnv } = useEnv();
  const prevEnv = useRef(env);
  const [tab, setTab] = useState("endpoints");
  const [createOpen, setCreateOpen] = useState(false);
  const [createNonce, setCreateNonce] = useState(0);
  const [deleting, setDeleting] = useState<WebhookEndpoint | null>(null);
  const [replaying, setReplaying] = useState<WebhookDelivery | null>(null);

  useEffect(() => {
    if (prevEnv.current !== activeEnv) {
      prevEnv.current = activeEnv;
      router.refresh();
    }
  }, [activeEnv, router]);

  function openCreate() {
    setCreateNonce((n) => n + 1);
    setCreateOpen(true);
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Webhooks</h1>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
            把平台事件实时推送到你的服务端点。每次投递都带签名头，可重试。
            当前环境为 {env === "test" ? "测试环境（沙箱）" : "生产环境（真实事件）"}。
          </p>
        </div>
        {providerId && (
          <Button onClick={openCreate}>
            <PlusIcon size={16} aria-hidden="true" />
            创建端点
          </Button>
        )}
      </header>

      {loadError ? (
        <ErrorState
          title="Webhook 数据加载失败"
          description={loadError}
          action={
            <Button variant="outline" onClick={() => router.refresh()}>
              重试
            </Button>
          }
        />
      ) : !providerId ? (
        <EmptyState
          icon={<WebhookIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，获得测试环境后即可配置 Webhook。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : (
        <>
          <Tabs
            value={tab}
            onChange={setTab}
            items={[
              { value: "endpoints", label: "端点" },
              { value: "deliveries", label: "投递记录" },
            ]}
          />
          <TabPanel id="webhooks" value="endpoints" selected={tab === "endpoints"}>
            {endpoints.length === 0 ? (
              <EmptyState
                icon={<WebhookIcon size={20} aria-hidden="true" />}
                title="还没有 Webhook 端点"
                description="创建端点后，平台事件会按所选事件类型推送到你的 URL。"
                action={
                  <Button onClick={openCreate}>
                    <PlusIcon size={16} aria-hidden="true" />
                    创建第一个端点
                  </Button>
                }
              />
            ) : (
              <EndpointTable
                endpoints={endpoints}
                env={env}
                onDelete={setDeleting}
              />
            )}
          </TabPanel>
          <TabPanel id="webhooks" value="deliveries" selected={tab === "deliveries"}>
            <DeliveryTable
              deliveries={deliveries}
              onReplay={setReplaying}
            />
          </TabPanel>
        </>
      )}

      {providerId && (
        <CreateWebhookDialog
          key={`create-${createNonce}`}
          open={createOpen}
          onOpenChange={setCreateOpen}
          providerId={providerId}
          env={env}
        />
      )}
      {providerId && deleting && (
        <DeleteWebhookDialog
          open={!!deleting}
          onOpenChange={(open) => {
            if (!open) setDeleting(null);
          }}
          providerId={providerId}
          env={env}
          endpoint={deleting}
        />
      )}
      {providerId && replaying && (
        <ReplayDeliveryDialog
          open={!!replaying}
          onOpenChange={(open) => {
            if (!open) setReplaying(null);
          }}
          providerId={providerId}
          delivery={replaying}
        />
      )}
    </div>
  );
}

function EndpointTable({
  endpoints,
  env,
  onDelete,
}: {
  endpoints: WebhookEndpoint[];
  env: Env;
  onDelete: (endpoint: WebhookEndpoint) => void;
}) {
  const columns: DataTableColumn<WebhookEndpoint>[] = [
    {
      key: "url",
      header: "回调 URL",
      sortable: true,
      sortValue: (e) => e.url,
      cell: (e) => (
        <code className="font-mono text-xs text-foreground break-all">{e.url}</code>
      ),
    },
    {
      key: "events",
      header: "事件",
      cell: (e) => (
        <div className="flex max-w-md flex-wrap gap-1">
          {e.events.length === 0 ? (
            <span className="text-xs text-muted-foreground">全部事件</span>
          ) : (
            e.events.map((event) => (
              <code
                key={event}
                className="rounded-md bg-surface-2 px-1.5 py-0.5 font-mono text-[11px] text-foreground"
              >
                {event}
              </code>
            ))
          )}
        </div>
      ),
    },
    {
      key: "status",
      header: "状态",
      cell: (e) => (
        <Badge variant={e.enabled ? "success" : "neutral"}>
          {e.enabled ? "已启用" : "已停用"}
        </Badge>
      ),
    },
    {
      key: "environment",
      header: "环境",
      cell: () => <EnvBadge env={env} />,
    },
    {
      key: "created_at",
      header: "创建时间",
      sortable: true,
      sortValue: (e) => e.created_at ?? "",
      cell: (e) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatDate(e.created_at)}
        </span>
      ),
    },
    {
      key: "actions",
      header: <span className="sr-only">操作</span>,
      className: "text-right",
      cell: (e) => (
        <Button
          variant="ghost"
          size="sm"
          onClick={() => onDelete(e)}
          aria-label={`删除 ${e.url}`}
        >
          <TrashIcon size={14} aria-hidden="true" />
          删除
        </Button>
      ),
    },
  ];

  return (
    <DataTable
      data={endpoints}
      columns={columns}
      rowKey={(e) => e.id}
      searchKeys={(e) => [e.url, ...e.events]}
      defaultSort={{ key: "created_at", dir: "desc" }}
      emptyLabel="暂无端点"
    />
  );
}

function DeliveryTable({
  deliveries,
  onReplay,
}: {
  deliveries: WebhookDelivery[];
  onReplay: (delivery: WebhookDelivery) => void;
}) {
  const columns: DataTableColumn<WebhookDelivery>[] = [
    {
      key: "status",
      header: "状态",
      sortable: true,
      sortValue: (d) => d.status,
      cell: (d) => (
        <Badge variant={DELIVERY_STATUS[d.status] ?? "neutral"}>{d.status}</Badge>
      ),
    },
    {
      key: "endpoint",
      header: "端点 ID",
      cell: (d) => (
        <code className="font-mono text-xs text-muted-foreground">{d.endpoint_id}</code>
      ),
    },
    {
      key: "event",
      header: "事件 ID",
      cell: (d) => (
        <code className="font-mono text-xs text-muted-foreground">{d.outbox_event_id}</code>
      ),
    },
    {
      key: "attempts",
      header: "尝试",
      numeric: true,
      sortable: true,
      sortValue: (d) => d.attempts,
      cell: (d) => <span className="tabular-nums">{d.attempts}</span>,
    },
    {
      key: "response_status",
      header: "响应",
      numeric: true,
      cell: (d) => (
        <span className="tabular-nums">{d.response_status ?? "—"}</span>
      ),
    },
    {
      key: "created_at",
      header: "创建时间",
      sortable: true,
      sortValue: (d) => d.created_at ?? "",
      cell: (d) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatDate(d.created_at)}
        </span>
      ),
    },
    {
      key: "actions",
      header: <span className="sr-only">操作</span>,
      className: "text-right",
      cell: (d) =>
        d.status === "dead_letter" || d.status === "failed" ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onReplay(d)}
            aria-label="重放投递"
          >
            <RefreshIcon size={14} aria-hidden="true" />
            重放
          </Button>
        ) : null,
    },
  ];

  return (
    <DataTable
      data={deliveries}
      columns={columns}
      rowKey={(d) => d.id}
      searchKeys={(d) => [d.id, d.endpoint_id, d.outbox_event_id, d.status]}
      defaultSort={{ key: "created_at", dir: "desc" }}
      emptyLabel="暂无投递记录"
    />
  );
}

function CreateWebhookDialog({
  open,
  onOpenChange,
  providerId,
  env,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerId: string;
  env: Env;
}) {
  const [state, formAction, pending] = useActionState(createWebhookAction, initialState);

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="创建 Webhook 端点"
      description={`端点将创建在${env === "test" ? "测试环境" : "生产环境"}。`}
      size="lg"
    >
      {state.ok && state.endpoint?.secret ? (
        <div className="space-y-4">
          <SuccessPanel
            title="Webhook 端点创建成功"
            description="签名密钥只显示一次，请立即复制并配置到接收端。"
          >
            <div className="mt-3 flex items-center gap-2 rounded-md border border-border bg-surface-2 p-3">
              <code className="min-w-0 flex-1 truncate font-mono text-xs text-foreground">
                {state.endpoint.secret}
              </code>
              <CopyButton text={state.endpoint.secret} label="复制签名密钥" />
            </div>
          </SuccessPanel>
          <div className="flex justify-end">
            <Button onClick={() => onOpenChange(false)}>完成</Button>
          </div>
        </div>
      ) : (
        <form action={formAction} className="space-y-4">
          <input type="hidden" name="provider_id" value={providerId} />
          <input type="hidden" name="env" value={env} />
          <Field label="回调 URL" htmlFor="url" hint="必须是可公网访问的 HTTPS 或 HTTP 端点。">
            <Input
              id="url"
              name="url"
              type="url"
              required
              autoComplete="off"
              placeholder="https://api.example.com/hooks/vlogbin"
            />
          </Field>
          <fieldset>
            <legend className="mb-1.5 text-sm font-medium text-foreground">订阅事件</legend>
            <div className="grid gap-2 rounded-lg border border-border bg-surface-1 p-3 sm:grid-cols-2">
              {EVENT_OPTIONS.map((event) => (
                <label key={event} className="flex items-center gap-2.5 text-sm">
                  <input
                    type="checkbox"
                    name="events"
                    value={event}
                    className="size-4 accent-[var(--brand-600)]"
                  />
                  <code className="font-mono text-xs text-foreground">{event}</code>
                </label>
              ))}
            </div>
          </fieldset>
          <Field label="签名密钥（可选）" htmlFor="secret" hint="留空时平台自动生成 32 字节随机密钥。">
            <Input id="secret" name="secret" autoComplete="off" placeholder="留空自动生成" />
          </Field>
          {state.error && <Alert title="创建失败">{state.error}</Alert>}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" type="button" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" loading={pending}>
              创建端点
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

function DeleteWebhookDialog({
  open,
  onOpenChange,
  providerId,
  env,
  endpoint,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerId: string;
  env: Env;
  endpoint: WebhookEndpoint;
}) {
  const router = useRouter();
  const { state, formAction, pending } = useActionFeedback<WebhookActionState>({
    action: deleteWebhookAction,
    initialState,
    onSuccess: () => {
      onOpenChange(false);
      router.refresh();
    },
    successTitle: "Webhook 端点已删除",
  });

  function confirm() {
    const fd = new FormData();
    fd.set("provider_id", providerId);
    fd.set("env", env);
    fd.set("webhook_id", endpoint.id);
    formAction(fd);
  }

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title="删除 Webhook 端点"
      description={
        <div className="space-y-2">
          <p>
            删除后 <span className="break-all font-mono text-xs text-foreground">{endpoint.url}</span>{" "}
            将不再收到任何事件。
          </p>
          {state.error && <Alert title="删除失败">{state.error}</Alert>}
        </div>
      }
      confirmText={endpoint.url}
      pending={pending}
      onConfirm={confirm}
      confirmLabel="删除端点"
    />
  );
}

function ReplayDeliveryDialog({
  open,
  onOpenChange,
  providerId,
  delivery,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerId: string;
  delivery: WebhookDelivery;
}) {
  const router = useRouter();
  const { state, formAction, pending } = useActionFeedback<WebhookActionState>({
    action: replayWebhookDeliveryAction,
    initialState,
    onSuccess: () => {
      onOpenChange(false);
      router.refresh();
    },
    successTitle: "投递已重新入队",
  });

  function confirm() {
    const fd = new FormData();
    fd.set("provider_id", providerId);
    fd.set("delivery_id", delivery.id);
    formAction(fd);
  }

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title="重放 Webhook 投递"
      description={
        <div className="space-y-2">
          <p>
            将失败/死信的投递重新入队，尝试次数清零。事件会在下一轮立即重发。
          </p>
          {state.error && <Alert title="重放失败">{state.error}</Alert>}
        </div>
      }
      pending={pending}
      onConfirm={confirm}
      confirmLabel="重放投递"
    />
  );
}
