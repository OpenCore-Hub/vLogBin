"use client";

import { startTransition, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import type { PlatformEvent } from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { formatDate } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { Select } from "@/components/ui/field";
import { Dialog } from "@/components/ui/overlay";
import { CodeBlock } from "@/components/ui/code-block";
import { EmptyState, ErrorState, Alert, Spinner } from "@/components/ui/feedback";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { useEnv } from "@/components/console/env-provider";
import {
  ActivityIcon,
  ArrowRightIcon,
  EyeIcon,
} from "@/components/ui/icons";
import { fetchEventsAction } from "./events-actions";

const EVENT_TYPE_OPTIONS = [
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
  "plan.created",
  "plan.updated",
  "plan.published",
];

const AGGREGATE_OPTIONS = [
  "credential",
  "customer_account",
  "subscription",
  "usage_event",
  "invoice",
  "webhook",
  "plan",
];

const STATUS_VARIANT: Record<string, "success" | "neutral" | "warning" | "danger"> = {
  published: "success",
  pending: "warning",
  failed: "warning",
  dead_letter: "danger",
};

export function EventsClient({
  providerId,
  env,
  initial,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  initial: { events: PlatformEvent[]; next_cursor: string | null; has_more: boolean };
  loadError: string | null;
}) {
  const router = useRouter();
  const { env: activeEnv } = useEnv();
  const prevEnv = useRef(env);
  const [events, setEvents] = useState<PlatformEvent[]>(initial.events);
  const [nextCursor, setNextCursor] = useState(initial.next_cursor);
  const [hasMore, setHasMore] = useState(initial.has_more);
  const [type, setType] = useState("");
  const [aggregateType, setAggregateType] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<PlatformEvent | null>(null);

  useEffect(() => {
    if (prevEnv.current !== activeEnv) {
      prevEnv.current = activeEnv;
      router.refresh();
    }
  }, [activeEnv, router]);

  function load(cursor?: string, reset = false, nextType?: string, nextAgg?: string) {
    if (!providerId) return;
    setLoading(true);
    setError(null);
    const fd = new FormData();
    fd.set("provider_id", providerId);
    fd.set("env", env);
    if (cursor) fd.set("cursor", cursor);
    fd.set("type", nextType ?? type);
    fd.set("aggregate_type", nextAgg ?? aggregateType);
    startTransition(async () => {
      const state = await fetchEventsAction(
        { ok: false, events: [], next_cursor: null, has_more: false },
        fd,
      );
      if (!state.ok) {
        setError(state.error ?? "事件流加载失败");
        setLoading(false);
        return;
      }
      setEvents((prev) => (reset ? state.events : [...prev, ...state.events]));
      setNextCursor(state.next_cursor);
      setHasMore(state.has_more);
      setLoading(false);
    });
  }

  function changeType(value: string) {
    setType(value);
    load(undefined, true, value, aggregateType);
  }

  function changeAggregate(value: string) {
    setAggregateType(value);
    load(undefined, true, type, value);
  }

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">事件流</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          查看当前环境最近的平台事件。事件按时间正序排列，可加载下一页。
          当前环境为 {env === "test" ? "测试环境（沙箱）" : "生产环境（真实事件）"}。
        </p>
      </header>

      {loadError ? (
        <ErrorState
          title="事件流加载失败"
          description={loadError}
          action={
            <Button variant="outline" onClick={() => router.refresh()}>
              重试
            </Button>
          }
        />
      ) : !providerId ? (
        <EmptyState
          icon={<ActivityIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，获得测试环境后即可查看平台事件。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-3">
            <Select
              aria-label="事件类型"
              value={type}
              onChange={(e) => changeType(e.target.value)}
              className="h-9 w-56 text-sm"
            >
              <option value="">全部事件类型</option>
              {EVENT_TYPE_OPTIONS.map((event) => (
                <option key={event} value={event}>
                  {event}
                </option>
              ))}
            </Select>
            <Select
              aria-label="聚合类型"
              value={aggregateType}
              onChange={(e) => changeAggregate(e.target.value)}
              className="h-9 w-52 text-sm"
            >
              <option value="">全部聚合类型</option>
              {AGGREGATE_OPTIONS.map((aggregate) => (
                <option key={aggregate} value={aggregate}>
                  {aggregate}
                </option>
              ))}
            </Select>
            {loading && <Spinner size={14} label="加载事件流" />}
          </div>

          {error && <Alert title="加载失败">{error}</Alert>}

          {events.length === 0 ? (
            <EmptyState
              icon={<ActivityIcon size={20} aria-hidden="true" />}
              title="暂无事件"
              description="当平台发生客户、订阅、用量或账单变更时，事件会显示在这里。"
            />
          ) : (
            <div className="overflow-hidden rounded-xl border border-border bg-surface-1">
              <div className="max-h-[640px] overflow-auto">
                <table className="w-full text-sm">
                  <thead className="sticky top-0 z-10 bg-surface-2 shadow-[inset_0_-1px_0_theme(colors.border)]">
                    <tr className="text-left text-xs font-medium text-muted-foreground">
                      <th className="px-4 py-3 font-medium">事件</th>
                      <th className="px-4 py-3 font-medium">聚合</th>
                      <th className="px-4 py-3 font-medium">状态</th>
                      <th className="px-4 py-3 font-medium">尝试</th>
                      <th className="px-4 py-3 font-medium">时间</th>
                      <th className="px-4 py-3 font-medium" aria-label="详情" />
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border">
                    {events.map((event) => (
                      <tr
                        key={event.id}
                        className="transition-colors hover:bg-surface-2/60"
                      >
                        <td className="px-4 py-3">
                          <div className="flex flex-wrap items-center gap-2">
                            <code className="font-mono text-xs font-medium text-foreground">
                              {event.event_type}
                            </code>
                            <EnvBadge env={event.environment_kind} />
                          </div>
                          <code className="mt-1 block font-mono text-[11px] text-muted-foreground">
                            {event.transaction_id}
                          </code>
                        </td>
                        <td className="px-4 py-3">
                          <span className="block text-xs text-muted-foreground">
                            {event.aggregate_type}
                          </span>
                          <code className="mt-1 block font-mono text-[11px] text-muted-foreground">
                            {event.aggregate_id}
                          </code>
                        </td>
                        <td className="px-4 py-3">
                          <Badge variant={STATUS_VARIANT[event.status] ?? "neutral"}>
                            {event.status}
                          </Badge>
                          {event.last_error && (
                            <span className="mt-1 block max-w-52 truncate text-[11px] text-danger">
                              {event.last_error}
                            </span>
                          )}
                        </td>
                        <td className="px-4 py-3 text-right font-mono tabular-nums">
                          {event.attempts}
                        </td>
                        <td className="px-4 py-3 text-xs text-muted-foreground tabular-nums">
                          {formatDate(event.created_at)}
                        </td>
                        <td className="px-4 py-3 text-right">
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label={`查看事件 ${event.event_type}`}
                            onClick={() => setSelected(event)}
                          >
                            <EyeIcon size={14} aria-hidden="true" />
                            查看
                          </Button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {hasMore && (
            <div className="flex justify-center">
              <Button
                variant="outline"
                onClick={() => load(nextCursor ?? undefined)}
                loading={loading}
              >
                加载更多
              </Button>
            </div>
          )}
        </div>
      )}

      {selected && (
        <EventDetailDialog event={selected} onOpenChange={(open) => !open && setSelected(null)} />
      )}
    </div>
  );
}

function EventDetailDialog({
  event,
  onOpenChange,
}: {
  event: PlatformEvent;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog
      open
      onOpenChange={onOpenChange}
      title={event.event_type}
      description={`${event.aggregate_type} · ${event.transaction_id}`}
      size="lg"
    >
      <div className="space-y-4">
        <div className="flex flex-wrap gap-2">
          <Badge variant={STATUS_VARIANT[event.status] ?? "neutral"}>{event.status}</Badge>
          <EnvBadge env={event.environment_kind} />
          <span className="text-xs text-muted-foreground tabular-nums">
            {formatDate(event.created_at)}
          </span>
        </div>
        <CodeBlock
          title="payload"
          language="json"
          code={JSON.stringify(event.payload, null, 2)}
        />
      </div>
    </Dialog>
  );
}
