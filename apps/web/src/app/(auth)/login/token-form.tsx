"use client";

import { useActionState, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { loginWithOperatorToken } from "./login-actions";
import type { LoginActionState } from "./login-state";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { Alert } from "@/components/ui/feedback";
import { EyeIcon, EyeOffIcon, KeyIcon } from "@/components/ui/icons";

const initialState: LoginActionState = { ok: false };

export function OperatorTokenForm({ next }: { next: string }) {
  const [state, formAction, pending] = useActionState<
    LoginActionState,
    FormData
  >(loginWithOperatorToken, initialState);
  const [show, setShow] = useState(false);
  const router = useRouter();

  useEffect(() => {
    if (state.ok) {
      router.replace(state.next ?? "/console");
      router.refresh();
    }
  }, [state.ok, state.next, router]);

  return (
    <form action={formAction} className="space-y-4">
      <input type="hidden" name="next" value={next} />
      <Field
        label="Operator Token"
        htmlFor="operator-token"
        hint="平台 API 的操作员令牌（OPERATOR_TOKEN），仅用于本地开发模式"
      >
        <div className="relative">
          <Input
            id="operator-token"
            type={show ? "text" : "password"}
            name="token"
            autoComplete="off"
            autoFocus
            required
            placeholder="vlb_op_..."
            className="pr-10 font-mono"
          />
          <button
            type="button"
            aria-label={show ? "隐藏" : "显示"}
            onClick={() => setShow((v) => !v)}
            className="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-muted-foreground hover:text-foreground"
          >
            {show ? <EyeOffIcon size={15} /> : <EyeIcon size={15} />}
          </button>
        </div>
      </Field>

      {state.error && <Alert variant="danger">{state.error}</Alert>}

      <Button type="submit" size="lg" loading={pending} className="w-full">
        <KeyIcon size={15} />
        登录
      </Button>
    </form>
  );
}
