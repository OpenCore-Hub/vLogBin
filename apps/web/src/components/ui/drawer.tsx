"use client";

import { useEffect, useRef, type ReactNode } from "react";
import { cn } from "@/lib/utils";
import { XIcon } from "./icons";

const DRAWER_WIDTH = {
  sm: "w-72",
  md: "w-80",
  lg: "w-96",
} as const;

export function Drawer({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  side = "left",
  size = "md",
  className,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title?: string;
  description?: string;
  children?: ReactNode;
  footer?: ReactNode;
  side?: "left" | "right";
  size?: keyof typeof DRAWER_WIDTH;
  className?: string;
}) {
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const prev = document.activeElement as HTMLElement | null;
    closeRef.current?.focus();
    return () => prev?.focus();
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onOpenChange(false);
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.body.style.overflow = prev;
      document.removeEventListener("keydown", onKey);
    };
  }, [open, onOpenChange]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50"
      role="dialog"
      aria-modal="true"
      aria-label={title ?? "抽屉"}
    >
      <div
        className="absolute inset-0 bg-neutral-950/45 animate-fade-in"
        onClick={() => onOpenChange(false)}
        aria-hidden="true"
      />
      <div
        className={cn(
          "absolute inset-y-0 flex flex-col border-r border-border bg-surface-1 shadow-[var(--shadow-premium)] animate-slide-in-left",
          side === "right" && "right-0 border-l border-r-0",
          DRAWER_WIDTH[size],
          className,
        )}
      >
        <div className="flex h-16 shrink-0 items-center justify-between gap-4 border-b border-border px-4">
          <div className="min-w-0">
            {title && <p className="text-sm font-semibold">{title}</p>}
            {description && (
              <p className="mt-0.5 truncate text-xs text-muted-foreground">
                {description}
              </p>
            )}
          </div>
          <button
            ref={closeRef}
            type="button"
            aria-label="关闭"
            onClick={() => onOpenChange(false)}
            className="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-2 hover:text-foreground"
          >
            <XIcon size={18} />
          </button>
        </div>
        <div className="flex-1 overflow-y-auto">{children}</div>
        {footer && (
          <div className="shrink-0 border-t border-border p-4">{footer}</div>
        )}
      </div>
    </div>
  );
}
