"use client";

import { Alert } from "@/components/ui/feedback";
import { Button } from "@/components/ui/button";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <div className="flex min-h-dvh items-center justify-center bg-canvas px-4">
      <div className="w-full max-w-md">
        <Alert variant="danger" title="页面出错了">
          <div className="space-y-3">
            <p>处理请求时发生未预期的错误。若问题持续出现，请提供下方错误标识以便排查。</p>
            {error.digest && (
              <p className="font-mono text-xs">错误标识：{error.digest}</p>
            )}
            <Button type="button" variant="outline" onClick={reset}>
              重试
            </Button>
          </div>
        </Alert>
      </div>
    </div>
  );
}
