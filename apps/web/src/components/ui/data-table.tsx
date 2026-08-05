"use client";

import {
  useCallback,
  useDeferredValue,
  useEffect,
  useMemo,
  useRef,
  type ReactNode,
} from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { cn } from "@/lib/utils";
import { Select } from "./field";
import { Pagination } from "./pagination";
import {
  ChevronDownIcon,
  SearchIcon,
  XIcon,
} from "./icons";

export type SortDir = "asc" | "desc";

export type DataTableColumn<T> = {
  key: string;
  header: ReactNode;
  cell: (row: T) => ReactNode;
  /** 可排序列需提供可比较值（字符串或数字）。 */
  sortable?: boolean;
  sortValue?: (row: T) => string | number;
  /** 数值列右对齐 + mono + tabular-nums（§7.4）。 */
  numeric?: boolean;
  className?: string;
};

export type DataTableProps<T> = {
  data: T[];
  columns: DataTableColumn<T>[];
  rowKey: (row: T) => string;
  /** 搜索键：返回该行所有可被 q 匹配的字符串。 */
  searchKeys: (row: T) => string[];
  filters?: DataTableFilter<T>[];
  defaultSort?: { key: string; dir: SortDir };
  defaultPageSize?: number;
  pageSizeOptions?: number[];
  empty?: ReactNode;
  className?: string;
  /** 默认“暂无数据”。 */
  emptyLabel?: string;
};

export type DataTableFilter<T> = {
  key: string;
  label: string;
  options: Array<{ value: string; label: string }>;
  predicate: (row: T, value: string) => boolean;
};

function num(v: unknown, fallback: number): number {
  const n = Number(v);
  return Number.isFinite(n) && n > 0 ? Math.floor(n) : fallback;
}

/**
 * DataTable（§7.4 / R16）：sticky 表头、行 hover、分页 + 每页条数选择，
 * 搜索/排序/分页全部写入 URL（?q=&sort=&dir=&page=&pageSize=），可分享、
 * 可回退。列表在客户端过滤排序，服务端页面保持 force-dynamic。
 */
export function DataTable<T>({
  data,
  columns,
  rowKey,
  searchKeys,
  filters = [],
  defaultSort,
  defaultPageSize = 10,
  pageSizeOptions = [10, 25, 50],
  empty,
  className,
  emptyLabel = "暂无数据",
}: DataTableProps<T>) {
  const searchParams = useSearchParams();
  const pathname = usePathname();
  const router = useRouter();

  const q = searchParams.get("q") ?? "";
  const filterValues = useMemo(
    () =>
      Object.fromEntries(
        filters.map((f) => [f.key, searchParams.get(f.key) ?? ""]),
      ),
    [filters, searchParams],
  );
  const sortKey = searchParams.get("sort") ?? defaultSort?.key ?? "";
  const sortDir: SortDir = searchParams.get("dir") === "asc" ? "asc" : "desc";
  const page = num(searchParams.get("page"), 1);
  const pageSize = num(searchParams.get("pageSize"), defaultPageSize);
  const deferredQ = useDeferredValue(q);
  const searchValueRef = useRef("");
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const updateParams = useCallback(
    (
      changes: Record<string, string | null>,
      mode: "push" | "replace" = "push",
    ) => {
      const next = new URLSearchParams(searchParams.toString());
      for (const [key, value] of Object.entries(changes)) {
        if (value === null || value === "") next.delete(key);
        else next.set(key, value);
      }
      const qs = next.toString();
      router[mode](qs ? `${pathname}?${qs}` : pathname, { scroll: false });
    },
    [pathname, router, searchParams],
  );

  // 输入防抖：只 replace URL，避免每个字符都压入历史栈。
  function onSearchChange(value: string) {
    searchValueRef.current = value;
    if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
    searchTimerRef.current = setTimeout(() => {
      updateParams({ q: value || null, page: null }, "replace");
    }, 300);
  }

  useEffect(
    () => () => {
      if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
    },
    [],
  );

  const filtered = useMemo(() => {
    let rows = data;
    const needle = deferredQ.trim().toLowerCase();
    if (needle) {
      rows = rows.filter((row) =>
        searchKeys(row).some((s) => s.toLowerCase().includes(needle)),
      );
    }
    for (const f of filters) {
      const value = filterValues[f.key];
      if (value) rows = rows.filter((row) => f.predicate(row, value));
    }
    const col = columns.find((c) => c.key === sortKey && c.sortable && c.sortValue);
    if (col?.sortable && col.sortValue) {
      rows = [...rows].sort((a, b) => {
        const av = col.sortValue!(a);
        const bv = col.sortValue!(b);
        const cmp = av < bv ? -1 : av > bv ? 1 : 0;
        return sortDir === "asc" ? cmp : -cmp;
      });
    }
    return rows;
  }, [data, deferredQ, sortKey, sortDir, columns, searchKeys, filters, filterValues]);

  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const start = (currentPage - 1) * pageSize;
  const visible = filtered.slice(start, start + pageSize);

  function toggleSort(key: string) {
    if (key === sortKey) {
      updateParams(
        { dir: sortDir === "asc" ? "desc" : "asc", page: null },
        "push",
      );
    } else {
      updateParams({ sort: key, dir: "asc", page: null }, "push");
    }
  }

  return (
    <div
      className={cn(
        "overflow-hidden rounded-2xl border border-border bg-surface-1 shadow-[var(--shadow-premium)]",
        className,
      )}
    >
      <div className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-3">
        <div className="relative min-w-0 flex-1 sm:max-w-xs">
          <SearchIcon
            size={15}
            className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <input
            key={q}
            type="search"
            defaultValue={q}
            onChange={(e) => onSearchChange(e.target.value)}
            placeholder="搜索…"
            aria-label="搜索列表"
            className="h-9 w-full rounded-md border border-border bg-surface-1 pl-9 pr-8 text-sm text-foreground placeholder:text-muted-foreground/70 focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/30"
          />
          {q && (
            <button
              type="button"
              aria-label="清空搜索"
              onClick={() => {
                if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
                updateParams({ q: null, page: null }, "replace");
              }}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 rounded p-0.5 text-muted-foreground transition-colors hover:text-foreground"
            >
              <XIcon size={14} />
            </button>
          )}
        </div>
        <div className="ml-auto flex items-center gap-2 text-xs text-muted-foreground">
          {filters.map((f) => (
            <Select
              key={f.key}
              aria-label={f.label}
              value={filterValues[f.key]}
              onChange={(e) =>
                updateParams(
                  { [f.key]: e.target.value || null, page: null },
                  "push",
                )
              }
              className="h-8 w-auto text-xs"
            >
              <option value="">{f.label}：全部</option>
              {f.options.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </Select>
          ))}
          <span className="tabular-nums">共 {filtered.length} 条</span>
          <Select
            aria-label="每页条数"
            value={String(pageSize)}
            onChange={(e) =>
              updateParams(
                { pageSize: e.target.value, page: null },
                "push",
              )
            }
            className="h-8 w-20 text-xs"
          >
            {pageSizeOptions.map((n) => (
              <option key={n} value={n}>
                {n} / 页
              </option>
            ))}
          </Select>
        </div>
      </div>

      {visible.length === 0 ? (
        <div className="p-4">
          {empty ?? (
            <p className="py-10 text-center text-sm text-muted-foreground">
              {emptyLabel}
            </p>
          )}
        </div>
      ) : (
        <>
          <div className="max-h-[600px] overflow-auto">
            <table className="w-full text-sm">
              <thead className="sticky top-0 z-10 bg-surface-2/90 backdrop-blur-sm shadow-[inset_0_-1px_0_theme(colors.border)]">
                <tr className="text-left text-xs font-medium text-muted-foreground">
                  {columns.map((col) => (
                    <th key={col.key} className="px-4 py-3 font-medium">
                      {col.sortable ? (
                        <button
                          type="button"
                          onClick={() => toggleSort(col.key)}
                          className={cn(
                            "inline-flex items-center gap-1 transition-colors hover:text-foreground",
                            sortKey === col.key && "text-foreground",
                          )}
                        >
                          {col.header}
                          <ChevronDownIcon
                            size={13}
                            className={cn(
                              "transition-transform",
                              sortKey === col.key && sortDir === "asc" &&
                                "rotate-180",
                              sortKey !== col.key && "opacity-40",
                            )}
                            aria-hidden="true"
                          />
                        </button>
                      ) : (
                        col.header
                      )}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {visible.map((row) => (
                  <tr
                    key={rowKey(row)}
                    className="transition-colors hover:bg-surface-2/60"
                  >
                    {columns.map((col) => (
                      <td
                        key={col.key}
                        className={cn(
                          "px-4 py-3",
                          col.numeric &&
                            "font-mono tabular-nums text-right",
                          col.className,
                        )}
                      >
                        {col.cell(row)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border px-4 py-3">
            <p className="text-xs text-muted-foreground">
              第 {currentPage} / {totalPages} 页
            </p>
            <Pagination
              page={currentPage}
              totalPages={totalPages}
              onPageChange={(nextPage) =>
                updateParams({ page: String(nextPage) }, "push")
              }
            />
          </div>
        </>
      )}
    </div>
  );
}
