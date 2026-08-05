import { cn } from "@/lib/utils";
import { CheckIcon } from "@/components/ui/icons";

const LINES: Array<{ text: string; done?: boolean }> = [
  { text: "$ vlb init --workspace acme" },
  { text: "✓ 创建工作空间", done: true },
  { text: "✓ 发布首个套餐", done: true },
  { text: "✓ 接入第一位客户", done: true },
  { text: "→ 等待用量事件上报…" },
];

/** 官网 Hero：终端 / 日志隐喻，凸显 vLogBin 品牌。 */
export function TerminalHero({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        "relative rounded-2xl bg-white/5 p-1.5 shadow-[0_24px_80px_-32px_rgba(0,0,0,0.55),0_8px_30px_-12px_rgba(45,212,191,0.18)] ring-1 ring-white/10",
        className,
      )}
      role="img"
      aria-label="终端示例：初始化 vLogBin 工作空间并发布套餐"
    >
      <div className="overflow-hidden rounded-2xl border border-white/10 bg-terminal-bg shadow-[inset_0_1px_0_rgba(255,255,255,0.08)]">
        {/* 标题栏 */}
        <div className="flex items-center justify-between border-b border-white/10 bg-white/[0.03] px-4 py-2.5">
          <div className="flex items-center gap-1.5">
            <span className="size-2.5 rounded-full bg-danger/80" />
            <span className="size-2.5 rounded-full bg-warning/80" />
            <span className="size-2.5 rounded-full bg-success/80" />
          </div>
          <span className="font-mono text-xs text-terminal-muted">
            vlogbin — zsh
          </span>
          <span className="font-mono text-xs text-terminal-dim">v0.1.0</span>
        </div>

        {/* 终端正文 */}
        <div className="space-y-1.5 px-5 py-6 font-mono text-[13px] leading-relaxed sm:text-sm">
          {LINES.map((line, i) => {
            const isLast = i === LINES.length - 1;
            return (
              <div key={i} className="flex items-baseline gap-2">
                {line.done ? (
                  <CheckIcon size={14} className="shrink-0 translate-y-0.5 text-terminal-fg" />
                ) : (
                  <span className="shrink-0 w-3.5 text-terminal-dim">❯</span>
                )}
                <span
                  className={cn(
                    isLast ? "text-terminal-muted" : "text-terminal-fg",
                  )}
                >
                  {line.text}
                  {isLast && (
                    <span className="ml-0.5 inline-block h-4 w-2 translate-y-0.5 bg-terminal-fg animate-caret" />
                  )}
                </span>
              </div>
            );
          })}
        </div>

        {/* 底部状态条 */}
        <div className="flex items-center gap-2 border-t border-white/10 bg-white/[0.03] px-4 py-2 font-mono text-[11px] text-terminal-muted">
          <span className="size-1.5 rounded-full bg-success animate-pulse" />
          All systems operational
        </div>
      </div>
    </div>
  );
}
