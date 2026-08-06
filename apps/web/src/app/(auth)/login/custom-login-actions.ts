"use server";

import { cookies, headers } from "next/headers";
import { redirect } from "next/navigation";
import { getOrCreateDeviceFingerprint } from "@/lib/auth/device";
import {
  LoginFlowData,
  clearLoginFlow,
  getLoginFlow,
  setLoginFlow,
} from "@/lib/auth/login-flow";
import {
  forgetSession,
  rememberSession,
} from "@/lib/auth/zitadel-sessions-store";
import {
  ZitadelApiError,
  createCallback,
  createSession,
  getLoginSettings,
  getSession,
  humanMFAInitSkipped,
  listAuthenticationMethodTypes,
  requestOtpChallenge,
  requestWebAuthnChallenge,
  searchUsers,
  setSession,
  submitWebAuthnAssertion,
  validateSession,
} from "@/lib/auth/zitadel-session";
import {
  authConfig,
  isCustomLoginAllowedForOrg,
  isCustomLoginAllowedForUser,
} from "@/lib/auth/config";
import {
  buildAuthorizationUrl,
  generatePkcePair,
  generateState,
} from "@/lib/auth/oidc";
import { recordAuthEvent } from "@/lib/auth/auth-events";
import { AuthenticationMethodType } from "@zitadel/proto/zitadel/user/v2/user_service_pb";
import {
  CustomLoginActionState,
  OIDC_NEXT_COOKIE,
  OIDC_STATE_COOKIE,
  OIDC_VERIFIER_COOKIE,
} from "./login-state";

const initialState: CustomLoginActionState = { ok: false, step: "identifier" };

function safeNext(value: string | null | undefined): string | undefined {
  if (!value) return undefined;
  return value.startsWith("/") && !value.startsWith("//") ? value : undefined;
}

function errorState(
  prev: CustomLoginActionState,
  err: unknown,
  fallback: string,
): CustomLoginActionState {
  if (err instanceof ZitadelApiError) {
    const message =
      err.code === "not-found" || err.code === "permission-denied"
        ? "登录标识或凭据无效，请重试。"
        : err.code === "rate-limited"
          ? "尝试过于频繁，请稍后重试。"
          : err.code === "mfa-required"
            ? "需要完成多因素验证。"
            : err.code === "unavailable" || err.code === "transport"
              ? "身份服务暂不可用，请稍后重试。"
              : err.code === "invalid-response"
                ? "身份服务响应异常，请稍后重试。"
                : fallback;
    return {
      ...prev,
      ok: false,
      error: err.failedAttempts
        ? `${message} 剩余尝试次数：${err.failedAttempts}。`
        : message,
    };
  }
  return { ...prev, ok: false, error: fallback };
}

function isInfrastructureError(err: unknown): boolean {
  return (
    err instanceof ZitadelApiError &&
    ["unavailable", "transport", "not-configured"].includes(err.code)
  );
}

async function redirectToHostedOidc(next?: string): Promise<never> {
  const state = generateState();
  const pkce = generatePkcePair();
  const jar = await cookies();
  const cookieBase = {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax" as const,
    path: "/",
    maxAge: 600,
  };
  jar.set(OIDC_STATE_COOKIE, state, cookieBase);
  jar.set(OIDC_VERIFIER_COOKIE, pkce.verifier, cookieBase);
  jar.set(OIDC_NEXT_COOKIE, safeNext(next) ?? "/console", cookieBase);
  const url = await buildAuthorizationUrl({
    state,
    codeChallenge: pkce.challenge,
  });
  recordAuthEvent("custom_login.hosted_fallback", {
    next: safeNext(next),
  });
  redirect(url);
}

function methodLabel(method: AuthenticationMethodType): string {
  switch (method) {
    case AuthenticationMethodType.TOTP:
      return "TOTP";
    case AuthenticationMethodType.OTP_EMAIL:
      return "OTP_EMAIL";
    case AuthenticationMethodType.OTP_SMS:
      return "OTP_SMS";
    case AuthenticationMethodType.U2F:
      return "U2F";
    case AuthenticationMethodType.PASSKEY:
      return "PASSKEY";
    default:
      return "UNKNOWN";
  }
}

export async function completeLoginFlow(flow: LoginFlowData): Promise<never> {
  if (!flow.authRequestId) {
    throw new ZitadelApiError("登录请求已失效，请重新开始。", "session-invalid");
  }
  const callback = await createCallback({
    authRequestId: flow.authRequestId,
    sessionId: flow.sessionId,
    sessionToken: flow.sessionToken,
  });
  recordAuthEvent("custom_login.callback.completed", {
    userId: flow.userId,
    organizationId: flow.organizationId,
  });
  const jar = await cookies();
  if (flow.next) {
    jar.set(OIDC_NEXT_COOKIE, flow.next, {
      httpOnly: true,
      secure: process.env.NODE_ENV === "production",
      sameSite: "lax",
      path: "/",
      maxAge: 600,
    });
  }
  await clearLoginFlow();
  await rememberSession({
    sessionId: flow.sessionId,
    sessionToken: flow.sessionToken,
    loginName: flow.loginName ?? "",
    userId: flow.userId ?? "",
    organizationId: flow.organizationId,
  });
  redirect(callback.callbackUrl);
}

async function mfaMethodsFor(
  userId: string,
): Promise<string[]> {
  const methods = await listAuthenticationMethodTypes(userId);
  return methods
    .map(methodLabel)
    .filter((method) =>
      ["TOTP", "OTP_EMAIL", "OTP_SMS", "U2F", "PASSKEY"].includes(method),
    );
}

export async function submitCustomLoginIdentifier(
  _prev: CustomLoginActionState,
  formData: FormData,
): Promise<CustomLoginActionState> {
  const authRequestId = String(formData.get("authRequestId") ?? "").trim();
  const identifier = String(formData.get("identifier") ?? "").trim();
  const next = safeNext(String(formData.get("next") ?? ""));

  if (!authRequestId || !identifier) {
    return { ...initialState, error: "请输入登录标识。" };
  }

  try {
    const settings = await getLoginSettings();
    let users = await searchUsers({ loginName: identifier });
    if (users.length === 0 && !settings.disableLoginWithEmail) {
      users = await searchUsers({ email: identifier });
    }
    if (users.length === 0 && !settings.disableLoginWithPhone) {
      users = await searchUsers({ phone: identifier });
    }
    if (users.length > 1) {
      return {
        ...initialState,
        error: "该登录标识存在多个账号，请联系管理员。",
      };
    }
    const user = users[0];
    if (!user || !user.active) {
      return {
        ...initialState,
        error: "无法识别该登录标识。",
      };
    }
    if (
      !isCustomLoginAllowedForUser(user.userId) ||
      !isCustomLoginAllowedForOrg(user.organizationId)
    ) {
      return await redirectToHostedOidc(next);
    }

    const fingerprintId = await getOrCreateDeviceFingerprint();
    const requestHeaders = await headers();
    const userAgent = requestHeaders.get("user-agent") ?? "";
    const ip =
      requestHeaders.get("x-forwarded-for")?.split(",")[0]?.trim() ||
      requestHeaders.get("x-real-ip") ||
      "0.0.0.0";

    const created = await createSession({
      userId: user.userId,
      fingerprintId,
      ip,
      description: userAgent.slice(0, 200),
      userAgentHeader: userAgent ? { "user-agent": [userAgent] } : undefined,
      lifetimeSeconds: 24 * 60 * 60,
    });
    recordAuthEvent("custom_login.identifier.success", {
      userId: user.userId,
      organizationId: created.session.user?.organizationId,
    });
    await setLoginFlow({
      sessionId: created.sessionId,
      sessionToken: created.sessionToken,
      loginName:
        created.session.user?.loginName ||
        user.preferredLoginName ||
        user.username,
      userId: user.userId,
      organizationId:
        created.session.user?.organizationId || user.organizationId || undefined,
      authRequestId,
      next,
    });
    return {
      ok: false,
      step: "password",
      loginName:
        created.session.user?.loginName ||
        user.preferredLoginName ||
        user.username,
      userId: user.userId,
      sessionId: created.sessionId,
    };
  } catch (err) {
    if (isInfrastructureError(err)) {
      return await redirectToHostedOidc(next);
    }
    return errorState(initialState, err, "登录失败，请稍后重试。");
  }
}

export async function submitCustomLoginPassword(
  prev: CustomLoginActionState,
  formData: FormData,
): Promise<CustomLoginActionState> {
  const flow = await getLoginFlow();
  if (!flow) {
    return { ...initialState, error: "登录状态已过期，请重新开始。" };
  }
  const password = String(formData.get("password") ?? "");
  if (!password) {
    return { ...prev, error: "请输入密码。" };
  }

  try {
    const updated = await setSession({
      sessionId: flow.sessionId,
      sessionToken: flow.sessionToken,
      password,
    });
    const nextFlow = { ...flow, sessionToken: updated.sessionToken };
    await setLoginFlow(nextFlow);
    const session = await getSession({
      sessionId: updated.sessionId,
      sessionToken: updated.sessionToken,
    });
    const validation = await validateSession(session);
    if (validation.valid) {
      recordAuthEvent("custom_login.password.success", {
        userId: flow.userId,
      });
      return await completeLoginFlow(nextFlow);
    }
    if (validation.reason === "mfa-required" && session.user) {
      recordAuthEvent("custom_login.mfa_required", {
        userId: session.user.id,
      });
      const settings = await getLoginSettings(
        session.user.organizationId || undefined,
      );
      const methods = await mfaMethodsFor(session.user.id);
      return {
        ok: false,
        step: "mfa",
        loginName: session.user.loginName,
        userId: session.user.id,
        sessionId: session.id,
        mfaMethods: methods,
        mfaSetupRequired:
          methods.length === 0 &&
          Boolean(settings.mfaInitSkipLifetimeSeconds),
      };
    }
    return {
      ...prev,
      ok: false,
      error:
        validation.reason === "email-not-verified"
          ? "请先完成邮箱验证。"
          : "登录尚未完成，请重新开始。",
    };
  } catch (err) {
    if (isInfrastructureError(err)) {
      return await redirectToHostedOidc(flow.next);
    }
    return errorState(prev, err, "密码校验失败，请重试。");
  }
}

export async function requestCustomLoginOtp(
  prev: CustomLoginActionState,
  formData: FormData,
): Promise<CustomLoginActionState> {
  const flow = await getLoginFlow();
  if (!flow) {
    return { ...initialState, error: "登录状态已过期，请重新开始。" };
  }
  const method = String(formData.get("method") ?? "");
  if (method !== "otpEmail" && method !== "otpSms") {
    return { ...prev, error: "不支持的验证方式。" };
  }
  const pendingMfaMethod: "otpSms" | "otpEmail" =
    method === "otpSms" ? "otpSms" : "otpEmail";
  try {
    const updated = await requestOtpChallenge({
      sessionId: flow.sessionId,
      sessionToken: flow.sessionToken,
      method: method === "otpSms" ? "sms" : "email",
    });
    const nextFlow: LoginFlowData = {
      ...flow,
      sessionToken: updated.sessionToken,
      pendingMfaMethod,
    };
    await setLoginFlow(nextFlow);
    return {
      ok: false,
      step: "mfa",
      loginName: prev.loginName,
      userId: prev.userId,
      sessionId: flow.sessionId,
      mfaMethods: [method === "otpEmail" ? "OTP_EMAIL" : "OTP_SMS"],
      otpRequested: true,
    };
  } catch (err) {
    if (isInfrastructureError(err)) {
      return await redirectToHostedOidc(flow.next);
    }
    return errorState(prev, err, "验证码发送失败，请重试。");
  }
}

export async function submitCustomLoginMfa(
  prev: CustomLoginActionState,
  formData: FormData,
): Promise<CustomLoginActionState> {
  const flow = await getLoginFlow();
  if (!flow) {
    return { ...initialState, error: "登录状态已过期，请重新开始。" };
  }
  const method = String(formData.get("method") ?? "");
  const code = String(formData.get("code") ?? "");
  if (!code) {
    return { ...prev, error: "请输入验证码。" };
  }

  try {
    let updated;
    if (method === "totp") {
      updated = await setSession({
        sessionId: flow.sessionId,
        sessionToken: flow.sessionToken,
        totpCode: code,
      });
    } else if (method === "otpEmail") {
      updated = await setSession({
        sessionId: flow.sessionId,
        sessionToken: flow.sessionToken,
        otpEmailCode: code,
      });
    } else if (method === "otpSms") {
      updated = await setSession({
        sessionId: flow.sessionId,
        sessionToken: flow.sessionToken,
        otpSmsCode: code,
      });
    } else {
      return { ...prev, error: "不支持的验证方式。" };
    }

    const nextFlow = {
      ...flow,
      sessionToken: updated.sessionToken,
      pendingMfaMethod: undefined,
    };
    await setLoginFlow(nextFlow);
    const session = await getSession({
      sessionId: updated.sessionId,
      sessionToken: updated.sessionToken,
    });
    const validation = await validateSession(session);
    if (validation.valid) {
      return await completeLoginFlow(nextFlow);
    }
    return {
      ...prev,
      ok: false,
      error:
        validation.reason === "mfa-required"
          ? "多因素验证未完成，请重试。"
          : "登录尚未完成，请重新开始。",
    };
  } catch (err) {
    if (isInfrastructureError(err)) {
      return await redirectToHostedOidc(flow.next);
    }
    return errorState(prev, err, "验证码校验失败，请重试。");
  }
}

export async function requestCustomLoginWebAuthn(
  prev: CustomLoginActionState,
  formData: FormData,
): Promise<CustomLoginActionState> {
  const flow = await getLoginFlow();
  if (!flow) {
    return { ...initialState, error: "登录状态已过期，请重新开始。" };
  }
  const method = String(formData.get("method") ?? "");
  if (method !== "passkey" && method !== "u2f") {
    return { ...prev, error: "不支持的安全密钥类型。" };
  }
  try {
    const domain = new URL(authConfig.baseUrl).hostname;
    const updated = await requestWebAuthnChallenge({
      sessionId: flow.sessionId,
      sessionToken: flow.sessionToken,
      method,
      domain,
    });
    const nextFlow: LoginFlowData = {
      ...flow,
      sessionToken: updated.sessionToken,
    };
    await setLoginFlow(nextFlow);
    return {
      ok: false,
      step: "mfa",
      loginName: prev.loginName,
      userId: prev.userId,
      sessionId: flow.sessionId,
      mfaMethods: [method === "passkey" ? "PASSKEY" : "U2F"],
      webAuthnOptions: updated.options,
    };
  } catch (err) {
    if (isInfrastructureError(err)) {
      return await redirectToHostedOidc(flow.next);
    }
    return errorState(prev, err, "安全密钥验证启动失败，请重试。");
  }
}

export async function submitCustomLoginWebAuthn(
  assertionData: unknown,
): Promise<void> {
  const flow = await getLoginFlow();
  if (!flow) {
    throw new ZitadelApiError("登录状态已过期，请重新开始。", "session-invalid");
  }
  const updated = await submitWebAuthnAssertion({
    sessionId: flow.sessionId,
    sessionToken: flow.sessionToken,
    assertionData,
  });
  const nextFlow: LoginFlowData = {
    ...flow,
    sessionToken: updated.sessionToken,
  };
  await setLoginFlow(nextFlow);
  const session = await getSession({
    sessionId: updated.sessionId,
    sessionToken: updated.sessionToken,
  });
  const validation = await validateSession(session);
  if (!validation.valid) {
    throw new ZitadelApiError(
      validation.reason === "mfa-required"
        ? "多因素验证未完成，请重试。"
        : "登录尚未完成，请重新开始。",
      validation.reason === "mfa-required" ? "mfa-required" : "session-invalid",
    );
  }
  await completeLoginFlow(nextFlow);
}

export async function continueWithSavedSession(
  _prev: CustomLoginActionState,
  formData: FormData,
): Promise<CustomLoginActionState> {
  const authRequestId = String(formData.get("authRequestId") ?? "").trim();
  const next = safeNext(String(formData.get("next") ?? ""));
  const sessionId = String(formData.get("sessionId") ?? "").trim();
  const sessionToken = String(formData.get("sessionToken") ?? "").trim();
  const loginName = String(formData.get("loginName") ?? "").trim();
  const userId = String(formData.get("userId") ?? "").trim();
  const organizationId = String(formData.get("organizationId") ?? "").trim();

  if (!authRequestId || !sessionId || !sessionToken || !userId || !loginName) {
    return { ...initialState, error: "会话信息不完整，请重新登录。" };
  }

  try {
    const session = await getSession({ sessionId, sessionToken });
    const validation = await validateSession(session);
    if (!validation.valid) {
      await forgetSession(sessionId);
      return { ...initialState, error: "会话已失效，请重新登录。" };
    }
    const flow: LoginFlowData = {
      sessionId,
      sessionToken,
      loginName,
      userId,
      organizationId: organizationId || undefined,
      authRequestId,
      next,
    };
    await setLoginFlow(flow);
    return await completeLoginFlow(flow);
  } catch (err) {
    if (isInfrastructureError(err)) {
      return await redirectToHostedOidc(next);
    }
    return errorState(initialState, err, "继续会话失败，请重新登录。");
  }
}

export async function skipMfaSetup(
  _prev: CustomLoginActionState,
  formData: FormData,
): Promise<CustomLoginActionState> {
  void formData;
  const flow = await getLoginFlow();
  if (!flow || !flow.userId) {
    return { ...initialState, error: "登录状态已过期，请重新开始。" };
  }
  try {
    await humanMFAInitSkipped(flow.userId);
    return await completeLoginFlow(flow);
  } catch (err) {
    return errorState(initialState, err, "跳过 MFA 设置失败，请重试。");
  }
}
