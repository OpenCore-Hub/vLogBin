"use client";

export type DonutDatum = {
  label: string;
  value: number;
  color?: string;
};

const DEFAULT_COLORS = [
  "#14b8a6",
  "#0d9488",
  "#f59e0b",
  "#ef4444",
  "#6366f1",
  "#64748b",
  "#84cc16",
];

/**
 * 自绘 SVG 环形图（§7.5）：stroke-dasharray 分段圆环，默认品牌墨青 + 语义色
 * 调色板，下方配图例（标签 / 数值 / 占比）。零第三方依赖。
 */
export function DonutChart({
  data,
  size = 176,
  thickness = 22,
  centerLabel,
  centerValue,
  emptyLabel = "暂无数据",
}: {
  data: DonutDatum[];
  size?: number;
  thickness?: number;
  centerLabel?: string;
  centerValue?: string;
  emptyLabel?: string;
}) {
  const total = data.reduce((sum, d) => sum + Math.max(0, d.value), 0);
  const radius = Math.max(8, (size - thickness) / 2);
  const circumference = 2 * Math.PI * radius;
  const center = size / 2;

  const segments = data.filter((d) => d.value > 0).reduce<
    Array<{
      key: string;
      color: string;
      len: number;
      offset: number;
      datum: DonutDatum;
    }>
  >((acc, d, i) => {
    const len = total > 0 ? (d.value / total) * circumference : 0;
    const prev = acc[acc.length - 1];
    const offset = prev ? prev.offset + prev.len : 0;
    acc.push({
      key: `${d.label}-${i}`,
      color: d.color ?? DEFAULT_COLORS[i % DEFAULT_COLORS.length],
      len,
      offset,
      datum: d,
    });
    return acc;
  }, []);

  return (
    <div
      className="flex flex-col items-center gap-4 sm:flex-row sm:items-center sm:justify-center"
      role="img"
      aria-label={`环形图（${segments.length} 项）`}
    >
      <div className="relative shrink-0" style={{ width: size, height: size }}>
        <svg
          width={size}
          height={size}
          viewBox={`0 0 ${size} ${size}`}
          className="block -rotate-90"
        >
          <circle
            cx={center}
            cy={center}
            r={radius}
            fill="none"
            strokeWidth={thickness}
            className="stroke-surface-2"
          />
          {segments.map((s) => (
            <circle
              key={s.key}
              cx={center}
              cy={center}
              r={radius}
              fill="none"
              stroke={s.color}
              strokeWidth={thickness}
              strokeDasharray={`${s.len} ${circumference - s.len}`}
              strokeDashoffset={-s.offset}
            />
          ))}
        </svg>
        <div className="absolute inset-0 flex flex-col items-center justify-center text-center">
          {total > 0 ? (
            <>
              {centerValue && (
                <span className="font-mono text-xl font-semibold tabular-nums">
                  {centerValue}
                </span>
              )}
              {centerLabel && (
                <span className="mt-0.5 text-xs text-muted-foreground">
                  {centerLabel}
                </span>
              )}
            </>
          ) : (
            <span className="text-xs text-muted-foreground">{emptyLabel}</span>
          )}
        </div>
      </div>

      {segments.length > 0 && (
        <ul className="w-full min-w-0 max-w-56 space-y-1.5 sm:w-auto">
          {segments.map((s) => (
            <li key={s.key} className="flex items-center gap-2 text-sm">
              <span
                className="size-2.5 shrink-0 rounded-full"
                style={{ backgroundColor: s.color }}
                aria-hidden="true"
              />
              <span className="min-w-0 flex-1 truncate text-muted-foreground">
                {s.datum.label}
              </span>
              <span className="font-mono text-xs tabular-nums">
                {s.datum.value}
              </span>
              <span className="w-11 text-right font-mono text-xs text-muted-foreground tabular-nums">
                {total > 0 ? Math.round((s.datum.value / total) * 100) : 0}%
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
