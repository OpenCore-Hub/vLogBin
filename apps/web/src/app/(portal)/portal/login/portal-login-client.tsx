"use client";

import { useActionState } from "react";
import { Field, Input } from "@/components/ui/field";
import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/feedback";
import { LogoCompact } from "@/components/brand/logo";
import { loginPortalAction, type PortalActionState } from "../actions";

const initialState: PortalActionState = { ok: false };

export function PortalLoginClient({ initialToken = "" }: { initialToken?: string }) {
  const [state, formAction, pending] = useActionState(loginPortalAction, initialState);
  return (
    <main className="flex min-h-screen items-center justify-center bg-canvas px-4">
      <div className="w-full max-w-md space-y-6">
        <div className="flex justify-center">
          <LogoCompact />
        </div>
        <div className="rounded-xl border border-border bg-surface-1 p-6">
          <h1 className="text-xl font-semibold tracking-tight">客户门户</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            使用邀请链接中的 Token 登录，仅可查看自己的账单与用量。
          </p>
          <form action={formAction} className="mt-5 space-y-4">
            <Field label="门户邀请 Token" htmlFor="portal-token">
              <Input
                id="portal-token"
                name="token"
                defaultValue={initialToken}
                autoComplete="off"
                required
                placeholder="粘贴邀请 Token"
                className="font-mono text-xs"
              />
            </Field>
            {state.error && <Alert title="登录失败">{state.error}</Alert>}
            <Button type="submit" loading={pending} className="w-full">
              进入门户
            </Button>
          </form>
        </div>
      </div>
    </main>
  );
}
