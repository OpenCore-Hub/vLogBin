import { createHmac, timingSafeEqual } from "node:crypto";

export function verifyWebhookSignature(
  secret: string,
  timestamp: string,
  payload: string,
  signature: string,
): boolean {
  const expected = createHmac("sha256", secret)
    .update(timestamp + payload)
    .digest();
  const given = Buffer.from(signature, "hex");
  return given.length === expected.length && timingSafeEqual(given, expected);
}

export function verifyWebhookSignatureWithin(
  secret: string,
  timestamp: string,
  payload: string,
  signature: string,
  maxAgeMs = 5 * 60 * 1000,
): boolean {
  const seconds = Number(timestamp);
  if (!Number.isFinite(seconds)) return false;
  const ageMs = Date.now() - seconds * 1000;
  if (ageMs > maxAgeMs || ageMs < -maxAgeMs) return false;
  return verifyWebhookSignature(secret, timestamp, payload, signature);
}
