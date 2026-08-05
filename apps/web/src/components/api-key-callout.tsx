"use client";

import { cn } from "@/lib/utils";
import { CopyButton } from "@/components/ui/code-block";
import { KeyIcon } from "@/components/ui/icons";

/**
 * 一次性明文密钥展示：新 UI token 色 + 品牌语音 + 复制反馈。
 * 密钥只会在 API 返回一次，关闭前请先复制保存。
 */
export function ApiKeyCallout({
  apiKey,
  title = "API Key",
  description = "密钥仅展示一次，请立即复制并妥善保存。",
  onCopied,
  className,
}: {
  apiKey: string;
  title?: string;
  description?: string;
  onCopied?: () => void;
  className?: string;
}) {
  return (
    <div
      role="alert"
      className={cn(
        "rounded-2xl border border-brand-500/20 bg-surface-1 p-4 shadow-[var(--shadow-inset-highlight)]",
        className,
      )}
    >
      <div className="flex items-center gap-2.5">
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-brand-600/10 text-brand-600 dark:bg-brand-500/15 dark:text-brand-400">
          <KeyIcon size={15} aria-hidden="true" />
        </span>
        <div className="min-w-0">
          <p className="text-sm font-semibold text-foreground">{title}</p>
          <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">
            {description}
          </p>
        </div>
      </div>
      <div className="mt-3 flex items-center gap-2 rounded-xl border border-border bg-surface-2 p-3">
        <code className="min-w-0 flex-1 truncate font-mono text-xs text-foreground select-all">
          {apiKey}
        </code>
        <CopyButton text={apiKey} label="复制密钥" onCopied={onCopied} />
      </div>
    </div>
  );
}
