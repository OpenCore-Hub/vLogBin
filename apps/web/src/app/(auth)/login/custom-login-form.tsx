"use client";

import { useActionState } from "react";
import {
  requestCustomLoginOtp,
  submitCustomLoginIdentifier,
  submitCustomLoginMfa,
  submitCustomLoginPassword,
} from "./custom-login-actions";
import { CustomLoginActionState } from "./login-state";
import { LoginSettingsSnapshot } from "@/lib/auth/zitadel-session";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { Alert } from "@/components/ui/feedback";
import { KeyIcon, ShieldIcon, UsersIcon } from "@/components/ui/icons";

const identifierInitial: CustomLoginActionState = {
  ok: false,
  step: "identifier",
};
const passwordInitial: CustomLoginActionState = {
  ok: false,
  step: "password",
};
const otpInitial: CustomLoginActionState = { ok: false, step: "mfa" };
const mfaInitial: CustomLoginActionState = { ok: false, step: "mfa" };

export function CustomLoginForm({
  authRequestId,
  next,
  loginSettings,
}: {
  authRequestId: string;
  next: string;
  loginSettings: LoginSettingsSnapshot;
}) {
  const [identifierState, identifierAction, identifierPending] = useActionState(
    submitCustomLoginIdentifier,
    identifierInitial,
  );
  const [passwordState, passwordAction, passwordPending] = useActionState(
    submitCustomLoginPassword,
    passwordInitial,
  );
  const [otpState, otpAction, otpPending] = useActionState(
    requestCustomLoginOtp,
    otpInitial,
  );
  const [mfaState, mfaAction, mfaPending] = useActionState(
    submitCustomLoginMfa,
    mfaInitial,
  );

  if (!loginSettings.allowUsernamePassword) {
    return (
      <Alert variant="warning" title="暂不支持本地账号登录">
        该组织未开放用户名密码登录，请使用 ZITADEL 托管登录页。
      </Alert>
    );
  }

  if (identifierState.step !== "password") {
    return (
      <form action={identifierAction} className="space-y-4">
        <input type="hidden" name="authRequestId" value={authRequestId} />
        <input type="hidden" name="next" value={next} />
        <Field
          label="登录标识"
          htmlFor="custom-login-identifier"
          hint="使用用户名、邮箱或手机号登录"
        >
          <div className="relative">
            <UsersIcon
              size={15}
              className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              id="custom-login-identifier"
              name="identifier"
              autoComplete="username"
              autoFocus
              required
              maxLength={200}
              className="pl-9"
            />
          </div>
        </Field>
        {identifierState.error && (
          <Alert variant="danger">{identifierState.error}</Alert>
        )}
        <Button
          type="submit"
          size="lg"
          loading={identifierPending}
          className="w-full"
        >
          <UsersIcon size={15} />
          继续
        </Button>
      </form>
    );
  }

  if (passwordState.step === "password") {
    return (
      <form action={passwordAction} className="space-y-4">
        <Field
          label="密码"
          htmlFor="custom-login-password"
          hint={passwordState.loginName ? `账号：${passwordState.loginName}` : undefined}
        >
          <div className="relative">
            <KeyIcon
              size={15}
              className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
            />
            <Input
              id="custom-login-password"
              name="password"
              type="password"
              autoComplete="current-password"
              autoFocus
              required
              className="pl-9"
            />
          </div>
        </Field>
        {passwordState.error && (
          <Alert variant="danger">{passwordState.error}</Alert>
        )}
        <Button
          type="submit"
          size="lg"
          loading={passwordPending}
          className="w-full"
        >
          <KeyIcon size={15} />
          登录
        </Button>
      </form>
    );
  }

  const methods = passwordState.mfaMethods ?? [];
  const hasTotp = methods.includes("TOTP");
  const showCodeForm = hasTotp || otpState.otpRequested;
  const hasPasskeyOnly =
    methods.length > 0 &&
    methods.every((m) => m === "PASSKEY" || m === "U2F");

  if (hasPasskeyOnly) {
    return (
      <div className="space-y-4">
        <Alert variant="warning" title="需要安全密钥">
          该账号需要安全密钥验证，请使用 ZITADEL 托管登录页完成登录。
        </Alert>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {showCodeForm ? (
        <form action={mfaAction} className="space-y-4">
          <input
            type="hidden"
            name="method"
            value={
              otpState.otpRequested
                ? otpState.mfaMethods?.[0] === "OTP_SMS"
                  ? "otpSms"
                  : "otpEmail"
                : "totp"
            }
          />
          <Field
            label="验证码"
            htmlFor="custom-login-mfa-code"
            hint="输入动态验证码"
          >
            <Input
              id="custom-login-mfa-code"
              name="code"
              inputMode="numeric"
              autoComplete="one-time-code"
              autoFocus
              required
              maxLength={8}
              className="font-mono tracking-[0.3em]"
            />
          </Field>
          {(mfaState.error || otpState.error) && (
            <Alert variant="danger">
              {mfaState.error || otpState.error}
            </Alert>
          )}
          <Button
            type="submit"
            size="lg"
            loading={mfaPending}
            className="w-full"
          >
            <ShieldIcon size={15} />
            验证
          </Button>
        </form>
      ) : (
        <div className="space-y-3">
          {methods.includes("OTP_EMAIL") && (
            <form action={otpAction}>
              <input type="hidden" name="method" value="otpEmail" />
              <Button
                type="submit"
                size="lg"
                variant="secondary"
                loading={otpPending}
                className="w-full"
              >
                发送邮箱验证码
              </Button>
            </form>
          )}
          {methods.includes("OTP_SMS") && (
            <form action={otpAction}>
              <input type="hidden" name="method" value="otpSms" />
              <Button
                type="submit"
                size="lg"
                variant="secondary"
                loading={otpPending}
                className="w-full"
              >
                发送短信验证码
              </Button>
            </form>
          )}
        </div>
      )}
    </div>
  );
}
