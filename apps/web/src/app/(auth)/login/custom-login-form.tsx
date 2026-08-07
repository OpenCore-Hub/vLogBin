"use client";

import {
  useActionState,
  useEffect,
  useRef,
  useState,
  useTransition,
} from "react";
import {
  continueWithSavedSession,
  requestCustomLoginWebAuthn,
  requestCustomLoginOtp,
  skipMfaSetup,
  startCustomLoginIdp,
  submitCustomLoginIdentifier,
  submitCustomLoginMfa,
  submitCustomLoginPassword,
  submitCustomLoginWebAuthn,
} from "./custom-login-actions";
import { CustomLoginActionState } from "./login-state";
import {
  ActiveIdentityProvider,
  LoginSettingsSnapshot,
} from "@/lib/auth/zitadel-session";
import { startOidcLogin } from "./login-actions";
import {
  assertionToJson,
  coerceWebAuthnRequestOptions,
} from "@/lib/auth/webauthn-client";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/field";
import { Alert } from "@/components/ui/feedback";
import {
  Building2Icon,
  GlobeIcon,
  KeyIcon,
  ShieldIcon,
  UsersIcon,
} from "@/components/ui/icons";

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
const webAuthnInitial: CustomLoginActionState = { ok: false, step: "mfa" };
const skipMfaInitial: CustomLoginActionState = { ok: false, step: "mfa" };

export function CustomLoginForm({
  authRequestId,
  next,
  loginSettings,
  savedSessions,
  identityProviders,
  initialState,
}: {
  authRequestId: string;
  next: string;
  loginSettings: LoginSettingsSnapshot;
  savedSessions: Array<{
    sessionId: string;
    sessionToken: string;
    loginName: string;
    userId: string;
    organizationId?: string;
  }>;
  identityProviders: ActiveIdentityProvider[];
  initialState?: CustomLoginActionState;
}) {
  const initial: CustomLoginActionState = initialState ?? identifierInitial;
  const passwordInitialState: CustomLoginActionState =
    initial.step === "mfa" ? { ...initial, step: "mfa" } : passwordInitial;
  const [identifierState, identifierAction, identifierPending] = useActionState(
    submitCustomLoginIdentifier,
    initial,
  );
  const [idpState, idpAction, idpPending] = useActionState(
    startCustomLoginIdp,
    initial,
  );
  const [savedSessionState, savedSessionAction, savedSessionPending] =
    useActionState(continueWithSavedSession, identifierInitial);
  const [passwordState, passwordAction, passwordPending] = useActionState(
    submitCustomLoginPassword,
    passwordInitialState,
  );
  const [otpState, otpAction, otpPending] = useActionState(
    requestCustomLoginOtp,
    otpInitial,
  );
  const [mfaState, mfaAction, mfaPending] = useActionState(
    submitCustomLoginMfa,
    mfaInitial,
  );
  const [webAuthnState, webAuthnAction, webAuthnPending] = useActionState(
    requestCustomLoginWebAuthn,
    webAuthnInitial,
  );
  const [skipMfaState, skipMfaAction, skipMfaPending] = useActionState(
    skipMfaSetup,
    skipMfaInitial,
  );
  const [, startWebAuthnSubmit] = useTransition();
  const [webAuthnError, setWebAuthnError] = useState<string | null>(null);
  const idpFormRef = useRef<HTMLFormElement>(null);

  useEffect(() => {
    if (idpState.step === "idp-post" && idpFormRef.current) {
      idpFormRef.current.submit();
    }
  }, [idpState.step]);

  useEffect(() => {
    const options = webAuthnState.webAuthnOptions as
      | PublicKeyCredentialRequestOptions
      | undefined;
    if (!options) return;
    let cancelled = false;
    void (async () => {
      try {
        const assertion = await navigator.credentials.get({
          publicKey: coerceWebAuthnRequestOptions(options),
        });
        if (cancelled || !assertion) return;
        const credential = assertion as PublicKeyCredential;
        const payload = assertionToJson(credential);
        startWebAuthnSubmit(() => {
          void submitCustomLoginWebAuthn(payload).catch((err) => {
            setWebAuthnError(
              err instanceof Error ? err.message : "安全密钥验证失败，请重试。",
            );
          });
        });
      } catch (err) {
        if (!cancelled) {
          setWebAuthnError(
            err instanceof Error ? err.message : "安全密钥验证失败，请重试。",
          );
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [webAuthnState.webAuthnOptions, startWebAuthnSubmit]);

  if (!loginSettings.allowUsernamePassword && identityProviders.length === 0) {
    return (
      <Alert variant="warning" title="暂不支持本地账号登录">
        该组织未开放本地账号或企业身份登录，请联系管理员。
      </Alert>
    );
  }

  if (
    idpState.step === "idp-post" &&
    idpState.idpFormUrl &&
    idpState.idpFormFields
  ) {
    return (
      <form
        ref={idpFormRef}
        action={idpState.idpFormUrl}
        method="post"
        className="space-y-4"
      >
        {Object.entries(idpState.idpFormFields).map(([key, value]) => (
          <input key={key} type="hidden" name={key} value={value} />
        ))}
        <div className="flex items-center justify-center gap-2 text-sm text-muted-foreground">
          <ShieldIcon size={15} className="animate-pulse" />
          正在跳转到 {idpState.idpName ?? "企业身份源"}…
        </div>
        <noscript>
          <Button type="submit" size="lg" className="w-full">
            继续
          </Button>
        </noscript>
      </form>
    );
  }

  if (
    identifierState.step !== "password" &&
    identifierState.step !== "mfa"
  ) {
    return (
      <div className="space-y-4">
        {savedSessions.length > 0 && (
          <div className="space-y-3">
            {savedSessions.map((session) => (
              <form key={session.sessionId} action={savedSessionAction}>
                <input type="hidden" name="authRequestId" value={authRequestId} />
                <input type="hidden" name="next" value={next} />
                <input type="hidden" name="sessionId" value={session.sessionId} />
                <input
                  type="hidden"
                  name="sessionToken"
                  value={session.sessionToken}
                />
                <input type="hidden" name="loginName" value={session.loginName} />
                <input type="hidden" name="userId" value={session.userId} />
                <input
                  type="hidden"
                  name="organizationId"
                  value={session.organizationId ?? ""}
                />
                <Button
                  type="submit"
                  size="lg"
                  variant="secondary"
                  loading={savedSessionPending}
                  className="w-full"
                >
                  <UsersIcon size={15} />
                  继续使用 {session.loginName}
                </Button>
              </form>
            ))}
          </div>
        )}
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
          {savedSessionState.error && (
            <Alert variant="danger">{savedSessionState.error}</Alert>
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
        {loginSettings.allowExternalIdp && (
          identityProviders.length > 0 ? (
            <>
              <div className="relative" aria-hidden="true">
                <div className="absolute inset-x-0 top-1/2 border-t border-border" />
                <span className="relative z-10 mx-auto flex w-fit bg-background px-3 text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
                  或
                </span>
              </div>
              <div className="space-y-3">
                {identityProviders.map((provider) => (
                  <form key={provider.id} action={idpAction}>
                    <input
                      type="hidden"
                      name="authRequestId"
                      value={authRequestId}
                    />
                    <input type="hidden" name="next" value={next} />
                    <input type="hidden" name="idpId" value={provider.id} />
                    <Button
                      type="submit"
                      size="lg"
                      variant="secondary"
                      loading={idpPending}
                      className="w-full"
                    >
                      <Building2Icon size={15} />
                      {provider.name}
                    </Button>
                  </form>
                ))}
              </div>
              {idpState.error && (
                <Alert variant="danger">{idpState.error}</Alert>
              )}
            </>
          ) : (
            <form action={startOidcLogin}>
              <input type="hidden" name="next" value={next} />
              <Button
                type="submit"
                size="lg"
                variant="secondary"
                className="w-full"
              >
                <GlobeIcon size={15} />
                使用 ZITADEL 托管登录
              </Button>
            </form>
          )
        )}
      </div>
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

  return (
    <div className="space-y-4">
      {showCodeForm && (
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
      )}
      <div className="space-y-3">
        {!otpState.otpRequested && methods.includes("OTP_EMAIL") && (
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
        {!otpState.otpRequested && methods.includes("OTP_SMS") && (
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
        {methods.includes("PASSKEY") && (
          <form action={webAuthnAction}>
            <input type="hidden" name="method" value="passkey" />
            <Button
              type="submit"
              size="lg"
              variant="secondary"
              loading={webAuthnPending}
              className="w-full"
            >
              使用 Passkey
            </Button>
          </form>
        )}
        {methods.includes("U2F") && (
          <form action={webAuthnAction}>
            <input type="hidden" name="method" value="u2f" />
            <Button
              type="submit"
              size="lg"
              variant="secondary"
              loading={webAuthnPending}
              className="w-full"
            >
              使用安全密钥
            </Button>
          </form>
        )}
        {methods.length === 0 && passwordState.mfaSetupRequired && (
          <form action={skipMfaAction}>
            <Button
              type="submit"
              size="lg"
              variant="secondary"
              loading={skipMfaPending}
              className="w-full"
            >
              暂不设置，直接进入控制台
            </Button>
          </form>
        )}
        {skipMfaState.error && (
          <Alert variant="danger">{skipMfaState.error}</Alert>
        )}
        {webAuthnError && <Alert variant="danger">{webAuthnError}</Alert>}
      </div>
    </div>
  );
}
