"use server";

import { z } from "zod";
import { completeLoginFlow } from "../login/custom-login-actions";
import {
  createSession,
  createUser,
  getLoginSettings,
  getSession,
  resendEmailCode,
  validateSession,
  verifyEmail,
  ZitadelApiError,
} from "@/lib/auth/zitadel-session";
import {
  getLoginFlow,
  setLoginFlow,
} from "@/lib/auth/login-flow";
import {
  clearSignupFlow,
  getSignupFlow,
  setSignupFlow,
} from "@/lib/auth/signup-flow";

export interface CustomSignupActionState {
  ok: boolean;
  error?: string;
  step: "form" | "verify-email";
  email?: string;
  next?: string;
}

const initialState: CustomSignupActionState = { ok: false, step: "form" };

function safeNext(value: string | null | undefined): string | undefined {
  if (!value) return undefined;
  return value.startsWith("/") && !value.startsWith("//") ? value : undefined;
}

function errorState(
  prev: CustomSignupActionState,
  err: unknown,
  fallback: string,
): CustomSignupActionState {
  if (err instanceof ZitadelApiError) {
    const message =
      err.code === "conflict"
        ? "该邮箱已注册，请直接登录。"
        : err.code === "rate-limited"
          ? "尝试过于频繁，请稍后重试。"
          : err.code === "unavailable" || err.code === "transport"
            ? "身份服务暂不可用，请稍后重试。"
            : fallback;
    return { ...prev, ok: false, error: message };
  }
  return { ...prev, ok: false, error: fallback };
}

export async function submitCustomSignup(
  _prev: CustomSignupActionState,
  formData: FormData,
): Promise<CustomSignupActionState> {
  const authRequestId = String(formData.get("authRequestId") ?? "").trim();
  const next = safeNext(String(formData.get("next") ?? ""));
  const email = String(formData.get("email") ?? "").trim().toLowerCase();
  const givenName = String(formData.get("givenName") ?? "").trim();
  const familyName = String(formData.get("familyName") ?? "").trim();
  const password = String(formData.get("password") ?? "");

  const parsed = z
    .object({
      authRequestId: z.string().min(1),
      email: z.string().email().max(200),
      givenName: z.string().min(1).max(200),
      familyName: z.string().min(1).max(200),
      password: z.string().min(8).max(200),
    })
    .safeParse({ authRequestId, email, givenName, familyName, password });
  if (!parsed.success) {
    return { ...initialState, error: "请完整填写注册信息，密码至少 8 位。" };
  }

  try {
    const settings = await getLoginSettings();
    if (!settings.allowRegister || !settings.allowUsernamePassword) {
      return {
        ...initialState,
        error: "当前组织未开放自助注册，请联系管理员。",
      };
    }
    const created = await createUser({
      email,
      givenName,
      familyName,
      password,
      sendEmailCode: true,
    });
    const session = await createSession({
      userId: created.userId,
      password,
      lifetimeSeconds: 24 * 60 * 60,
    });
    await setLoginFlow({
      sessionId: session.sessionId,
      sessionToken: session.sessionToken,
      loginName: email,
      userId: created.userId,
      organizationId: session.session.user?.organizationId,
      authRequestId,
      next,
    });
    await setSignupFlow({
      authRequestId,
      next,
      userId: created.userId,
      email,
      givenName,
      familyName,
    });
    return { ok: false, step: "verify-email", email, next };
  } catch (err) {
    return errorState(initialState, err, "注册失败，请稍后重试。");
  }
}

export async function submitSignupEmailCode(
  prev: CustomSignupActionState,
  formData: FormData,
): Promise<CustomSignupActionState> {
  const code = String(formData.get("code") ?? "").trim();
  if (!code) {
    return { ...prev, error: "请输入邮箱验证码。" };
  }
  const signupFlow = await getSignupFlow();
  const loginFlow = await getLoginFlow();
  if (!signupFlow || !loginFlow) {
    return { ...initialState, error: "注册状态已过期，请重新开始。" };
  }
  try {
    await verifyEmail({ userId: signupFlow.userId, verificationCode: code });
    const session = await getSession({
      sessionId: loginFlow.sessionId,
      sessionToken: loginFlow.sessionToken,
    });
    const validation = await validateSession(session);
    if (!validation.valid) {
      return {
        ...prev,
        error:
          validation.reason === "email-not-verified"
            ? "邮箱验证未完成，请重试。"
            : "注册尚未完成，请重新开始。",
      };
    }
    await clearSignupFlow();
    return await completeLoginFlow(loginFlow);
  } catch (err) {
    return errorState(prev, err, "邮箱验证失败，请重试。");
  }
}

export async function resendSignupEmailCode(
  prev: CustomSignupActionState,
  formData: FormData,
): Promise<CustomSignupActionState> {
  void formData;
  const signupFlow = await getSignupFlow();
  if (!signupFlow) {
    return { ...initialState, error: "注册状态已过期，请重新开始。" };
  }
  try {
    await resendEmailCode(signupFlow.userId);
    return { ...prev, step: "verify-email", email: signupFlow.email };
  } catch (err) {
    return errorState(prev, err, "验证码发送失败，请重试。");
  }
}
