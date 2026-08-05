"use client";

import Link from "next/link";
import type {
  AnchorHTMLAttributes,
  ButtonHTMLAttributes,
  ReactNode,
} from "react";
import { cn } from "@/lib/utils";
import { SpinnerIcon } from "./icons";

export type ButtonVariant =
  | "primary"
  | "secondary"
  | "outline"
  | "ghost"
  | "danger"
  | "danger-outline";

export type ButtonSize = "sm" | "md" | "lg" | "icon";

const VARIANTS: Record<ButtonVariant, string> = {
  primary:
    "bg-brand-700 text-white hover:bg-brand-800 shadow-[var(--shadow-premium)]",
  secondary:
    "bg-surface-1 text-foreground border border-border hover:bg-surface-2 shadow-[var(--shadow-inset-highlight)]",
  outline:
    "bg-transparent text-foreground border border-border-strong hover:border-brand-500 hover:text-brand-700 hover:bg-brand-50/40",
  ghost: "bg-transparent text-muted-foreground hover:bg-surface-2 hover:text-foreground",
  danger: "bg-danger text-white hover:bg-danger/90 active:bg-danger/80 shadow-sm",
  "danger-outline":
    "bg-transparent text-danger border border-danger/40 hover:bg-danger-soft",
};

const SIZES: Record<ButtonSize, string> = {
  sm: "h-8 px-3 text-xs gap-1.5",
  md: "h-9.5 px-4 text-sm gap-2",
  lg: "h-11 px-6 text-sm gap-2",
  icon: "h-9 w-9",
};

function baseClasses(variant: ButtonVariant, size: ButtonSize) {
  return cn(
    "inline-flex shrink-0 items-center justify-center rounded-full font-medium",
    "pressable select-none whitespace-nowrap will-change-transform",
    "disabled:pointer-events-none disabled:opacity-55",
    VARIANTS[variant],
    SIZES[size],
  );
}

type CommonProps = {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  children?: ReactNode;
};

export function Button({
  variant = "primary",
  size = "md",
  loading = false,
  className,
  children,
  disabled,
  ...props
}: CommonProps & ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      className={cn(baseClasses(variant, size), className)}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      {...props}
    >
      {loading && <SpinnerIcon size={size === "sm" ? 13 : 15} className="animate-spin" />}
      {children}
    </button>
  );
}

type LinkButtonProps = CommonProps &
  AnchorHTMLAttributes<HTMLAnchorElement> & { href: string; prefetch?: boolean };

export function LinkButton({
  variant = "primary",
  size = "md",
  href,
  className,
  children,
  ...props
}: LinkButtonProps) {
  return (
    <Link
      href={href}
      className={cn(baseClasses(variant, size), className)}
      {...props}
    >
      {children}
    </Link>
  );
}

export function IconButton({
  variant = "ghost",
  size = "icon",
  label,
  className,
  children,
  ...props
}: CommonProps &
  ButtonHTMLAttributes<HTMLButtonElement> & { label: string }) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={cn(baseClasses(variant, size), "rounded-md", className)}
      {...props}
    >
      {children}
    </button>
  );
}
