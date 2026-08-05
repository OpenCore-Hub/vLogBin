"use client";

import { useId, useState } from "react";
import type { TrendPoint } from "@/lib/api/types";
import { formatMoney } from "@/lib/format";
import {
  ChartFrame,
  ChartTooltip,
  CHART_PAD,
  chartY,
  shortDate,
  trendMax,
  TrendEmpty,
  useMeasure,
} from "./chart-base";

/**
 * 自绘 SVG 柱状图（§7.5）：纵向渐变（顶 brand-500 → 底 brand-500/soft），
 * hover 抬升透明度并显示 tooltip。纯 SVG 零依赖。
 */
export function BarChart({
  data,
  height = 220,
  format = "count",
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
  const max = trendMax(data);
  const valid = n >= 2 && max > 0;
  const plotW = Math.max(1, width - CHART_PAD.left - CHART_PAD.right);
  const slot = n > 0 ? plotW / n : plotW;
  const barW = Math.max(2, Math.min(16, slot * 0.6));
  const baseY = height - CHART_PAD.bottom;
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
              aria-label={`柱状图（${n} 天）`}
              className="block"
            >
              <defs>
                <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor="#14b8a6" />
                  <stop offset="100%" stopColor="#14b8a6" stopOpacity="0.18" />
                </linearGradient>
              </defs>

              {/* 首/中/尾日期标签 */}
              {[0, Math.floor((n - 1) / 2), n - 1].map((idx, k) => (
                <text
                  key={k}
                  x={CHART_PAD.left + (idx + 0.5) * slot}
                  y={height - 6}
                  textAnchor="middle"
                  className="fill-muted-foreground text-[10px]"
                >
                  {formatDate(data[idx].date)}
                </text>
              ))}

              {data.map((p, i) => {
                const cx = CHART_PAD.left + (i + 0.5) * slot;
                const y = chartY(p.value, max, height);
                const active = hover === i;
                return (
                  <rect
                    key={p.date}
                    x={cx - barW / 2}
                    y={y}
                    width={barW}
                    height={Math.max(1, baseY - y)}
                    rx={2}
                    fill={`url(#${gradId})`}
                    className="transition-opacity duration-100"
                    style={{ opacity: active ? 1 : 0.85 }}
                    onMouseEnter={() => setHover(i)}
                    onMouseLeave={() => setHover(null)}
                  />
                );
              })}

              {/* hover 高亮描边 */}
              {hover !== null && data[hover] && (
                <rect
                  x={CHART_PAD.left + (hover + 0.5) * slot - barW / 2}
                  y={chartY(data[hover].value, max, height)}
                  width={barW}
                  height={Math.max(1, baseY - chartY(data[hover].value, max, height))}
                  rx={2}
                  fill="none"
                  stroke="#14b8a6"
                  strokeWidth="1.5"
                  pointerEvents="none"
                />
              )}
            </svg>

            {hover !== null && data[hover] && (
              <ChartTooltip
                left={CHART_PAD.left + (hover + 0.5) * slot}
                top={chartY(data[hover].value, max, height) - 12}
              >
                <div className="text-muted-foreground">
                  {shortDate(data[hover].date)}
                </div>
                <div className="font-mono text-sm font-medium tabular-nums text-foreground">
                  {formatValue(data[hover].value)}
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
