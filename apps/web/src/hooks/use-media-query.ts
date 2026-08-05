"use client";

import { useSyncExternalStore } from "react";

function subscribe(query: string, onChange: () => void) {
  const mql = window.matchMedia(query);
  mql.addEventListener("change", onChange);
  return () => mql.removeEventListener("change", onChange);
}

function getSnapshot(query: string) {
  return window.matchMedia(query).matches;
}

/**
 * 响应式断点 hook：基于 window.matchMedia 的外部订阅，SSR 期间返回初始值。
 * 用于「窄屏收进用户菜单」等无法仅靠 CSS 完成的布局决策。
 */
export function useMediaQuery(query: string, initial = false): boolean {
  return useSyncExternalStore(
    (onChange) => subscribe(query, onChange),
    () => getSnapshot(query),
    () => initial,
  );
}
