import "server-only";

import { cookies } from "next/headers";
import { z } from "zod";
import { authConfig, isSessionSecretConfigured } from "./config";
import { SecretCryptoError, sealSecret, unsealSecret } from "./crypto";

export const LOGIN_FLOW_COOKIE = "vlb_login_flow";
export const LOGIN_FLOW_MAX_AGE_SECONDS = 30 * 60;

export const loginFlowSchema = z.object({
  sessionId: z.string().min(1).max(200),
  sessionToken: z.string().min(1).max(500),
  loginName: z.string().min(1).max(200).optional(),
  userId: z.string().min(1).max(200).optional(),
  organizationId: z.string().min(1).max(200).optional(),
  authRequestId: z.string().min(1).max(200).optional(),
  next: z.string().min(1).max(2000).optional(),
  pendingMfaMethod: z.enum(["otpEmail", "otpSms"]).optional(),
});

export type LoginFlowData = z.infer<typeof loginFlowSchema>;

export class LoginFlowError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "LoginFlowError";
  }
}

function cookieOptions(maxAge: number) {
  return {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax" as const,
    path: "/",
    maxAge,
  };
}

function safeNext(value: string | undefined | null): string | undefined {
  if (!value) return undefined;
  return value.startsWith("/") && !value.startsWith("//") ? value : undefined;
}

export async function getLoginFlow(): Promise<LoginFlowData | null> {
  const jar = await cookies();
  const raw = jar.get(LOGIN_FLOW_COOKIE)?.value;
  if (!raw) return null;
  try {
    const parsed = loginFlowSchema.safeParse(
      JSON.parse(unsealSecret(raw)) as unknown,
    );
    if (!parsed.success) {
      jar.delete(LOGIN_FLOW_COOKIE);
      return null;
    }
    return parsed.data;
  } catch (err) {
    if (err instanceof SecretCryptoError) {
      jar.delete(LOGIN_FLOW_COOKIE);
    }
    return null;
  }
}

export async function setLoginFlow(
  input: Omit<LoginFlowData, "next"> & { next?: string },
): Promise<void> {
  if (!isSessionSecretConfigured()) {
    throw new LoginFlowError(
      "未配置 SESSION_SECRET（至少 32 字符），拒绝写入登录流程 cookie。",
    );
  }
  const data = loginFlowSchema.parse({
    ...input,
    next: safeNext(input.next),
  });
  const jar = await cookies();
  jar.set(
    LOGIN_FLOW_COOKIE,
    sealSecret(JSON.stringify(data)),
    cookieOptions(LOGIN_FLOW_MAX_AGE_SECONDS),
  );
}

export async function clearLoginFlow(): Promise<void> {
  const jar = await cookies();
  jar.delete(LOGIN_FLOW_COOKIE);
}

export function isLoginFlowConfigured(): boolean {
  return authConfig.mode === "oidc-custom-login";
}
