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
  ZitadelApiError,
  createCallback,
  createSession,
  getLoginSettings,
  getSession,
  listAuthenticationMethodTypes,
  requestOtpChallenge,
  searchUsers,
  setSession,
  validateSession,
} from "@/lib/auth/zitadel-session";
import { AuthenticationMethodType } from "@zitadel/proto/zitadel/user/v2/user_service_pb";
import {
  CustomLoginActionState,
  OIDC_NEXT_COOKIE,
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

async function completeFlow(flow: LoginFlowData): Promise<never> {
  if (!flow.authRequestId) {
    throw new ZitadelApiError("登录请求已失效，请重新开始。", "session-invalid");
  }
  const callback = await createCallback({
    authRequestId: flow.authRequestId,
    sessionId: flow.sessionId,
    sessionToken: flow.sessionToken,
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
      return await completeFlow(nextFlow);
    }
    if (validation.reason === "mfa-required" && session.user) {
      return {
        ok: false,
        step: "mfa",
        loginName: session.user.loginName,
        userId: session.user.id,
        sessionId: session.id,
        mfaMethods: await mfaMethodsFor(session.user.id),
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
      return await completeFlow(nextFlow);
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
    return errorState(prev, err, "验证码校验失败，请重试。");
  }
}
