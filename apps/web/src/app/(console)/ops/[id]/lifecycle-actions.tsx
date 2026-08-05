"use client";

import { useActionState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { activateProviderAction, transitionLifecycleAction } from "../actions";
import { Button } from "@/components/ui/button";
import { Alert, SuccessPanel } from "@/components/ui/feedback";
import { CodeBlock } from "@/components/ui/code-block";
import { Input, Select } from "@/components/ui/field";
import { KeyIcon, ZapIcon } from "@/components/ui/icons";
import type { Region } from "@/lib/api/operator";

// 与服务端 internal/domain/lifecycle.go 的状态机保持同步。
// 集成测试 TestLifecycleStateMachineMatrix 通过真实 HTTP API 遍历
// 每个 (state, action) 组合，强制前后端保持一致。
// 注意：REGISTERED 没有纯状态转移出口——激活（分配区域/Cell、创建测试环境）
// 是独立的资源分配事件，走 POST /providers/{id}/activate。
const ALLOWED_TRANSITIONS: Record<string, readonly string[]> = {
  REGISTERED: [],
  TEST_ACTIVE: ["LIVE_REVIEW"],
  LIVE_REVIEW: ["LIVE_ACTIVE", "RESTRICTED", "SUSPENDED"],
  LIVE_ACTIVE: ["RESTRICTED", "SUSPENDED", "OFFBOARDING"],
  RESTRICTED: ["LIVE_ACTIVE", "SUSPENDED", "OFFBOARDING"],
  SUSPENDED: ["LIVE_ACTIVE", "OFFBOARDING"],
  OFFBOARDING: [],
};

function isAllowed(currentState: string, target: string): boolean {
  const allowed = ALLOWED_TRANSITIONS[currentState];
  return allowed !== undefined && allowed.includes(target);
}

const ACTIONS: { to: string; label: string; variant: "outline" | "primary" | "danger-outline" | "danger"; hint: string }[] = [
  { to: "LIVE_REVIEW", label: "申请上线审核", variant: "outline", hint: "提交生产环境审核" },
  { to: "LIVE_ACTIVE", label: "激活生产环境", variant: "primary", hint: "正式对外提供服务" },
  { to: "RESTRICTED", label: "限制", variant: "danger-outline", hint: "限制生产流量" },
  { to: "SUSPENDED", label: "暂停", variant: "danger", hint: "暂停服务" },
];

export function LifecycleActions({
  providerId,
  currentState,
  regions,
}: {
  providerId: string;
  currentState: string;
  regions: Region[];
}) {
  const [state, formAction, pending] = useActionState(transitionLifecycleAction, {
    ok: false,
  });
  const [activateState, activateFormAction, activatePending] = useActionState(
    activateProviderAction,
    { ok: false },
  );
  const router = useRouter();

  // 转换/激活成功后强制刷新 Server Component，使 currentState prop 更新、
  // 操作面板重新渲染（revalidatePath 不会更新当前页 props）。
  useEffect(() => {
    if (state.ok || activateState.ok) {
      router.refresh();
    }
  }, [state.ok, activateState.ok, router]);

  const showActivate = currentState === "REGISTERED";

  return (
    <div className="space-y-3">
      {showActivate ? (
        <div className="space-y-3">
          <p className="text-xs leading-relaxed text-muted-foreground">
            激活将分配区域与单元（Cell）、创建测试环境并签发 API
            Key。这是 REGISTERED 状态的唯一出路，纯状态转移已被服务端拒绝。
          </p>
          <form
            action={activateFormAction}
            className="flex flex-wrap items-end gap-2"
          >
            <input type="hidden" name="provider_id" value={providerId} />
            <Select
              name="home_region_code"
              defaultValue=""
              className="min-w-[200px]"
              aria-label="所属区域"
            >
              <option value="" disabled>
                {regions.length === 0 ? "暂无可选区域" : "请选择区域…"}
              </option>
              {regions.map((r) => (
                <option key={r.id} value={r.code}>
                  {r.code} · {r.jurisdiction}
                </option>
              ))}
            </Select>
            <Input
              type="text"
              name="reason"
              maxLength={500}
              placeholder="操作原因（可选，写入审计）"
              className="min-w-[220px]"
              aria-label="操作原因（可选）"
            />
            <Button
              type="submit"
              variant="primary"
              size="sm"
              disabled={activatePending || regions.length === 0}
            >
              <ZapIcon size={14} aria-hidden="true" />
              激活测试环境
            </Button>
          </form>
        </div>
      ) : (
        <form action={formAction} className="flex flex-wrap items-end gap-2">
          <input type="hidden" name="provider_id" value={providerId} />
          <Input
            type="text"
            name="reason"
            maxLength={500}
            placeholder="操作原因（可选，写入审计）"
            className="min-w-[220px]"
            aria-label="操作原因（可选）"
          />
          {ACTIONS.map((a) => {
            const allowed = isAllowed(currentState, a.to);
            return (
              <Button
                key={a.to}
                type="submit"
                name="to"
                value={a.to}
                variant={a.variant}
                size="sm"
                disabled={pending || !allowed}
                title={
                  allowed
                    ? `${a.label}（${a.hint}）`
                    : `当前状态 ${currentState} 不允许此操作`
                }
              >
                {a.label}
              </Button>
            );
          })}
        </form>
      )}

      {!showActivate && state.error && (
        <Alert variant="danger" title="操作失败">
          {state.error}
        </Alert>
      )}

      {showActivate && activateState.error && (
        <Alert variant="danger" title="激活失败">
          {activateState.error}
        </Alert>
      )}

      {activateState.ok && activateState.apiKey && (
        <SuccessPanel
          title="测试环境已激活"
          description="这是测试环境 API Key，仅展示一次，请妥善保管。"
        >
          <div className="mt-3">
            <div className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-terminal-muted">
              <KeyIcon size={13} aria-hidden="true" />
              Test API Key
            </div>
            <CodeBlock code={activateState.apiKey} language="key" dense />
          </div>
        </SuccessPanel>
      )}

      {!showActivate && state.ok && state.apiKey && (
        <SuccessPanel
          title="生产环境已激活"
          description="这是生产环境 API Key，仅展示一次，请妥善保管。"
        >
          <div className="mt-3">
            <div className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-terminal-muted">
              <KeyIcon size={13} aria-hidden="true" />
              Live API Key
            </div>
            <CodeBlock code={state.apiKey} language="key" dense />
          </div>
        </SuccessPanel>
      )}
    </div>
  );
}
