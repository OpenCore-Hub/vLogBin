import "server-only";

import { cookies } from "next/headers";
import { z } from "zod";
import { isSessionSecretConfigured } from "./config";
import { SecretCryptoError, sealSecret, unsealSecret } from "./crypto";

export const SIGNUP_FLOW_COOKIE = "vlb_signup_flow";
export const SIGNUP_FLOW_MAX_AGE_SECONDS = 30 * 60;

export const signupFlowSchema = z.object({
  authRequestId: z.string().min(1).max(200),
  next: z.string().min(1).max(2000).optional(),
  userId: z.string().min(1).max(200),
  email: z.string().email().max(200),
  givenName: z.string().min(1).max(200),
  familyName: z.string().min(1).max(200),
  passkeyId: z.string().min(1).max(200).optional(),
});

export type SignupFlowData = z.infer<typeof signupFlowSchema>;

export class SignupFlowError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "SignupFlowError";
  }
}

export async function getSignupFlow(): Promise<SignupFlowData | null> {
  const jar = await cookies();
  const raw = jar.get(SIGNUP_FLOW_COOKIE)?.value;
  if (!raw) return null;
  try {
    const parsed = signupFlowSchema.safeParse(
      JSON.parse(unsealSecret(raw)) as unknown,
    );
    if (!parsed.success) {
      jar.delete(SIGNUP_FLOW_COOKIE);
      return null;
    }
    return parsed.data;
  } catch (err) {
    if (err instanceof SecretCryptoError) {
      jar.delete(SIGNUP_FLOW_COOKIE);
    }
    return null;
  }
}

export async function setSignupFlow(input: SignupFlowData): Promise<void> {
  if (!isSessionSecretConfigured()) {
    throw new SignupFlowError(
      "未配置 SESSION_SECRET（至少 32 字符），拒绝写入注册流程 cookie。",
    );
  }
  const data = signupFlowSchema.parse(input);
  const jar = await cookies();
  jar.set(SIGNUP_FLOW_COOKIE, sealSecret(JSON.stringify(data)), {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: SIGNUP_FLOW_MAX_AGE_SECONDS,
  });
}

export async function clearSignupFlow(): Promise<void> {
  const jar = await cookies();
  jar.delete(SIGNUP_FLOW_COOKIE);
}
