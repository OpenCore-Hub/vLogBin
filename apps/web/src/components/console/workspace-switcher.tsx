"use client";

import type { Workspace } from "@/lib/api/operator";
import { CheckIcon, LayersIcon } from "@/components/ui/icons";
import { DropdownMenu, DropdownChevron } from "@/components/ui/overlay";

export function WorkspaceSwitcher({
  workspaces,
  activeWorkspaceId,
  onSwitch,
}: {
  workspaces: Workspace[];
  activeWorkspaceId: string | null;
  onSwitch: (workspaceId: string) => void;
}) {
  const active =
    workspaces.find((w) => w.id === activeWorkspaceId) ?? workspaces[0] ?? null;
  if (workspaces.length <= 1) {
    return (
      <span
        aria-label={`当前工作区：${active?.name ?? "Workspace"}`}
        className="inline-flex items-center gap-2 text-sm text-muted-foreground"
      >
        <LayersIcon size={14} aria-hidden="true" />
        <span className="max-w-40 truncate">{active?.name ?? "Workspace"}</span>
      </span>
    );
  }

  return (
    <DropdownMenu
      align="end"
      triggerLabel={`切换工作区：${active?.name ?? "Workspace"}`}
      trigger={
        <span className="inline-flex items-center gap-2">
          <LayersIcon size={14} aria-hidden="true" />
          <span className="max-w-40 truncate">{active?.name ?? "Workspace"}</span>
          <DropdownChevron />
        </span>
      }
      items={workspaces.map((workspace) => ({
        type: "item",
        label: (
          <span className="flex w-full items-center justify-between gap-8">
            <span className="flex min-w-0 flex-col">
              <span className="truncate text-sm text-foreground">
                {workspace.name}
              </span>
              <span className="truncate font-mono text-xs text-muted-foreground">
                {workspace.slug}
              </span>
            </span>
            {workspace.id === activeWorkspaceId && (
              <CheckIcon size={14} className="text-brand-600" />
            )}
          </span>
        ),
        onSelect: () => onSwitch(workspace.id),
      }))}
    />
  );
}
