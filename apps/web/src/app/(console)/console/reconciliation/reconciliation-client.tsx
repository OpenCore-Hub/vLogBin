"use client";

import { useRouter } from "next/navigation";
import type { ReconciliationResult } from "@/lib/api/operator";
import { formatDateTime } from "@/lib/format";
import { Button } from "@/components/ui/button";
import { EmptyState, ErrorState } from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import { ActivityIcon } from "@/components/ui/icons";

const STATUS_VARIANT: Record<string, "success" | "warning" | "danger" | "neutral"> = {
  ok: "success",
  drift: "warning",
  error: "danger",
};

export function ReconciliationClient({
  results,
  loadError,
}: {
  results: ReconciliationResult[];
  loadError: string | null;
}) {
  const router = useRouter();
  const ok = results.filter((r) => r.status === "ok").length;
  const drift = results.filter((r) => r.status === "drift").length;
  const error = results.filter((r) => r.status === "error").length;

  const summary = [
    { label: "通过", value: String(ok) },
    { label: "漂移", value: String(drift) },
    { label: "错误", value: String(error) },
    { label: "检查数", value: String(results.length) },
  ];

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">对账</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          财务、用量与 Outbox 一致性检查结果，按最近运行时间展示。
        </p>
      </header>

      {loadError ? (
        <ErrorState
          title="对账结果加载失败"
          description={loadError}
          action={
            <Button variant="outline" onClick={() => router.refresh()}>
              重试
            </Button>
          }
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

          {results.length === 0 ? (
            <EmptyState
              icon={<ActivityIcon size={20} aria-hidden="true" />}
              title="暂无对账结果"
              description="对账 worker 运行后，一致性检查结果会显示在这里。"
            />
          ) : (
            <ResultTable results={results} />
          )}
        </>
      )}
    </div>
  );
}

function ResultTable({ results }: { results: ReconciliationResult[] }) {
  const columns: DataTableColumn<ReconciliationResult>[] = [
    {
      key: "check_name",
      header: "检查",
      sortable: true,
      sortValue: (r) => r.check_name,
      cell: (r) => (
        <code className="font-mono text-xs text-foreground">{r.check_name}</code>
      ),
    },
    {
      key: "status",
      header: "状态",
      cell: (r) => (
        <Badge variant={STATUS_VARIANT[r.status] ?? "neutral"}>{r.status}</Badge>
      ),
    },
    {
      key: "expected_count",
      header: "预期",
      numeric: true,
      sortable: true,
      sortValue: (r) => r.expected_count,
      cell: (r) => String(r.expected_count),
    },
    {
      key: "actual_count",
      header: "实际",
      numeric: true,
      sortable: true,
      sortValue: (r) => r.actual_count,
      cell: (r) => String(r.actual_count),
    },
    {
      key: "drift_count",
      header: "漂移",
      numeric: true,
      sortable: true,
      sortValue: (r) => r.drift_count,
      cell: (r) => String(r.drift_count),
    },
    {
      key: "checked_at",
      header: "检查时间",
      sortable: true,
      sortValue: (r) => r.checked_at ?? "",
      cell: (r) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatDateTime(r.checked_at)}
        </span>
      ),
    },
  ];

  return (
    <DataTable
      data={results}
      columns={columns}
      rowKey={(r) => r.id}
      searchKeys={(r) => [r.check_name, r.status]}
      filters={[
        {
          key: "status",
          label: "状态",
          options: [
            { value: "ok", label: "通过" },
            { value: "drift", label: "漂移" },
            { value: "error", label: "错误" },
          ],
          predicate: (r, value) => r.status === value,
        },
      ]}
      defaultSort={{ key: "checked_at", dir: "desc" }}
      emptyLabel="暂无对账结果"
    />
  );
}
