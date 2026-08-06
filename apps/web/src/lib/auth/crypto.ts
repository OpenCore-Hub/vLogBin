import "server-only";

import {
  createCipheriv,
  createDecipheriv,
  createHash,
  randomBytes,
} from "node:crypto";
import { authConfig } from "./config";

export class SecretCryptoError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "SecretCryptoError";
  }
}

function deriveKey(): Buffer {
  return createHash("sha256").update(authConfig.sessionSecret).digest();
}

/** AES-256-GCM 密封；返回 base64url(iv).base64url(tag).base64url(data)。 */
export function sealSecret(plain: string): string {
  const key = deriveKey();
  const iv = randomBytes(12);
  const cipher = createCipheriv("aes-256-gcm", key, iv);
  const data = Buffer.concat([cipher.update(plain, "utf8"), cipher.final()]);
  const tag = cipher.getAuthTag();
  return [iv, tag, data].map((b) => b.toString("base64url")).join(".");
}

export function unsealSecret(payload: string): string {
  const parts = payload.split(".");
  if (parts.length !== 3) throw new SecretCryptoError("加密数据损坏");
  const [ivS, tagS, dataS] = parts;
  try {
    const decipher = createDecipheriv(
      "aes-256-gcm",
      deriveKey(),
      Buffer.from(ivS, "base64url"),
    );
    decipher.setAuthTag(Buffer.from(tagS, "base64url"));
    return Buffer.concat([
      decipher.update(Buffer.from(dataS, "base64url")),
      decipher.final(),
    ]).toString("utf8");
  } catch {
    throw new SecretCryptoError("加密数据解密失败");
  }
}
