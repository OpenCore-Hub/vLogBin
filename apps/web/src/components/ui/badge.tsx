import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export type BadgeVariant =
  | "neutral"
  | "brand"
  | "success"
  | "warning"
  | "danger"
  | "info"
  | "env-test"
  | "env-live"
  | "draft"
  | "active"
  | "review"
  | "suspended"
  | "test";

const VARIANT_CLASSES: Record<BadgeVariant, string> = {
  neutral: "bg-surface-2 text-muted-foreground border-border",
  brand: "bg-brand-50 text-brand-700 border-brand-200",
  success: "bg-success-soft text-success border-success/30",
  warning: "bg-warning-soft text-warning border-warning/30",
  danger: "bg-danger-soft text-danger border-danger/30",
  info: "bg-info-soft text-info border-info/30",
  "env-test": "bg-env-test-soft text-env-test border-env-test/30",
  "env-live": "bg-env-live-soft text-env-live border-env-live/30",
  draft: "bg-lifecycle-draft-soft text-lifecycle-draft border-border",
  active: "bg-lifecycle-active-soft text-lifecycle-active border-success/30",
  review: "bg-lifecycle-review-soft text-lifecycle-review border-warning/30",
  suspended:
    "bg-lifecycle-suspended-soft text-lifecycle-suspended border-danger/30",
  test: "bg-lifecycle-test-soft text-lifecycle-test border-info/30",
};

export function Badge({
  variant = "neutral",
  className,
  children,
  title,
}: {
  variant?: BadgeVariant;
  className?: string;
  children: ReactNode;
  title?: string;
}) {
  return (
    <span
      title={title}
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium whitespace-nowrap",
        VARIANT_CLASSES[variant],
        className,
      )}
    >
      {children}
    </span>
  );
}

// 与服务端 internal/domain/lifecycle.go 状态机保持同步：
// REGISTERED → TEST_ACTIVE → LIVE_REVIEW → LIVE_ACTIVE / RESTRICTED / SUSPENDED → OFFBOARDING
const LIFECYCLE_MAP: Record<string, BadgeVariant> = {
  REGISTERED: "draft",
  TEST_ACTIVE: "test",
  TEST: "test",
  ONBOARDING: "test",
  LIVE_REVIEW: "review",
  LIVE_ACTIVE: "active",
  RESTRICTED: "warning",
  SUSPENDED: "suspended",
  OFFBOARDING: "draft",
  DRAFT: "draft",
};

/** Provider 生命周期状态徽章（语义色映射）。 */
export function LifecycleBadge({ state }: { state: string }) {
  return (
    <Badge variant={LIFECYCLE_MAP[state] ?? "neutral"} title={`Lifecycle: ${state}`}>
      {state}
    </Badge>
  );
}

/** 环境徽章：test / live。 */
export function EnvBadge({ env }: { env: string }) {
  const isLive = env === "live";
  return (
    <Badge
      variant={isLive ? "env-live" : "env-test"}
      title={`Environment: ${env}`}
    >
      <span
        className={cn(
          "inline-block size-1.5 rounded-full",
          isLive ? "bg-env-live" : "bg-env-test",
        )}
      />
      {env}
    </Badge>
  );
}
