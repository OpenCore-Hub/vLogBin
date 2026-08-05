"use client";

import { useId, useMemo, useState } from "react";
import type { TrendPoint } from "@/lib/api/types";
import { formatMoney } from "@/lib/format";
import {
  ChartFrame,
  ChartTooltip,
  CHART_PAD,
  chartX,
  chartY,
  shortDate,
  trendMax,
  TrendEmpty,
  useMeasure,
} from "./chart-base";

type Point = { x: number; y: number };

/** Catmull-Rom → 三次贝塞尔，生成平滑曲线路径。 */
function smoothPath(pts: Point[]): string {
  if (pts.length === 0) return "";
  if (pts.length === 1) return `M ${pts[0].x} ${pts[0].y}`;
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

/**
 * 自绘 SVG 面积图（§7.5）：墨青渐变（brand-500 → transparent，0.25 → 0）
 * 面积 + 主色折线 + 端点圆点，hover 显示十字线 + tooltip。纯 SVG 零依赖。
 */
export function AreaChart({
  data,
  height = 220,
  format = "money",
  formatDate = shortDate,
  emptyLabel = "暂无趋势数据",
}: {
  data: TrendPoint[];
  height?: number;
  /** 数值格式（可序列化；图表内部完成格式化）。 */
  format?: "money" | "count";
  formatDate?: (date: string) => string;
  emptyLabel?: string;
}) {
  const { ref, width } = useMeasure<HTMLDivElement>();
  const gradId = useId();
  const [hover, setHover] = useState<number | null>(null);

  const n = data.length;
  const valid = n >= 2 && trendMax(data) > 0;

  const { line, area } = useMemo(() => {
    if (!valid || width === 0) return { line: "", area: "" };
    const max = trendMax(data);
    const pts: Point[] = data.map((p, i) => ({
      x: chartX(n, i, width),
      y: chartY(p.value, max, height),
    }));
    const baseY = height - CHART_PAD.bottom;
    const smooth = smoothPath(pts);
    return {
      line: smooth,
      area: `${smooth} L ${pts[pts.length - 1].x.toFixed(2)} ${baseY} L ${pts[0].x.toFixed(2)} ${baseY} Z`,
    };
  }, [data, n, width, height, valid]);

  const plotH = height - CHART_PAD.top - CHART_PAD.bottom;
  const baseY = height - CHART_PAD.bottom;

  const handleMove = (e: React.MouseEvent<SVGRectElement>) => {
    if (!valid || width === 0) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const x = e.clientX - rect.left;
    const plotW = Math.max(1, width - CHART_PAD.left - CHART_PAD.right);
    const ratio = (x - CHART_PAD.left) / plotW;
    const idx = Math.round(ratio * (n - 1));
    setHover(Math.max(0, Math.min(n - 1, idx)));
  };

  const hoverP = valid && hover !== null ? data[hover] : null;
  const gridYs = [0.25, 0.5, 0.75];
  const formatValue = (v: number) =>
    format === "money" ? formatMoney(v) : `${v.toLocaleString("zh-CN")} 次`;

  return (
    <ChartFrame measureRef={ref} width={width} height={height}>
      {width > 0 &&
        (valid ? (
          <>
            <svg
              width={width}
              height={height}
              role="img"
              aria-label={`趋势图（${n} 天）`}
              className="block"
            >
              <defs>
                <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#14b8a6" stopOpacity="0.25" />
                  <stop offset="100%" stopColor="#14b8a6" stopOpacity="0" />
                </linearGradient>
              </defs>

              {gridYs.map((f) => {
                const y = CHART_PAD.top + (1 - f) * plotH;
                return (
                  <line
                    key={f}
                    x1={CHART_PAD.left}
                    x2={width - CHART_PAD.right}
                    y1={y}
                    y2={y}
                    className="stroke-border"
                    strokeWidth="1"
                  />
                );
              })}

              {/* 首/中/尾日期标签 */}
              {[0, Math.floor((n - 1) / 2), n - 1].map((idx, k) => (
                <text
                  key={k}
                  x={chartX(n, idx, width)}
                  y={height - 6}
                  textAnchor={k === 0 ? "start" : k === 2 ? "end" : "middle"}
                  className="fill-muted-foreground text-[10px]"
                >
                  {formatDate(data[idx].date)}
                </text>
              ))}

              <path d={area} fill={`url(#${gradId})`} />
              <path
                d={line}
                fill="none"
                stroke="#14b8a6"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              />

              {/* hover 高亮 */}
              {hoverP && hover !== null && (
                <g>
                  <line
                    x1={chartX(n, hover, width)}
                    x2={chartX(n, hover, width)}
                    y1={CHART_PAD.top}
                    y2={baseY}
                    className="stroke-border-strong"
                    strokeWidth="1"
                    strokeDasharray="3 3"
                  />
                  <circle
                    cx={chartX(n, hover, width)}
                    cy={chartY(hoverP.value, trendMax(data), height)}
                    r="3.5"
                    fill="#14b8a6"
                    className="stroke-surface-1"
                    strokeWidth="2"
                  />
                </g>
              )}

              {/* 端点圆点 */}
              <circle
                cx={chartX(n, 0, width)}
                cy={chartY(data[0].value, trendMax(data), height)}
                r="3"
                fill="#14b8a6"
                className="stroke-surface-1"
                strokeWidth="2"
              />
              <circle
                cx={chartX(n, n - 1, width)}
                cy={chartY(data[n - 1].value, trendMax(data), height)}
                r="3"
                fill="#14b8a6"
                className="stroke-surface-1"
                strokeWidth="2"
              />

              <rect
                x={CHART_PAD.left}
                y={CHART_PAD.top}
                width={Math.max(1, width - CHART_PAD.left - CHART_PAD.right)}
                height={plotH}
                fill="transparent"
                className="cursor-crosshair"
                onMouseMove={handleMove}
                onMouseLeave={() => setHover(null)}
              />
            </svg>

            {hoverP && hover !== null && (
              <ChartTooltip
                left={chartX(n, hover, width)}
                top={chartY(hoverP.value, trendMax(data), height) - 12}
              >
                <div className="text-muted-foreground">
                  {shortDate(hoverP.date)}
                </div>
                <div className="font-mono text-sm font-medium tabular-nums text-foreground">
                  {formatValue(hoverP.value)}
                </div>
              </ChartTooltip>
            )}
          </>
        ) : (
          <TrendEmpty height={height} label={emptyLabel} />
        ))}
    </ChartFrame>
  );
}
