"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { cn } from "@/lib/utils";
import { Button, type ButtonVariant } from "./button";
import { AlertIcon, ChevronDownIcon, XIcon } from "./icons";

/* ---------------- Dialog ---------------- */
export function Dialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  footer,
  size = "md",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  children?: ReactNode;
  footer?: ReactNode;
  size?: "sm" | "md" | "lg";
}) {
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (open) {
      const prev = document.activeElement as HTMLElement | null;
      panelRef.current?.focus();
      return () => prev?.focus();
    }
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onOpenChange(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, onOpenChange]);

  if (!open) return null;

  const sizes = { sm: "max-w-md", md: "max-w-lg", lg: "max-w-2xl" } as const;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div
        className="absolute inset-0 bg-neutral-950/45 animate-fade-in"
        onClick={() => onOpenChange(false)}
        aria-hidden="true"
      />
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="vlb-dialog-title"
        tabIndex={-1}
        className={cn(
          "surface-premium relative z-10 w-full rounded-2xl outline-none animate-slide-up",
          sizes[size],
        )}
      >
        <div className="flex items-start justify-between gap-4 px-5 pt-5">
          <div className="min-w-0 space-y-1">
            <h2 id="vlb-dialog-title" className="text-base font-semibold">
              {title}
            </h2>
            {description && (
              <p className="text-sm text-muted-foreground">{description}</p>
            )}
          </div>
          <button
            type="button"
            aria-label="关闭"
            onClick={() => onOpenChange(false)}
            className="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-2 hover:text-foreground"
          >
            <XIcon size={16} />
          </button>
        </div>
        {children && <div className="px-5 py-4">{children}</div>}
        {footer && (
          <div className="flex justify-end gap-2 border-t border-border px-5 py-4">
            {footer}
          </div>
        )}
      </div>
    </div>
  );
}

/* ---------------- ConfirmDialog ---------------- */
export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel = "确认",
  cancelLabel = "取消",
  variant = "danger",
  pending = false,
  confirmText,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: ButtonVariant;
  pending?: boolean;
  /** 传入时要求用户输入匹配文本后才可确认（type-to-confirm，§7.2）。 */
  confirmText?: string;
  onConfirm: () => void;
}) {
  const [typed, setTyped] = useState("");
  const canConfirm = !confirmText || typed === confirmText;

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={pending}>
            {cancelLabel}
          </Button>
          <Button
            variant={variant}
            onClick={() => {
              if (canConfirm) onConfirm();
            }}
            loading={pending}
            disabled={!canConfirm}
            data-testid="confirm-dialog-confirm"
          >
            {confirmLabel}
          </Button>
        </>
      }
    >
      <div className="flex items-start gap-2.5 text-sm text-muted-foreground">
        <AlertIcon size={16} className="mt-0.5 shrink-0 text-warning" />
        <div className="min-w-0">{description}</div>
      </div>
      {confirmText ? (
        <div className="mt-4">
          <label
            htmlFor="confirm-dialog-type"
            className="mb-1.5 block text-xs font-medium text-muted-foreground"
          >
            输入 <span className="font-mono text-foreground">{confirmText}</span> 确认
          </label>
          <input
            id="confirm-dialog-type"
            type="text"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            autoComplete="off"
            spellCheck={false}
            autoFocus
            aria-describedby="confirm-dialog-type-hint"
            className="w-full rounded-md border border-border bg-surface-1 px-3 py-2 text-sm text-foreground focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/30"
          />
          <p id="confirm-dialog-type-hint" className="mt-1.5 text-xs text-muted-foreground">
            输入应用名称后按钮才会启用。
          </p>
        </div>
      ) : null}
    </Dialog>
  );
}

/* ---------------- Tooltip ---------------- */
export function Tooltip({
  label,
  children,
  side = "top",
}: {
  label: string;
  children: ReactNode;
  side?: "top" | "bottom" | "left" | "right";
}) {
  const [visible, setVisible] = useState(false);
  const sidePos: Record<string, string> = {
    top: "bottom-full left-1/2 -translate-x-1/2 mb-1.5",
    bottom: "top-full left-1/2 -translate-x-1/2 mt-1.5",
    left: "right-full top-1/2 -translate-y-1/2 mr-1.5",
    right: "left-full top-1/2 -translate-y-1/2 ml-1.5",
  };
  return (
    <span
      className="relative inline-flex"
      onMouseEnter={() => setVisible(true)}
      onMouseLeave={() => setVisible(false)}
      onFocus={() => setVisible(true)}
      onBlur={() => setVisible(false)}
    >
      {children}
      {visible && (
        <span
          role="tooltip"
          className={cn(
            "pointer-events-none absolute z-50 whitespace-nowrap rounded-md bg-neutral-900 px-2 py-1 text-xs text-neutral-50 shadow-md animate-fade-in",
            sidePos[side],
          )}
        >
          {label}
        </span>
      )}
    </span>
  );
}

/* ---------------- DropdownMenu ---------------- */
export type DropdownItem =
  | {
      type: "item";
      label: ReactNode;
      onSelect: () => void;
      danger?: boolean;
      disabled?: boolean;
    }
  | { type: "separator" };

export function DropdownMenu({
  trigger,
  items,
  align = "end",
  triggerLabel,
}: {
  trigger: ReactNode;
  items: DropdownItem[];
  align?: "start" | "end";
  /** 触发器按钮的可访问名称（仅图标触发器时必填）。 */
  triggerLabel?: string;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const select = useCallback((item: DropdownItem) => {
    setOpen(false);
    if (item.type === "item") item.onSelect();
  }, []);

  return (
    <div ref={ref} className="relative inline-flex">
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={triggerLabel}
        onClick={() => setOpen((v) => !v)}
        className="inline-flex items-center gap-1.5 rounded-md text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        {trigger}
      </button>
      {open && (
        <div
          role="menu"
          className={cn(
            "absolute top-full z-40 mt-1.5 min-w-44 rounded-lg border border-border bg-surface-1 p-1 shadow-md animate-fade-in",
            align === "end" ? "right-0" : "left-0",
          )}
        >
          {items.map((item, i) =>
            item.type === "separator" ? (
              <div key={i} className="my-1 h-px bg-border" />
            ) : (
              <button
                key={i}
                role="menuitem"
                type="button"
                disabled={item.disabled}
                onClick={() => select(item)}
                className={cn(
                  "flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-sm transition-colors disabled:opacity-50",
                  item.danger
                    ? "text-danger hover:bg-danger-soft"
                    : "text-foreground hover:bg-surface-2",
                )}
              >
                {item.label}
              </button>
            ),
          )}
        </div>
      )}
    </div>
  );
}

/* ---------------- DropdownTrigger（带 chevron 的简单触发器） ---------------- */
export function DropdownChevron({ open }: { open?: boolean }) {
  return (
    <ChevronDownIcon
      size={14}
      className={cn("transition-transform", open && "rotate-180")}
    />
  );
}
