"use client";

import { cn } from "@/lib/utils";
import { Button } from "./button";
import { ChevronLeftIcon, ChevronRightIcon } from "./icons";

type PageItem = number | "ellipsis-left" | "ellipsis-right";

function getPageItems(page: number, totalPages: number): PageItem[] {
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, i) => i + 1);
  }
  const items = new Set<number>([1, totalPages, page - 1, page, page + 1]);
  const sorted = [...items]
    .filter((n) => n >= 1 && n <= totalPages)
    .sort((a, b) => a - b);
  const result: PageItem[] = [];
  for (let i = 0; i < sorted.length; i += 1) {
    const current = sorted[i] as number;
    const prev = result[result.length - 1];
    if (typeof prev === "number" && current - prev > 1) {
      result.push(current > prev + 2 ? "ellipsis-left" : prev + 1);
    }
    result.push(current);
  }
  return result;
}

export function Pagination({
  page,
  totalPages,
  onPageChange,
  className,
}: {
  page: number;
  totalPages: number;
  onPageChange: (page: number) => void;
  className?: string;
}) {
  if (totalPages <= 1) return null;

  const items = getPageItems(page, totalPages);
  return (
    <nav
      aria-label="分页"
      className={cn("flex items-center gap-1.5", className)}
    >
      <Button
        variant="outline"
        size="sm"
        disabled={page <= 1}
        onClick={() => onPageChange(page - 1)}
      >
        <ChevronLeftIcon size={14} aria-hidden="true" />
        上一页
      </Button>
      {items.map((item, index) =>
        typeof item === "number" ? (
          <Button
            key={item}
            variant={item === page ? "primary" : "outline"}
            size="sm"
            aria-current={item === page ? "page" : undefined}
            onClick={() => onPageChange(item)}
            className="min-w-8 px-2"
          >
            {item}
          </Button>
        ) : (
          <span
            key={`${item}-${index}`}
            className="px-1 text-sm text-muted-foreground"
            aria-hidden="true"
          >
            …
          </span>
        ),
      )}
      <Button
        variant="outline"
        size="sm"
        disabled={page >= totalPages}
        onClick={() => onPageChange(page + 1)}
      >
        下一页
        <ChevronRightIcon size={14} aria-hidden="true" />
      </Button>
    </nav>
  );
}
