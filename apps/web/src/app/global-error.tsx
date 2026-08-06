"use client";

import { Button } from "@/components/ui/button";
import { LogoMark } from "@/components/ui/icons";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <html lang="zh-CN">
      <body className="min-h-dvh bg-canvas text-foreground">
        <main className="flex min-h-dvh items-center justify-center px-4">
          <div className="surface-premium w-full max-w-md rounded-2xl border border-border p-8 text-center">
            <LogoMark size={28} className="mx-auto" />
            <h1 className="mt-5 text-xl font-semibold tracking-tight">
              应用出现异常
            </h1>
            <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
              请重试；若持续出现，可提供错误标识协助排查。
            </p>
            {error.digest && (
              <p className="mt-3 font-mono text-xs text-muted-foreground">
                错误标识：{error.digest}
              </p>
            )}
            <div className="mt-6 flex justify-center">
              <Button variant="primary" onClick={reset}>
                重试
              </Button>
            </div>
          </div>
        </main>
      </body>
    </html>
  );
}
