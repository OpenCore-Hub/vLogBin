"use client";

import { useCallback, useState } from "react";
import { cn } from "@/lib/utils";
import { CopyIcon, CheckIcon } from "./icons";
import { Tooltip } from "./overlay";

async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // fall through to legacy path
  }
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

export function CopyButton({
  text,
  label = "复制",
  copiedLabel = "已复制",
  className,
  onCopied,
}: {
  text: string;
  label?: string;
  copiedLabel?: string;
  className?: string;
  onCopied?: () => void;
}) {
  const [copied, setCopied] = useState(false);

  const onCopy = useCallback(async () => {
    const ok = await copyText(text);
    if (ok) {
      setCopied(true);
      onCopied?.();
      window.setTimeout(() => setCopied(false), 1600);
    }
  }, [text, onCopied]);

  return (
    <Tooltip label={copied ? copiedLabel : label}>
      <button
        type="button"
        aria-label={label}
        onClick={onCopy}
        className={cn(
          "rounded-md p-1.5 text-terminal-muted transition-colors hover:bg-white/10 hover:text-terminal-fg",
          copied && "text-success",
          className,
        )}
      >
        {copied ? <CheckIcon size={14} /> : <CopyIcon size={14} />}
      </button>
    </Tooltip>
  );
}

/** 终端风格代码块：墨夜底、等宽字体、可选标题与复制。 */
export function CodeBlock({
  code,
  title,
  language,
  className,
  dense = false,
  onCopied,
}: {
  code: string;
  title?: string;
  language?: string;
  className?: string;
  dense?: boolean;
  onCopied?: () => void;
}) {
  return (
    <div
      className={cn(
        "overflow-hidden rounded-lg border border-white/10 bg-terminal-bg text-terminal-fg",
        className,
      )}
    >
      {(title || language) && (
        <div className="flex items-center justify-between border-b border-white/10 px-3 py-1.5">
          <div className="flex items-center gap-2">
            <span className="size-2 rounded-full bg-white/20" />
            <span className="text-xs text-terminal-muted">{title}</span>
          </div>
          <div className="flex items-center gap-2">
            {language && (
              <span className="text-[10px] uppercase tracking-wider text-terminal-dim">
                {language}
              </span>
            )}
            <CopyButton text={code} onCopied={onCopied} />
          </div>
        </div>
      )}
      <pre
        className={cn(
          "overflow-x-auto font-mono text-[13px] leading-relaxed",
          dense ? "p-2.5" : "p-4",
        )}
      >
        <code>{code}</code>
      </pre>
    </div>
  );
}
