import "server-only";

import { authConfig } from "../auth/config";
import type {
  Customer,
  Invoice,
  PortalDashboard,
  PortalSessionInfo,
  Subscription,
  UsageEvent,
} from "./types";

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function asString(value: unknown, fallback = ""): string {
  return typeof value === "string" ? value : fallback;
}

function asOptionalString(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

function asCustomer(value: unknown): Customer {
  const rec = asRecord(value) ?? {};
  return {
    id: asString(rec.id),
    external_id: asString(rec.external_id),
    account_type: asString(rec.account_type, "unknown"),
    display_name: asString(rec.display_name),
    environment_id: asString(rec.environment_id),
    environment_kind: asString(rec.environment_kind, "unknown"),
    created_at: asOptionalString(rec.created_at),
  };
}

function asSubscription(value: unknown): Subscription {
  const rec = asRecord(value) ?? {};
  return {
    id: asString(rec.id),
    external_id: asString(rec.external_id),
    customer_external_id: asString(rec.customer_external_id),
    plan_code: asString(rec.plan_code),
    catalog_version_id: asString(rec.catalog_version_id),
    status: asString(rec.status, "unknown"),
    environment_kind: asString(rec.environment_kind, "unknown"),
    started_at: asOptionalString(rec.started_at),
    terminated_at: asOptionalString(rec.terminated_at),
  };
}

function asUsageEvent(value: unknown): UsageEvent {
  const rec = asRecord(value) ?? {};
  return {
    id: asString(rec.id),
    transaction_id: asString(rec.transaction_id),
    kind: asString(rec.kind, "unknown"),
    metric_code: asString(rec.metric_code),
    customer_external_id: asString(rec.customer_external_id),
    environment_id: asString(rec.environment_id),
    environment_kind: asString(rec.environment_kind, "unknown"),
    event_timestamp: asOptionalString(rec.event_timestamp),
    created_at: asOptionalString(rec.created_at),
  };
}

function asInvoice(value: unknown): Invoice {
  const rec = asRecord(value) ?? {};
  return {
    id: asString(rec.id),
    number: asString(rec.number),
    lago_id: asString(rec.lago_id),
    issuing_date: asString(rec.issuing_date),
    invoice_type: asString(rec.invoice_type, "unknown"),
    status: asString(rec.status, "unknown"),
    payment_status: asString(rec.payment_status, "unknown"),
    currency: asString(rec.currency),
    total_amount_cents:
      typeof rec.total_amount_cents === "number" ? rec.total_amount_cents : 0,
    customer_external_id: asString(rec.customer_external_id),
    environment_id: asString(rec.environment_id),
    environment_kind: asString(rec.environment_kind, "unknown"),
  };
}

async function request<T>(
  path: string,
  token: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(`${authConfig.apiBaseUrl}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
      ...init?.headers,
    },
    cache: "no-store",
  });
  if (!res.ok) {
    throw new Error(`Portal request failed with status ${res.status}`);
  }
  const text = await res.text();
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
}

/** 校验邀请链接中的 portal token（客户会话建立前）。 */
export async function validatePortalSession(token: string): Promise<PortalSessionInfo> {
  const data = await request<Partial<PortalSessionInfo>>(
    "/v1/portal/sessions",
    token,
    { method: "POST", body: JSON.stringify({ token }) },
  );
  return {
    valid: data.valid === true,
    provider_id: asString(data.provider_id),
    environment_id: asString(data.environment_id),
    environment_kind: asString(data.environment_kind),
    customer_external_id: asString(data.customer_external_id),
    expires_at: asOptionalString(data.expires_at),
  };
}

/** 读取客户门户 Dashboard（仅当前 token 的客户数据）。 */
export async function getPortalDashboard(token: string): Promise<PortalDashboard> {
  const data = await request<Record<string, unknown>>("/v1/portal/dashboard", token);
  const subscriptions = Array.isArray(data.subscriptions)
    ? data.subscriptions.map(asSubscription)
    : [];
  const usageEvents = Array.isArray(data.usage_events)
    ? data.usage_events.map(asUsageEvent)
    : [];
  const invoices = Array.isArray(data.invoices) ? data.invoices.map(asInvoice) : [];
  return {
    provider_name: asString(data.provider_name),
    provider_slug: asString(data.provider_slug),
    customer: asCustomer(data.customer),
    subscriptions,
    usage_events: usageEvents,
    invoices,
  };
}
