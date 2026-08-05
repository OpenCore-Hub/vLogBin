"use client";

import { useId, type ReactNode } from "react";
import { cn } from "@/lib/utils";

export function Tabs({
  items,
  value,
  onChange,
  className,
  size = "md",
}: {
  items: Array<{ value: string; label: ReactNode; badge?: ReactNode }>;
  value: string;
  onChange: (value: string) => void;
  className?: string;
  size?: "sm" | "md";
}) {
  const id = useId();
  return (
    <div
      role="tablist"
      aria-label="页面分区"
      className={cn(
        "inline-flex items-center gap-1 rounded-lg bg-surface-2 p-1",
        className,
      )}
    >
      {items.map((item) => {
        const selected = item.value === value;
        return (
          <button
            key={item.value}
            role="tab"
            id={`${id}-${item.value}`}
            aria-selected={selected}
            aria-controls={`${id}-panel-${item.value}`}
            onClick={() => onChange(item.value)}
            className={cn(
              "inline-flex items-center gap-1.5 rounded-md font-medium transition-colors",
              size === "sm" ? "px-2.5 py-1 text-xs" : "px-3 py-1.5 text-sm",
              selected
                ? "bg-surface-1 text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {item.label}
            {item.badge}
          </button>
        );
      })}
    </div>
  );
}

export function TabPanel({
  id,
  value,
  selected,
  children,
}: {
  id: string;
  value: string;
  selected: boolean;
  children: ReactNode;
}) {
  return (
    <div
      id={`${id}-panel-${value}`}
      role="tabpanel"
      aria-labelledby={`${id}-${value}`}
      hidden={!selected}
      className="focus:outline-none"
    >
      {children}
    </div>
  );
}
