"use client";

import { useCallback } from "react";
import { useRouter } from "next/navigation";
import type { Workspace } from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { EnvSwitcher } from "./env-switcher";
import { WorkspaceSwitcher } from "./workspace-switcher";
import { useEnv } from "./env-provider";
import { ThemeToggle } from "./theme-toggle";
import { DropdownMenu } from "@/components/ui/overlay";
import { BoxIcon, GlobeIcon } from "@/components/ui/icons";
import { useMediaQuery } from "@/hooks/use-media-query";
import { cn } from "@/lib/utils";

export interface TopbarUser {
  name: string;
  email: string;
  isOperator: boolean;
}

const ENV_META: Record<Env, { label: string; dot: string }> = {
  test: { label: "测试环境", dot: "bg-env-test" },
  live: { label: "生产环境", dot: "bg-env-live" },
};

/** 顶栏：环境切换器 + 主题 + 用户菜单（零摩擦，无打断）。 */
export function Topbar({
  user,
  workspaces,
  activeWorkspaceId,
  onWorkspaceChange,
}: {
  user: TopbarUser;
  workspaces: Workspace[];
  activeWorkspaceId: string | null;
  onWorkspaceChange: (workspaceId: string) => Promise<void>;
}) {
  const { env, switchTo } = useEnv();
  const router = useRouter();
  const initial = user.name.trim().charAt(0).toUpperCase() || "U";
  // R21：窄屏（<sm）时环境切换器收进用户菜单，不溢出顶栏。
  const compact = useMediaQuery("(max-width: 639px)");

  const items = useCallback(
    (): Parameters<typeof DropdownMenu>[0]["items"] => [
      ...(compact
        ? [
            {
              type: "item" as const,
              label: (
                <span className="inline-flex items-center gap-2">
                  <GlobeIcon size={14} aria-hidden="true" />
                  {ENV_META[env].label}
                </span>
              ),
              onSelect: () => {
                const target: Env = env === "test" ? "live" : "test";
                void switchTo(target);
              },
            },
            { type: "separator" as const },
          ]
        : []),
      ...(user.isOperator
        ? [
            {
              type: "item" as const,
              label: "运营商台",
              onSelect: () => {
                window.location.href = "/ops";
              },
            },
            { type: "separator" as const },
          ]
        : []),
      {
        type: "item" as const,
        label: "退出登录",
        danger: true,
        onSelect: () => {
          window.location.href = "/auth/logout";
        },
      },
    ],
    [user.isOperator, compact, env, switchTo],
  );

  return (
    <header className="sticky top-0 z-30 flex h-14 items-center justify-between gap-3 border-b border-border bg-canvas/80 pl-12 pr-4 shadow-[var(--shadow-sm)] backdrop-blur-md sm:pr-6 lg:pl-6">
      <div className="flex items-center gap-2">
        {/* 窄屏常驻环境徽标（R21：live 徽标仍常驻可见） */}
        <span
          className={cn(
            "inline-flex items-center gap-1.5 rounded-md border border-border bg-surface-2 px-2 py-1 text-xs text-muted-foreground sm:hidden",
          )}
          aria-label={`当前环境：${ENV_META[env].label}`}
        >
          <span
            className={cn("h-1.5 w-1.5 rounded-full", ENV_META[env].dot)}
            aria-hidden="true"
          />
          {ENV_META[env].label}
        </span>
        {user.isOperator && (
          <span className="inline-flex items-center gap-1 rounded-md border border-border bg-surface-2 px-2 py-1 text-xs text-muted-foreground">
            <BoxIcon size={12} />
            Operator
          </span>
        )}
      </div>
      <div className="flex items-center gap-1.5">
        <WorkspaceSwitcher
          workspaces={workspaces}
          activeWorkspaceId={activeWorkspaceId}
          onSwitch={(workspaceId) => {
            void onWorkspaceChange(workspaceId).then(() => router.refresh());
          }}
        />
        {/* 桌面（≥sm）直接展示环境切换器；窄屏收进用户菜单 */}
        <div className={cn(compact && "hidden")}>
          <EnvSwitcher />
        </div>
        <ThemeToggle />
        <DropdownMenu
          triggerLabel="用户菜单"
          trigger={
            <span className="ml-1 flex size-7 items-center justify-center rounded-full bg-brand-600 text-xs font-semibold text-white">
              {initial}
            </span>
          }
          items={items()}
        />
      </div>
    </header>
  );
}
