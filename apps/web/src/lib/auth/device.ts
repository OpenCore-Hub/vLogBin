import "server-only";

import { randomUUID } from "node:crypto";
import { cookies } from "next/headers";

export const DEVICE_COOKIE = "vlb_device";
export const DEVICE_COOKIE_MAX_AGE_SECONDS = 365 * 24 * 60 * 60;

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

/** 返回稳定的设备指纹；不存在时生成并写入 httpOnly cookie。 */
export async function getOrCreateDeviceFingerprint(): Promise<string> {
  const jar = await cookies();
  const existing = jar.get(DEVICE_COOKIE)?.value;
  if (existing && UUID_PATTERN.test(existing)) {
    return existing;
  }
  const fingerprint = randomUUID();
  jar.set(DEVICE_COOKIE, fingerprint, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "lax",
    path: "/",
    maxAge: DEVICE_COOKIE_MAX_AGE_SECONDS,
  });
  return fingerprint;
}
