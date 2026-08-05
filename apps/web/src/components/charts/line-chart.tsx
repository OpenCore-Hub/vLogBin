"use client";

import { useState } from "react";
import type { TrendPoint } from "@/lib/api/types";
import { formatMoney } from "@/lib/format";
import {
  ChartFrame,
  ChartTooltip,
  CHART_PAD,
  chartX,
  chartY,
  shortDate,
  smoothPath,
  trendMax,
  TrendEmpty,
  useMeasure,
} from "./chart-base";

/**
 * 自绘 SVG 折线图（§7.5）：主色折线 + 网格线 + 首/中/尾日期标签，hover 显示
 * 十字线 + tooltip。单点数据也渲染端点圆点（客户用量等稀疏数据可见）。纯 SVG 零依赖。
 */
export function LineChart({
  data,
  height = 220,
  format = "count",
  formatDate = shortDate,
  emptyLabel = "暂无趋势数据",
}: {
  data: TrendPoint[];
  height?: number;
  format?: "money" | "count";
  formatDate?: (date: string) => string;
  emptyLabel?: string;
}) {
  const { ref, width } = useMeasure<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);

  const n = data.length;
  const max = trendMax(data);
  const valid = n > 0;
  const plotH = height - CHART_PAD.top - CHART_PAD.bottom;
  const baseY = height - CHART_PAD.bottom;
  const formatValue = (v: number) =>
    format === "money" ? formatMoney(v) : `${v.toLocaleString("zh-CN")} 次`;

  const line =
    valid && width > 0
      ? smoothPath(
          data.map((p, i) => ({
            x: chartX(n, i, width),
            y: chartY(p.value, max, height),
          })),
        )
      : "";

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

  return (
    <ChartFrame measureRef={ref} width={width} height={height}>
      {width > 0 &&
        (valid ? (
          <>
            <svg
              width={width}
              height={height}
              role="img"
              aria-label={`折线图（${n} 天）`}
              className="block"
            >
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

              <path
                d={line}
                fill="none"
                stroke="#14b8a6"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              />

              {data.map((p, i) => (
                <circle
                  key={`${p.date}-${i}`}
                  cx={chartX(n, i, width)}
                  cy={chartY(p.value, max, height)}
                  r={n === 1 ? 3.5 : 2}
                  fill="#14b8a6"
                  className="stroke-surface-1"
                  strokeWidth="1.5"
                />
              ))}

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
                    cy={chartY(hoverP.value, max, height)}
                    r="4"
                    fill="#14b8a6"
                    className="stroke-surface-1"
                    strokeWidth="2"
                  />
                </g>
              )}

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
                top={chartY(hoverP.value, max, height) - 12}
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
