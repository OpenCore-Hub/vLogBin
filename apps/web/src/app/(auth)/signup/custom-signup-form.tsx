"use client";

import { useActionState } from "react";
import {
  CustomSignupActionState,
  resendSignupEmailCode,
  submitCustomSignup,
  submitSignupEmailCode,
} from "./custom-signup-actions";
import { LoginSettingsSnapshot } from "@/lib/auth/zitadel-session";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { Alert } from "@/components/ui/feedback";
import { CheckIcon, ShieldIcon, UsersIcon } from "@/components/ui/icons";

const formInitial: CustomSignupActionState = { ok: false, step: "form" };
const verifyInitial: CustomSignupActionState = {
  ok: false,
  step: "verify-email",
};

export function CustomSignupForm({
  authRequestId,
  next,
  loginSettings,
}: {
  authRequestId: string;
  next: string;
  loginSettings: LoginSettingsSnapshot;
}) {
  const [formState, formAction, formPending] = useActionState(
    submitCustomSignup,
    formInitial,
  );
  const [verifyState, verifyAction, verifyPending] = useActionState(
    submitSignupEmailCode,
    verifyInitial,
  );
  const [resendState, resendAction, resendPending] = useActionState(
    resendSignupEmailCode,
    verifyInitial,
  );

  if (!loginSettings.allowRegister || !loginSettings.allowUsernamePassword) {
    return (
      <Alert variant="warning" title="暂不支持自助注册">
        当前组织未开放自助注册，请联系管理员。
      </Alert>
    );
  }

  if (formState.step === "verify-email" || verifyState.step === "verify-email") {
    const email = verifyState.email || formState.email || "";
    return (
      <div className="space-y-4">
        <form action={verifyAction} className="space-y-4">
          <Field
            label="邮箱验证码"
            htmlFor="custom-signup-code"
            hint={email ? `验证码已发送至 ${email}` : undefined}
          >
            <Input
              id="custom-signup-code"
              name="code"
              inputMode="numeric"
              autoComplete="one-time-code"
              autoFocus
              required
              maxLength={8}
              className="font-mono tracking-[0.3em]"
            />
          </Field>
          {(verifyState.error || resendState.error) && (
            <Alert variant="danger">
              {verifyState.error || resendState.error}
            </Alert>
          )}
          <Button
            type="submit"
            size="lg"
            loading={verifyPending}
            className="w-full"
          >
            <ShieldIcon size={15} />
            验证并完成注册
          </Button>
        </form>
        <form action={resendAction}>
          <Button
            type="submit"
            size="lg"
            variant="secondary"
            loading={resendPending}
            className="w-full"
          >
            <CheckIcon size={15} />
            重新发送验证码
          </Button>
        </form>
      </div>
    );
  }

  return (
    <form action={formAction} className="space-y-4">
      <input type="hidden" name="authRequestId" value={authRequestId} />
      <input type="hidden" name="next" value={next} />
      <Field label="邮箱" htmlFor="custom-signup-email">
        <Input
          id="custom-signup-email"
          name="email"
          type="email"
          autoComplete="email"
          autoFocus
          required
          maxLength={200}
        />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="名字" htmlFor="custom-signup-given-name">
          <Input
            id="custom-signup-given-name"
            name="givenName"
            autoComplete="given-name"
            required
            maxLength={200}
          />
        </Field>
        <Field label="姓氏" htmlFor="custom-signup-family-name">
          <Input
            id="custom-signup-family-name"
            name="familyName"
            autoComplete="family-name"
            required
            maxLength={200}
          />
        </Field>
      </div>
      <Field
        label="密码"
        htmlFor="custom-signup-password"
        hint="至少 8 位，用于初始登录"
      >
        <Input
          id="custom-signup-password"
          name="password"
          type="password"
          autoComplete="new-password"
          required
          minLength={8}
          maxLength={200}
        />
      </Field>
      {formState.error && <Alert variant="danger">{formState.error}</Alert>}
      <Button type="submit" size="lg" loading={formPending} className="w-full">
        <UsersIcon size={15} />
        创建账号
      </Button>
    </form>
  );
}
