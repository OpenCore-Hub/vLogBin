import "server-only";

import { cookies } from "next/headers";
import { z } from "zod";
import { isSessionSecretConfigured } from "./config";
import { SecretCryptoError, sealSecret, unsealSecret } from "./crypto";

export const ZITADEL_SESSIONS_COOKIE = "vlb_zitadel_sessions";
const MAX_REMEMBERED_SESSIONS = 5;

export const rememberedSessionSchema = z.object({
  sessionId: z.string().min(1).max(200),
  sessionToken: z.string().min(1).max(500),
  loginName: z.string().min(1).max(200),
  userId: z.string().min(1).max(200),
  organizationId: z.string().min(1).max(200).optional(),
});

export type RememberedSession = z.infer<typeof rememberedSessionSchema>;

export async function getRememberedSessions(): Promise<RememberedSession[]> {
  const jar = await cookies();
  const raw = jar.get(ZITADEL_SESSIONS_COOKIE)?.value;
  if (!raw) return [];
  try {
    const parsed = z
      .array(rememberedSessionSchema)
      .max(MAX_REMEMBERED_SESSIONS)
      .safeParse(JSON.parse(unsealSecret(raw)) as unknown);
    return parsed.success ? parsed.data : [];
  } catch (err) {
    if (err instanceof SecretCryptoError) {
      jar.delete(ZITADEL_SESSIONS_COOKIE);
    }
    return [];
  }
}

export async function rememberSession(
  input: RememberedSession,
): Promise<void> {
  if (!isSessionSecretConfigured()) {
    return;
  }
  const current = await getRememberedSessions();
  const next = [
    input,
    ...current.filter((session) => session.sessionId !== input.sessionId),
  ].slice(0, MAX_REMEMBERED_SESSIONS);
  const jar = await cookies();
  jar.set(
    ZITADEL_SESSIONS_COOKIE,
    sealSecret(JSON.stringify(next)),
    {
      httpOnly: true,
      secure: process.env.NODE_ENV === "production",
      sameSite: "lax",
      path: "/",
      maxAge: 365 * 24 * 60 * 60,
    },
  );
}

export async function forgetSession(sessionId: string): Promise<void> {
  const current = await getRememberedSessions();
  const next = current.filter((session) => session.sessionId !== sessionId);
  const jar = await cookies();
  if (next.length === 0) {
    jar.delete(ZITADEL_SESSIONS_COOKIE);
    return;
  }
  jar.set(
    ZITADEL_SESSIONS_COOKIE,
    sealSecret(JSON.stringify(next)),
    {
      httpOnly: true,
      secure: process.env.NODE_ENV === "production",
      sameSite: "lax",
      path: "/",
      maxAge: 365 * 24 * 60 * 60,
    },
  );
}
