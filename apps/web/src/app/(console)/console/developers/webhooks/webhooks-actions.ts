"use server";

import { revalidatePath } from "next/cache";
import { requireAuth } from "@/lib/auth/rbac";
import type { Env } from "@/lib/env-shared";
import {
  createWebhook,
  deleteWebhook,
  replayWebhookDelivery,
  type WebhookDelivery,
  type WebhookEndpoint,
} from "@/lib/api/operator";

const WEBHOOKS_PATH = "/console/developers/webhooks";

export interface WebhookActionState {
  ok: boolean;
  error?: string;
  endpoint?: WebhookEndpoint;
  delivery?: WebhookDelivery;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return "发生未知错误，请稍后重试。";
}

function parseEnv(value: FormDataEntryValue | null): Env | null {
  return value === "test" || value === "live" ? value : null;
}

export async function createWebhookAction(
  _prev: WebhookActionState,
  formData: FormData,
): Promise<WebhookActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const url = String(formData.get("url") ?? "").trim();
  const events = formData
    .getAll("events")
    .map((v) => String(v).trim())
    .filter(Boolean);
  const secret = String(formData.get("secret") ?? "").trim();
  if (!providerId || !env) return { ok: false, error: "缺少必要参数" };
  if (!url) return { ok: false, error: "请输入回调 URL" };

  try {
    const endpoint = await createWebhook(providerId, env, {
      url,
      events,
      secret: secret || undefined,
    });
    if (!endpoint) return { ok: false, error: "创建失败：API 未返回端点" };
    revalidatePath(WEBHOOKS_PATH);
    return { ok: true, endpoint };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function deleteWebhookAction(
  _prev: WebhookActionState,
  formData: FormData,
): Promise<WebhookActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const env = parseEnv(formData.get("env"));
  const webhookId = String(formData.get("webhook_id") ?? "").trim();
  if (!providerId || !env || !webhookId) return { ok: false, error: "缺少必要参数" };

  try {
    await deleteWebhook(providerId, env, webhookId);
    revalidatePath(WEBHOOKS_PATH);
    return { ok: true };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function replayWebhookDeliveryAction(
  _prev: WebhookActionState,
  formData: FormData,
): Promise<WebhookActionState> {
  await requireAuth();
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const deliveryId = String(formData.get("delivery_id") ?? "").trim();
  if (!providerId || !deliveryId) return { ok: false, error: "缺少必要参数" };

  try {
    const delivery = await replayWebhookDelivery(providerId, deliveryId);
    if (!delivery) return { ok: false, error: "重放失败：API 未返回投递记录" };
    revalidatePath(WEBHOOKS_PATH);
    return { ok: true, delivery };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}
