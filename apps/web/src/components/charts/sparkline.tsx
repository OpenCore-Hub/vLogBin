"use client";

import { useId } from "react";
import type { TrendPoint } from "@/lib/api/types";
import { formatMoney } from "@/lib/format";
import { useMeasure } from "./chart-base";

const PAD = { top: 5, right: 4, bottom: 5, left: 4 };

function xAt(n: number, index: number, width: number): number {
  if (n <= 1) return PAD.left;
  const plotW = Math.max(1, width - PAD.left - PAD.right);
  return PAD.left + (index / (n - 1)) * plotW;
}

function yAt(value: number, max: number, height: number): number {
  const plotH = Math.max(1, height - PAD.top - PAD.bottom);
  const ratio = max > 0 ? Math.max(0, Math.min(1, value / max)) : 0;
  return PAD.top + (1 - ratio) * plotH;
}

/**
 * 自绘 SVG 迷你趋势线（§7.5）：紧凑卡片内使用，墨青折线 + 极浅面积 + 端点圆点，
 * 无坐标轴，hover 不需要 tooltip（卡片正文已经给出当前值）。纯 SVG 零依赖。
 */
export function Sparkline({
  data,
  height = 36,
  format = "count",
  ariaLabel,
}: {
  data: TrendPoint[];
  height?: number;
  format?: "money" | "count";
  ariaLabel?: string;
}) {
  const { ref, width } = useMeasure<HTMLDivElement>();
  const gradId = useId();
  const n = data.length;
  const max = data.reduce((m, p) => Math.max(m, p.value), 0);
  const valid = n > 0;
  const label =
    ariaLabel ??
    (valid ? `迷你趋势图（${n} 天）` : "迷你趋势图（暂无数据）");

  if (width === 0) {
    return <div ref={ref} style={{ height }} />;
  }

  if (!valid) {
    return (
      <div ref={ref} style={{ height }}>
        <svg
          width={width}
          height={height}
          role="img"
          aria-label={label}
          className="block"
        >
          <line
            x1={PAD.left}
            x2={width - PAD.right}
            y1={height / 2}
            y2={height / 2}
            stroke="currentColor"
            strokeWidth="1"
            strokeDasharray="3 3"
            className="text-border-strong"
          />
        </svg>
      </div>
    );
  }

  const pts = data.map((p, i) => ({
    x: xAt(n, i, width),
    y: yAt(p.value, max, height),
  }));
  const line = pts
    .map((p, i) => `${i === 0 ? "M" : "L"} ${p.x.toFixed(1)} ${p.y.toFixed(1)}`)
    .join(" ");
  const baseY = height - PAD.bottom;
  const area = `${line} L ${pts[pts.length - 1].x.toFixed(1)} ${baseY} L ${pts[0].x.toFixed(1)} ${baseY} Z`;
  const last = data[data.length - 1];

  return (
    <div ref={ref} style={{ height }}>
      <svg
        width={width}
        height={height}
        role="img"
        aria-label={label}
        className="block"
      >
        <defs>
          <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#14b8a6" stopOpacity="0.18" />
            <stop offset="100%" stopColor="#14b8a6" stopOpacity="0" />
          </linearGradient>
        </defs>
        <path d={area} fill={`url(#${gradId})`} />
        <path
          d={line}
          fill="none"
          stroke="#14b8a6"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <circle
          cx={pts[pts.length - 1].x}
          cy={pts[pts.length - 1].y}
          r="2.5"
          fill="#14b8a6"
          className="stroke-surface-1"
          strokeWidth="1.5"
        />
        <title>{`${last.date} · ${
          format === "money" ? formatMoney(last.value) : `${last.value} 次`
        }`}</title>
      </svg>
    </div>
  );
}
