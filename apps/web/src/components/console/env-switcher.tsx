"use client";

import { cn } from "@/lib/utils";
import type { Env } from "@/lib/env-shared";
import { CheckIcon, GlobeIcon } from "@/components/ui/icons";
import { DropdownMenu, DropdownChevron } from "@/components/ui/overlay";
import { useEnv } from "./env-provider";

const ENV_META: Record<Env, { label: string; hint: string; dot: string }> = {
  test: {
    label: "测试环境",
    hint: "沙箱 · 不影响生产数据",
    dot: "bg-env-test",
  },
  live: {
    label: "生产环境",
    hint: "真实数据 · 谨慎操作",
    dot: "bg-env-live",
  },
};

function EnvItem({ value, current }: { value: Env; current: Env }) {
  const meta = ENV_META[value];
  return (
    <span className="flex w-full items-center justify-between gap-8">
      <span className="flex flex-col items-start">
        <span className="inline-flex items-center gap-2">
          <span className={cn("h-1.5 w-1.5 rounded-full", meta.dot)} aria-hidden="true" />
          {meta.label}
        </span>
        <span className="mt-0.5 text-xs text-muted-foreground">{meta.hint}</span>
      </span>
      {value === current && <CheckIcon size={14} className="text-brand-600" />}
    </span>
  );
}

/** 环境切换器：URL ?env= 优先，其次持久化 cookie，默认 test。 */
export function EnvSwitcher({ className }: { className?: string }) {
  const { env, switchTo, pending } = useEnv();
  const meta = ENV_META[env];

  return (
    <DropdownMenu
      align="end"
      trigger={
        <span className={cn("inline-flex items-center gap-2", className)}>
          <GlobeIcon size={14} aria-hidden="true" />
          <span>{meta.label}</span>
          {pending ? (
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-brand-500" />
          ) : (
            <span className={cn("h-1.5 w-1.5 rounded-full", meta.dot)} aria-hidden="true" />
          )}
          <DropdownChevron />
        </span>
      }
      items={[
        {
          type: "item",
          label: <EnvItem value="test" current={env} />,
          onSelect: () => void switchTo("test"),
        },
        { type: "separator" },
        {
          type: "item",
          label: <EnvItem value="live" current={env} />,
          onSelect: () => void switchTo("live"),
        },
      ]}
    />
  );
}
