"use client";

import { useCallback, useSyncExternalStore } from "react";
import { cn } from "@/lib/utils";
import { IconButton } from "@/components/ui/button";
import { MoonIcon, SunIcon } from "@/components/ui/icons";

export const THEME_COOKIE = "vlb_theme";

type Theme = "light" | "dark";

function currentTheme(): Theme {
  if (typeof document === "undefined") return "light";
  return document.documentElement.classList.contains("dark") ? "dark" : "light";
}

function subscribeTheme(callback: () => void): () => void {
  if (typeof document === "undefined") return () => {};
  const el = document.documentElement;
  const observer = new MutationObserver(callback);
  observer.observe(el, { attributes: true, attributeFilter: ["class"] });
  return () => observer.disconnect();
}

/** 主题切换：切换 html.dark 并持久化到 cookie（非 httpOnly，客户端可写）。 */
export function ThemeToggle() {
  const theme = useSyncExternalStore(subscribeTheme, currentTheme, () => "light");

  const toggle = useCallback(() => {
    const next: Theme = theme === "dark" ? "light" : "dark";
    document.documentElement.classList.toggle("dark", next === "dark");
    document.cookie = `${THEME_COOKIE}=${next}; path=/; max-age=${60 * 60 * 24 * 365}; samesite=lax`;
  }, [theme]);

  return (
    <IconButton
      label={theme === "dark" ? "切换到亮色" : "切换到暗色"}
      onClick={toggle}
      className={cn("size-8", "text-muted-foreground hover:text-foreground")}
    >
      {theme === "dark" ? <SunIcon size={16} /> : <MoonIcon size={16} />}
    </IconButton>
  );
}
