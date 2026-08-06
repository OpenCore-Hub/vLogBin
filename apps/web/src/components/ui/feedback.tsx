"use client";

import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { CopyButton } from "./code-block";
import { AlertIcon, CheckCircleIcon, InfoIcon, SpinnerIcon } from "./icons";

/* ---------------- Spinner ---------------- */
export function Spinner({
  size = 16,
  className,
  label = "加载中…",
}: {
  size?: number;
  className?: string;
  label?: string;
}) {
  return (
    <span
      role="status"
      aria-label={label}
      className={cn("inline-flex items-center gap-2", className)}
    >
      <SpinnerIcon size={size} className="animate-spin text-brand-600 dark:text-brand-500" />
      {label ? <span className="sr-only">{label}</span> : null}
    </span>
  );
}

/* ---------------- Skeleton ---------------- */
export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden="true" className={cn("animate-shimmer rounded-md", className)} />;
}

/* ---------------- Alert ---------------- */
export function Alert({
  variant = "danger",
  title,
  children,
  className,
}: {
  variant?: "danger" | "warning";
  title?: string;
  children?: ReactNode;
  className?: string;
}) {
  const styles = {
    danger: {
      wrap: "border-danger/25 bg-danger-soft text-danger",
      title: "text-danger",
      body: "text-danger/90",
    },
    warning: {
      wrap: "border-warning/25 bg-warning-soft text-warning",
      title: "text-warning",
      body: "text-warning/90",
    },
  }[variant];

  return (
    <div
      role="alert"
      className={cn(
        "flex items-start gap-2.5 rounded-2xl border px-4 py-3.5 text-sm animate-fade-in shadow-[var(--shadow-sm)]",
        styles.wrap,
        className,
      )}
    >
      <AlertIcon size={16} className="mt-0.5 shrink-0" aria-hidden="true" />
      <div className="min-w-0 space-y-1">
        {title && <p className={cn("font-medium leading-snug", styles.title)}>{title}</p>}
        {children && <div className={cn("leading-relaxed", styles.body)}>{children}</div>}
      </div>
    </div>
  );
}

/* ---------------- EmptyState ---------------- */
export function EmptyState({
  icon,
  title,
  description,
  action,
  className,
}: {
  icon?: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "surface-premium flex flex-col items-center justify-center rounded-2xl border border-dashed border-border-strong px-6 py-14 text-center",
        className,
      )}
    >
      {icon && (
        <div className="mb-3 flex h-11 w-11 items-center justify-center rounded-full bg-surface-2 text-muted-foreground">
          {icon}
        </div>
      )}
      <h3 className="text-sm font-semibold">{title}</h3>
      {description && (
        <p className="mt-1 max-w-sm text-sm text-muted-foreground">{description}</p>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}

/* ---------------- ErrorState ---------------- */
export function ErrorState({
  title = "加载失败",
  description,
  requestId,
  action,
  className,
}: {
  title?: string;
  description?: string;
  requestId?: string;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <Alert variant="danger" title={title} className={className}>
      <div className="space-y-3">
        {description && <p>{description}</p>}
        {requestId && (
          <div className="flex items-center gap-2">
            <code className="font-mono text-xs text-muted-foreground">
              {requestId}
            </code>
            <CopyButton text={requestId} label="复制 request_id" />
          </div>
        )}
        {action}
      </div>
    </Alert>
  );
}

/* ---------------- SuccessPanel ---------------- */
export function SuccessPanel({
  title,
  description,
  children,
  className,
}: {
  title?: string;
  description?: string;
  children?: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "rounded-2xl border border-success/25 bg-success-soft p-4 animate-fade-in shadow-[var(--shadow-inset-highlight)]",
        className,
      )}
    >
      <div className="flex items-start gap-2.5">
        <CheckCircleIcon size={18} className="mt-0.5 shrink-0 text-success" aria-hidden="true" />
        <div className="min-w-0 space-y-1">
          {title && <p className="font-medium text-success">{title}</p>}
          {description && <p className="text-sm leading-relaxed text-success/90">{description}</p>}
          {children}
        </div>
      </div>
    </div>
  );
}

/* ---------------- InfoNote（低调的提示条，用于说明性文案） ---------------- */
export function InfoNote({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "flex items-start gap-2.5 rounded-lg border border-border bg-surface-2 px-3.5 py-3 text-sm text-muted-foreground",
        className,
      )}
    >
      <InfoIcon size={16} className="mt-0.5 shrink-0 text-info" aria-hidden="true" />
      <div className="min-w-0 leading-relaxed">{children}</div>
    </div>
  );
}
