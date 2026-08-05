"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { cn } from "@/lib/utils";
import type { TrendPoint } from "@/lib/api/types";

/** 测量容器实际宽度（ResizeObserver），SSR 首帧为 0，挂载后获得真实宽度。 */
export function useMeasure<T extends HTMLElement>() {
  const ref = useRef<T | null>(null);
  const [width, setWidth] = useState(0);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const update = () => setWidth(el.getBoundingClientRect().width);
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  return { ref, width };
}

/** 图表内部几何常量：为底部日期标签与顶部端点预留留白。 */
export const CHART_PAD = { top: 8, right: 8, bottom: 22, left: 8 };

export function chartX(n: number, index: number, width: number): number {
  if (n <= 1) return CHART_PAD.left;
  const plotW = Math.max(1, width - CHART_PAD.left - CHART_PAD.right);
  return CHART_PAD.left + (index / (n - 1)) * plotW;
}

/** Y 值 → 像素（0 映射到底部，max 映射到顶部）。 */
export function chartY(value: number, max: number, height: number): number {
  const plotH = Math.max(1, height - CHART_PAD.top - CHART_PAD.bottom);
  const ratio = max > 0 ? Math.max(0, Math.min(1, value / max)) : 0;
  return CHART_PAD.top + (1 - ratio) * plotH;
}

export function trendMax(data: TrendPoint[]): number {
  let max = 0;
  for (const p of data) {
    if (p.value > max) max = p.value;
  }
  return max;
}

type ChartPoint = { x: number; y: number };

/** Catmull-Rom → 三次贝塞尔，生成平滑曲线路径（AreaChart / LineChart 共用）。 */
export function smoothPath(pts: ChartPoint[]): string {
  if (pts.length === 0) return "";
  if (pts.length === 1) return `M ${pts[0].x.toFixed(2)} ${pts[0].y.toFixed(2)}`;
  let d = `M ${pts[0].x.toFixed(2)} ${pts[0].y.toFixed(2)}`;
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[Math.max(0, i - 1)];
    const p1 = pts[i];
    const p2 = pts[i + 1];
    const p3 = pts[Math.min(pts.length - 1, i + 2)];
    const c1x = p1.x + (p2.x - p0.x) / 6;
    const c1y = p1.y + (p2.y - p0.y) / 6;
    const c2x = p2.x - (p3.x - p1.x) / 6;
    const c2y = p2.y - (p3.y - p1.y) / 6;
    d += ` C ${c1x.toFixed(2)} ${c1y.toFixed(2)}, ${c2x.toFixed(2)} ${c2y.toFixed(2)}, ${p2.x.toFixed(2)} ${p2.y.toFixed(2)}`;
  }
  return d;
}

/** 短日期标签：2026-08-04 → "8/4"。 */
export function shortDate(date: string): string {
  const m = date.match(/^\d{4}-(\d{2})-(\d{2})$/);
  return m ? `${Number(m[1])}/${Number(m[2])}` : date;
}

/** tooltip 浮层：surface-3 + 2px 主色左描边 + mono 数值（§7.5）。 */
export function ChartTooltip({
  left,
  top,
  children,
}: {
  left: number;
  top: number;
  children: ReactNode;
}) {
  return (
    <div
      className="pointer-events-none absolute z-10 -translate-x-1/2 -translate-y-full whitespace-nowrap rounded-md border border-border-strong bg-surface-3 py-1.5 pl-3 pr-3 text-xs shadow-md"
      style={{ left, top }}
    >
      <div className="absolute bottom-[-4px] left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 border-b border-r border-border-strong bg-surface-3" />
      {children}
    </div>
  );
}

/** 数据不足时的空态（长度 < 2 或全零视为无有效趋势）。 */
export function TrendEmpty({
  height,
  label,
}: {
  height: number;
  label: string;
}) {
  return (
    <div
      className="flex flex-col items-center justify-center gap-1 text-muted-foreground"
      style={{ height }}
    >
      <svg
        width="28"
        height="20"
        viewBox="0 0 28 20"
        fill="none"
        aria-hidden="true"
        className="opacity-60"
      >
        <path
          d="M2 16 L9 11 L15 13 L26 5"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <circle cx="26" cy="5" r="2" fill="currentColor" />
      </svg>
      <span className="text-xs">{label}</span>
    </div>
  );
}

/** 测量外壳：相对定位容器 + 宽度测量（SSR 首帧 0 宽不渲染子节点）。 */
export function ChartFrame({
  measureRef,
  width,
  height,
  className,
  children,
}: {
  measureRef: React.RefObject<HTMLDivElement | null>;
  width: number;
  height: number;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div
      ref={measureRef}
      className={cn("relative", className)}
      style={{ height }}
    >
      {width > 0 ? children : null}
    </div>
  );
}
