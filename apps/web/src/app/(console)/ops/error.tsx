"use client";

import { useEffect } from "react";
import { ErrorState } from "@/components/ui/feedback";
import { Button } from "@/components/ui/button";

/** R20：页面级 ErrorBoundary 兜底，友好文案 + 重试 + 返回，避免白屏。 */
export default function OpsError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // 生产级：错误可观测（错误边界自身不白屏）。
    console.error("[ops] page error:", error);
  }, [error]);

  return (
    <ErrorState
      title="运营商台加载失败"
      description="发生了意外错误。你可以重试一次，或返回上一页；问题持续时可联系支持并附上错误编号。"
      action={
        <div className="flex gap-2">
          <Button variant="primary" onClick={reset}>
            重试
          </Button>
          <Button variant="ghost" onClick={() => window.history.back()}>
            返回上一页
          </Button>
        </div>
      }
    />
  );
}
