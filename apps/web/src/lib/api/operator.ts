/**
 * Server-side client for the platform operator API.
 *
 * 凭据来源（双模式，见 lib/auth/config.ts）：
 * - oidc：会话中的 ZITADEL access token（过期自动静默刷新）
 * - operator-token：登录时存入会话的 OPERATOR_TOKEN
 * 令牌永不下发浏览器；本模块只能从 Server Components / Actions / Route
 * Handlers 导入。
 */

import { after } from "next/server";
import {
  getSession,
  isAccessTokenExpiring,
  refreshAccessToken,
  updateSession,
} from "../auth/session";
import { authConfig } from "../auth/config";

import type {
  Provider,
  Environment,
  Region,
  CatalogVersion,
  Subscription,
  Customer,
  CustomerCreateInput,
  CustomerDetail,
  UsageEvent,
  AuditEvent,
  Credential,
  Invoice,
  Metric,
  Plan,
  PlanDetail,
  PlanCollection,
  PlanInput,
  Price,
  EntitlementGrant,
  PriceInput,
  EntitlementInput,
  CatalogVersionMeta,
  CatalogVersionDetail,
  LifecycleTarget,
  CreateProviderResult,
  LifecycleResult,
  Workspace,
  WorkspaceMembership,
  SignupResult,
  OverviewStats,
  OverviewTrends,
  TrendPoint,
  Capability,
  WebhookEndpoint,
  WebhookDelivery,
  HostedAuthConfig,
  HostedAuthCreateResult,
  CreateProviderInput,
  SignupInput,
} from "./types";

export type {
  Provider,
  Environment,
  Region,
  CatalogVersion,
  Subscription,
  Customer,
  CustomerCreateInput,
  CustomerDetail,
  UsageEvent,
  AuditEvent,
  Credential,
  Invoice,
  Metric,
  Plan,
  PlanDetail,
  PlanCollection,
  PlanInput,
  Price,
  EntitlementGrant,
  PriceInput,
  EntitlementInput,
  CatalogVersionMeta,
  CatalogVersionDetail,
  LifecycleTarget,
  CreateProviderResult,
  LifecycleResult,
  Workspace,
  WorkspaceMembership,
  SignupResult,
  OverviewStats,
  OverviewTrends,
  TrendPoint,
  Capability,
  WebhookEndpoint,
  WebhookDelivery,
  HostedAuthConfig,
  HostedAuthCreateResult,
  CreateProviderInput,
  SignupInput,
};

export class ApiError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(code: string, message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
  }
}

function baseUrl(): string {
  return authConfig.apiBaseUrl;
}

/** 从会话解析 API 凭据；access token 过期时静默刷新（RSC 用 after 延后提交）。 */
async function resolveApiToken(): Promise<string | null> {
  const session = await getSession();
  if (!session) return null;
  if (!session.accessToken) return null;
  if (isAccessTokenExpiring(session)) {
    try {
      const updated = await refreshAccessToken(session);
      if (updated.accessToken) {
        after(async () => {
          await updateSession({
            accessToken: updated.accessToken,
            refreshToken: updated.refreshToken,
            tokenExp: updated.tokenExp,
            email: updated.email,
            name: updated.name,
          });
        });
        return updated.accessToken;
      }
    } catch {
      // 刷新失败：退回旧 token，API 会以 401 拒绝，页面据此提示重新登录
    }
  }
  return session.accessToken;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = await resolveApiToken();
  if (!token) {
    throw new ApiError(
      "unauthenticated",
      "未登录或会话缺少访问凭据，请重新登录。",
      401,
    );
  }

  let res: Response;
  try {
    res = await fetch(`${baseUrl()}${path}`, {
      ...init,
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${token}`,
        ...init?.headers,
      },
      cache: "no-store",
    });
  } catch (err) {
    throw new ApiError(
      "api_unreachable",
      `Platform API unreachable: ${err instanceof Error ? err.message : String(err)}`,
      0,
    );
  }

  if (!res.ok) {
    let code = "api_error";
    let message = `Request failed with status ${res.status}`;
    try {
      const body: unknown = await res.json();
      if (body && typeof body === "object" && "error" in body) {
        const errObj = (body as { error?: { code?: unknown; message?: unknown } })
          .error;
        if (typeof errObj?.code === "string") code = errObj.code;
        if (typeof errObj?.message === "string") message = errObj.message;
      }
    } catch {
      // Non-JSON error body — keep the status-based defaults.
    }
    throw new ApiError(code, message, res.status);
  }

  // 204 No Content（DELETE）没有 body；统一走 text + 非空解析，避免
  // res.json() 对空响应抛 "Unexpected end of JSON input"。
  const text = await res.text();
  if (!text) return undefined as T;
  return JSON.parse(text) as T;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function asWorkspace(value: unknown): Workspace | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    slug: typeof rec.slug === "string" ? rec.slug : "",
    name: typeof rec.name === "string" ? rec.name : "",
    created_by: typeof rec.created_by === "string" ? rec.created_by : "",
    created_at: typeof rec.created_at === "string" ? rec.created_at : undefined,
    updated_at: typeof rec.updated_at === "string" ? rec.updated_at : undefined,
  };
}

function asWorkspaceMembership(value: unknown): WorkspaceMembership | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    workspace_id: typeof rec.workspace_id === "string" ? rec.workspace_id : "",
    user_sub: typeof rec.user_sub === "string" ? rec.user_sub : "",
    role: typeof rec.role === "string" ? rec.role : "",
    status: typeof rec.status === "string" ? rec.status : "",
    created_at: typeof rec.created_at === "string" ? rec.created_at : undefined,
    updated_at: typeof rec.updated_at === "string" ? rec.updated_at : undefined,
  };
}

/**
 * 注册供给（R11）：为平台用户创建默认 workspace 并授予 provider_admin。
 * 回调阶段会话尚未建立，因此显式携带 ZITADEL access token。
 * 幂等：老用户直接返回已有 workspace。
 */
export async function provisionWorkspace(
  accessToken: string,
  input: SignupInput,
): Promise<SignupResult> {
  let res: Response;
  try {
    res = await fetch(`${baseUrl()}/v1/signup`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${accessToken}`,
      },
      body: JSON.stringify(input),
      cache: "no-store",
    });
  } catch (err) {
    throw new ApiError(
      "api_unreachable",
      `Platform API unreachable: ${err instanceof Error ? err.message : String(err)}`,
      0,
    );
  }

  if (!res.ok) {
    let code = "api_error";
    let message = `Request failed with status ${res.status}`;
    try {
      const body: unknown = await res.json();
      if (body && typeof body === "object" && "error" in body) {
        const errObj = (body as { error?: { code?: unknown; message?: unknown } })
          .error;
        if (typeof errObj?.code === "string") code = errObj.code;
        if (typeof errObj?.message === "string") message = errObj.message;
      }
    } catch {
      // Non-JSON error body — keep the status-based defaults.
    }
    throw new ApiError(code, message, res.status);
  }

  const data = (await res.json()) as Record<string, unknown>;
  return {
    workspace: asWorkspace(data.workspace),
    membership: asWorkspaceMembership(data.membership),
    provider: asProvider(data.provider),
  };
}

/** 列出当前会话用户所属的 workspaces（console 选择工作区用）。 */
export async function listMyWorkspaces(): Promise<Workspace[]> {
  const data = await request<{ workspaces?: unknown }>("/v1/me/workspaces");
  if (!Array.isArray(data.workspaces)) return [];
  return data.workspaces
    .map(asWorkspace)
    .filter((w): w is Workspace => w !== null);
}

function asProvider(value: unknown): Provider | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    slug: typeof rec.slug === "string" ? rec.slug : "",
    name: typeof rec.name === "string" ? rec.name : "",
    lifecycle_state:
      typeof rec.lifecycle_state === "string" ? rec.lifecycle_state : "UNKNOWN",
    sla_tier: typeof rec.sla_tier === "string" ? rec.sla_tier : undefined,
    home_region_id:
      typeof rec.home_region_id === "string" ? rec.home_region_id : undefined,
    cell_id: typeof rec.cell_id === "string" ? rec.cell_id : undefined,
    created_at:
      typeof rec.created_at === "string" ? rec.created_at : undefined,
    updated_at:
      typeof rec.updated_at === "string" ? rec.updated_at : undefined,
  };
}

function asEnvironment(value: unknown): Environment | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    provider_id: typeof rec.provider_id === "string" ? rec.provider_id : "",
    kind: typeof rec.kind === "string" ? rec.kind : "unknown",
    status: typeof rec.status === "string" ? rec.status : "unknown",
    issuer: typeof rec.issuer === "string" ? rec.issuer : undefined,
    created_at:
      typeof rec.created_at === "string" ? rec.created_at : undefined,
  };
}

function asNumber(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function asCatalogVersion(value: unknown): CatalogVersion | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    version: asNumber(rec.version),
    state: typeof rec.state === "string" ? rec.state : "unknown",
    environment_id:
      typeof rec.environment_id === "string" ? rec.environment_id : "",
    environment_kind:
      typeof rec.environment_kind === "string" ? rec.environment_kind : "unknown",
    metrics_count: asNumber(rec.metrics_count),
    plans_count: asNumber(rec.plans_count),
    created_at:
      typeof rec.created_at === "string" ? rec.created_at : undefined,
    published_at:
      typeof rec.published_at === "string" ? rec.published_at : undefined,
  };
}

function asSubscription(value: unknown): Subscription | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    external_id: typeof rec.external_id === "string" ? rec.external_id : "",
    customer_external_id:
      typeof rec.customer_external_id === "string"
        ? rec.customer_external_id
        : "",
    plan_code: typeof rec.plan_code === "string" ? rec.plan_code : "",
    catalog_version_id:
      typeof rec.catalog_version_id === "string" ? rec.catalog_version_id : "",
    status: typeof rec.status === "string" ? rec.status : "unknown",
    environment_kind:
      typeof rec.environment_kind === "string" ? rec.environment_kind : "unknown",
    started_at:
      typeof rec.started_at === "string" ? rec.started_at : undefined,
    terminated_at:
      typeof rec.terminated_at === "string" ? rec.terminated_at : undefined,
  };
}

function asCustomer(value: unknown): Customer | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    external_id:
      typeof rec.external_id === "string" ? rec.external_id : "",
    account_type:
      typeof rec.account_type === "string" ? rec.account_type : "unknown",
    display_name:
      typeof rec.display_name === "string" ? rec.display_name : "",
    environment_id:
      typeof rec.environment_id === "string" ? rec.environment_id : "",
    environment_kind:
      typeof rec.environment_kind === "string"
        ? rec.environment_kind
        : "unknown",
    created_at:
      typeof rec.created_at === "string" ? rec.created_at : undefined,
  };
}

function asCustomerDetail(value: unknown): CustomerDetail | null {
  const rec = asRecord(value);
  if (!rec) return null;
  const customer = asCustomer(rec.customer);
  if (!customer) return null;
  const subscriptions = Array.isArray(rec.subscriptions)
    ? rec.subscriptions
        .map(asSubscription)
        .filter((s): s is Subscription => s !== null)
    : [];
  const usage_events = Array.isArray(rec.usage_events)
    ? rec.usage_events
        .map(asUsageEvent)
        .filter((e): e is UsageEvent => e !== null)
    : [];
  const invoices = Array.isArray(rec.invoices)
    ? rec.invoices.map(asInvoice).filter((i): i is Invoice => i !== null)
    : [];
  return { customer, subscriptions, usage_events, invoices };
}

function asUsageEvent(value: unknown): UsageEvent | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    transaction_id:
      typeof rec.transaction_id === "string" ? rec.transaction_id : "",
    kind: typeof rec.kind === "string" ? rec.kind : "unknown",
    metric_code:
      typeof rec.metric_code === "string" ? rec.metric_code : "",
    customer_external_id:
      typeof rec.customer_external_id === "string"
        ? rec.customer_external_id
        : "",
    environment_id:
      typeof rec.environment_id === "string" ? rec.environment_id : "",
    environment_kind:
      typeof rec.environment_kind === "string"
        ? rec.environment_kind
        : "unknown",
    event_timestamp:
      typeof rec.event_timestamp === "string"
        ? rec.event_timestamp
        : undefined,
    created_at:
      typeof rec.created_at === "string" ? rec.created_at : undefined,
  };
}

function asAuditEvent(value: unknown): AuditEvent | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "number" ? rec.id : Number(rec.id ?? 0),
    provider_id:
      typeof rec.provider_id === "string" ? rec.provider_id : undefined,
    environment_id:
      typeof rec.environment_id === "string" ? rec.environment_id : undefined,
    actor_type: typeof rec.actor_type === "string" ? rec.actor_type : "unknown",
    actor_id: typeof rec.actor_id === "string" ? rec.actor_id : "",
    action: typeof rec.action === "string" ? rec.action : "",
    target_type:
      typeof rec.target_type === "string" ? rec.target_type : undefined,
    target_id: typeof rec.target_id === "string" ? rec.target_id : undefined,
    metadata: rec.metadata,
    request_id:
      typeof rec.request_id === "string" ? rec.request_id : undefined,
    created_at:
      typeof rec.created_at === "string" ? rec.created_at : undefined,
  };
}

function asCredential(value: unknown): Credential | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    name: typeof rec.name === "string" ? rec.name : "",
    key_prefix: typeof rec.key_prefix === "string" ? rec.key_prefix : "",
    scopes: asStringArray(rec.scopes),
    allowed_cidrs: asStringArray(rec.allowed_cidrs),
    environment_id:
      typeof rec.environment_id === "string" ? rec.environment_id : "",
    environment_kind:
      typeof rec.environment_kind === "string" ? rec.environment_kind : "unknown",
    environment_issuer:
      typeof rec.environment_issuer === "string" ? rec.environment_issuer : "",
    expires_at:
      typeof rec.expires_at === "string" ? rec.expires_at : undefined,
    revoked_at:
      typeof rec.revoked_at === "string" ? rec.revoked_at : undefined,
    last_used_at:
      typeof rec.last_used_at === "string" ? rec.last_used_at : undefined,
    created_at:
      typeof rec.created_at === "string" ? rec.created_at : undefined,
  };
}

function asInvoice(value: unknown): Invoice | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    number: typeof rec.number === "string" ? rec.number : "",
    lago_id: typeof rec.lago_id === "string" ? rec.lago_id : "",
    issuing_date: typeof rec.issuing_date === "string" ? rec.issuing_date : "",
    invoice_type:
      typeof rec.invoice_type === "string" ? rec.invoice_type : "unknown",
    status: typeof rec.status === "string" ? rec.status : "unknown",
    payment_status:
      typeof rec.payment_status === "string" ? rec.payment_status : "unknown",
    currency: typeof rec.currency === "string" ? rec.currency : "",
    total_amount_cents: asNumber(rec.total_amount_cents),
    customer_external_id:
      typeof rec.customer_external_id === "string"
        ? rec.customer_external_id
        : "",
    customer_account_id:
      typeof rec.customer_account_id === "string"
        ? rec.customer_account_id
        : undefined,
    subscription_id:
      typeof rec.subscription_id === "string"
        ? rec.subscription_id
        : undefined,
    catalog_version_id:
      typeof rec.catalog_version_id === "string"
        ? rec.catalog_version_id
        : undefined,
    environment_id:
      typeof rec.environment_id === "string" ? rec.environment_id : "",
    environment_kind:
      typeof rec.environment_kind === "string"
        ? rec.environment_kind
        : "unknown",
  };
}

function asMetric(value: unknown): Metric | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    code: typeof rec.code === "string" ? rec.code : "",
    name: typeof rec.name === "string" ? rec.name : "",
    aggregation_type:
      typeof rec.aggregation_type === "string"
        ? rec.aggregation_type
        : "unknown",
    field_name:
      typeof rec.field_name === "string" ? rec.field_name : undefined,
    billable: typeof rec.billable === "boolean" ? rec.billable : false,
  };
}

function asPlan(value: unknown): Plan | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    code: typeof rec.code === "string" ? rec.code : "",
    name: typeof rec.name === "string" ? rec.name : "",
    interval: typeof rec.interval === "string" ? rec.interval : "unknown",
    currency: typeof rec.currency === "string" ? rec.currency : "",
  };
}

function asPrice(value: unknown): Price | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    charge_model:
      typeof rec.charge_model === "string" ? rec.charge_model : "unknown",
    metric_code:
      typeof rec.metric_code === "string" ? rec.metric_code : undefined,
    properties: "properties" in rec ? rec.properties : undefined,
  };
}

function asEntitlementGrant(value: unknown): EntitlementGrant | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    key: typeof rec.key === "string" ? rec.key : "",
    value_type:
      typeof rec.value_type === "string" ? rec.value_type : "unknown",
    value: "value" in rec ? rec.value : undefined,
  };
}

function asCatalogVersionMeta(value: unknown): CatalogVersionMeta | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    provider_id:
      typeof rec.provider_id === "string" ? rec.provider_id : "",
    environment_id:
      typeof rec.environment_id === "string" ? rec.environment_id : "",
    version: asNumber(rec.version),
    state: typeof rec.state === "string" ? rec.state : "unknown",
    created_at:
      typeof rec.created_at === "string" ? rec.created_at : undefined,
    validated_at:
      typeof rec.validated_at === "string" ? rec.validated_at : undefined,
    published_at:
      typeof rec.published_at === "string" ? rec.published_at : undefined,
    retired_at:
      typeof rec.retired_at === "string" ? rec.retired_at : undefined,
  };
}

function asCatalogVersionDetail(value: unknown): CatalogVersionDetail | null {
  const rec = asRecord(value);
  if (!rec) return null;
  const version = asCatalogVersionMeta(rec.version);
  if (!version) return null;
  const metrics = Array.isArray(rec.metrics)
    ? rec.metrics
        .map(asMetric)
        .filter((m): m is Metric => m !== null)
    : [];
  const plans = Array.isArray(rec.plans)
    ? rec.plans
        .map(asPlan)
        .filter((p): p is Plan => p !== null)
    : [];
  const prices = Array.isArray(rec.prices)
    ? rec.prices
        .map(asPrice)
        .filter((p): p is Price => p !== null)
    : [];
  const entitlement_grants = Array.isArray(rec.entitlement_grants)
    ? rec.entitlement_grants
        .map(asEntitlementGrant)
        .filter((g): g is EntitlementGrant => g !== null)
    : [];
  return { version, metrics, plans, prices, entitlement_grants };
}

function asPlanDetail(value: unknown): PlanDetail | null {
  const rec = asRecord(value);
  if (!rec) return null;
  const plan = asPlan(rec.plan);
  if (!plan) return null;
  const prices = Array.isArray(rec.prices)
    ? rec.prices.map(asPrice).filter((p): p is Price => p !== null)
    : [];
  const entitlement_grants = Array.isArray(rec.entitlement_grants)
    ? rec.entitlement_grants
        .map(asEntitlementGrant)
        .filter((g): g is EntitlementGrant => g !== null)
    : [];
  return { plan, prices, entitlement_grants };
}

function asPlanCollection(value: unknown): PlanCollection | null {
  const rec = asRecord(value);
  if (!rec) return null;
  const plans = Array.isArray(rec.plans)
    ? rec.plans.map(asPlanDetail).filter((p): p is PlanDetail => p !== null)
    : [];
  const metrics = Array.isArray(rec.metrics)
    ? rec.metrics.map(asMetric).filter((m): m is Metric => m !== null)
    : [];
  return { plans, metrics };
}

/** The API may return the plaintext key as a string or as an object. */
export function extractApiKey(value: unknown): string | null {
  if (typeof value === "string" && value.length > 0) return value;
  const rec = asRecord(value);
  if (!rec) return null;
  for (const field of ["key", "secret", "token", "value", "plaintext"]) {
    const v = rec[field];
    if (typeof v === "string" && v.length > 0) return v;
  }
  return null;
}

export async function listProviders(): Promise<Provider[]> {
  const data = await request<{ providers?: unknown }>("/v1/operator/providers");
  if (!Array.isArray(data.providers)) return [];
  return data.providers
    .map(asProvider)
    .filter((p): p is Provider => p !== null);
}

function asTrendPoint(value: unknown): TrendPoint | null {
  if (!value || typeof value !== "object") return null;
  const rec = value as Record<string, unknown>;
  const date = typeof rec.date === "string" ? rec.date : "";
  const num = Number(rec.value);
  if (!date || Number.isNaN(num)) return null;
  return { date, value: num };
}

function asTrendSeries(value: unknown): TrendPoint[] {
  return Array.isArray(value)
    ? value
        .map(asTrendPoint)
        .filter((p): p is TrendPoint => p !== null)
    : [];
}

/** 概览聚合统计：单请求跨所有 provider 聚合（R29，替代 web 端 N+1 扇出）。 */
export async function getOverviewStats(): Promise<OverviewStats> {
  const data = await request<Partial<OverviewStats> | null>(
    "/v1/operator/overview-stats",
  );
  const trends = (data?.trends ?? {}) as Partial<OverviewTrends>;
  return {
    published_versions: Number(data?.published_versions) || 0,
    active_subscriptions: Number(data?.active_subscriptions) || 0,
    customers: Number(data?.customers) || 0,
    revenue_cents: Number(data?.revenue_cents) || 0,
    trends: {
      revenue: asTrendSeries(trends.revenue),
      usage_events: asTrendSeries(trends.usage_events),
    },
  };
}

export async function getProvider(
  id: string,
): Promise<{ provider: Provider | null; environments: Environment[] }> {
  const data = await request<{ provider?: unknown; environments?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(id)}`,
  );
  const environments = Array.isArray(data.environments)
    ? data.environments
        .map(asEnvironment)
        .filter((e): e is Environment => e !== null)
    : [];
  return { provider: asProvider(data.provider), environments };
}

export async function listCatalogVersions(
  providerId: string,
): Promise<CatalogVersion[]> {
  const data = await request<{ versions?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/catalog/versions`,
  );
  if (!Array.isArray(data.versions)) return [];
  return data.versions
    .map(asCatalogVersion)
    .filter((v): v is CatalogVersion => v !== null);
}

/** 列出指定环境当前目录版本的套餐与可用指标（Console Plans 页单请求全量）。 */
export async function listCatalogPlans(
  providerId: string,
  env: "test" | "live",
): Promise<PlanCollection> {
  const data = await request<{ plans?: unknown; metrics?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/catalog/plans?env=${env}`,
  );
  return asPlanCollection(data) ?? { plans: [], metrics: [] };
}

/** 获取单个套餐详情（prices 的 metric_code 已解析）。 */
export async function getCatalogPlan(
  providerId: string,
  env: "test" | "live",
  code: string,
): Promise<PlanDetail | null> {
  const data = await request<{ plan?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/catalog/plans/${encodeURIComponent(code)}?env=${env}`,
  );
  return asPlanDetail(data.plan);
}

/** 创建套餐（写入当前 draft 版本；无 draft 时自动克隆最新 published）。 */
export async function createCatalogPlan(
  providerId: string,
  env: "test" | "live",
  input: PlanInput,
): Promise<PlanDetail | null> {
  const data = await request<{ plan?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/catalog/plans?env=${env}`,
    { method: "POST", body: JSON.stringify(input) },
  );
  return asPlanDetail(data.plan);
}

/** 更新套餐（保留 plan id；已发布内容不可变，自动 staged 到新 draft）。 */
export async function updateCatalogPlan(
  providerId: string,
  env: "test" | "live",
  code: string,
  input: PlanInput,
): Promise<PlanDetail | null> {
  const data = await request<{ plan?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/catalog/plans/${encodeURIComponent(code)}?env=${env}`,
    { method: "PUT", body: JSON.stringify(input) },
  );
  return asPlanDetail(data.plan);
}

/** 删除套餐（仅从当前 draft 版本移除；published 内容不受影响）。 */
export async function deleteCatalogPlan(
  providerId: string,
  env: "test" | "live",
  code: string,
): Promise<void> {
  await request<unknown>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/catalog/plans/${encodeURIComponent(code)}?env=${env}`,
    { method: "DELETE" },
  );
}

export async function listSubscriptions(
  providerId: string,
): Promise<Subscription[]> {
  const data = await request<{ subscriptions?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/subscriptions`,
  );
  if (!Array.isArray(data.subscriptions)) return [];
  return data.subscriptions
    .map(asSubscription)
    .filter((s): s is Subscription => s !== null);
}

export async function getCatalogVersionDetail(
  providerId: string,
  versionId: string,
): Promise<CatalogVersionDetail | null> {
  const data = await request<{
    version?: unknown;
    metrics?: unknown;
    plans?: unknown;
    prices?: unknown;
    entitlement_grants?: unknown;
  }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/catalog/versions/${encodeURIComponent(versionId)}`,
  );
  return asCatalogVersionDetail(data);
}

export async function listCustomers(
  providerId: string,
  env?: "test" | "live",
): Promise<Customer[]> {
  const qs = env ? `?env=${env}` : "";
  const data = await request<{ customers?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/customers${qs}`,
  );
  if (!Array.isArray(data.customers)) return [];
  return data.customers
    .map(asCustomer)
    .filter((c): c is Customer => c !== null);
}

/** 在指定环境创建客户（external_id 唯一）。 */
export async function createCustomer(
  providerId: string,
  env: "test" | "live",
  input: CustomerCreateInput,
): Promise<Customer | null> {
  const data = await request<{ customer?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/customers?env=${env}`,
    { method: "POST", body: JSON.stringify(input) },
  );
  return asCustomer(data.customer);
}

/** 客户详情：一次请求返回客户 + 订阅 / 用量 / 账单（同环境）。 */
export async function getCustomerDetail(
  providerId: string,
  env: "test" | "live",
  externalId: string,
): Promise<CustomerDetail | null> {
  const data = await request<Partial<CustomerDetail>>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/customers/${encodeURIComponent(externalId)}?env=${env}`,
  );
  return asCustomerDetail(data);
}

export async function listUsageEvents(
  providerId: string,
): Promise<UsageEvent[]> {
  const data = await request<{ usage_events?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/usage-events`,
  );
  if (!Array.isArray(data.usage_events)) return [];
  return data.usage_events
    .map(asUsageEvent)
    .filter((e): e is UsageEvent => e !== null);
}

export async function listAuditEvents(providerId: string): Promise<AuditEvent[]> {
  const data = await request<{ audit_events?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/audit`,
  );
  if (!Array.isArray(data.audit_events)) return [];
  return data.audit_events
    .map(asAuditEvent)
    .filter((e): e is AuditEvent => e !== null);
}

/**
 * 列出 Provider 在所有环境（test/live）下签发的 API 密钥（operator 视图）。
 * key_hash 永远不会下发给客户端，仅通过 key_prefix 标识密钥。
 */
export async function listCredentials(providerId: string): Promise<Credential[]> {
  const data = await request<{ credentials?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/credentials`,
  );
  if (!Array.isArray(data.credentials)) return [];
  return data.credentials
    .map(asCredential)
    .filter((c): c is Credential => c !== null);
}

/**
 * 吊销 Provider 的 API 密钥，即时生效：认证中间件会在下一次请求拒绝该密钥。
 * revoked_by 记录在 Provider 的审计轨迹上（默认 "operator"）。
 */
export async function revokeCredential(
  providerId: string,
  credentialId: string,
  revokedBy = "operator",
): Promise<Credential | null> {
  const data = await request<{ credential?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/credentials/${encodeURIComponent(credentialId)}/revoke`,
    { method: "POST", body: JSON.stringify({ revoked_by: revokedBy }) },
  );
  return asCredential(data.credential);
}

export async function listInvoices(
  providerId: string,
): Promise<Invoice[]> {
  const data = await request<{ invoices?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/invoices`,
  );
  if (!Array.isArray(data.invoices)) return [];
  return data.invoices
    .map(asInvoice)
    .filter((i): i is Invoice => i !== null);
}

function asCapability(value: unknown): Capability | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    provider_id: typeof rec.provider_id === "string" ? rec.provider_id : "",
    capability: typeof rec.capability === "string" ? rec.capability : "",
    status: typeof rec.status === "string" ? rec.status : "pending",
    granted_at: typeof rec.granted_at === "string" ? rec.granted_at : undefined,
    revoked_at: typeof rec.revoked_at === "string" ? rec.revoked_at : undefined,
    granted_by: typeof rec.granted_by === "string" ? rec.granted_by : undefined,
    reason: typeof rec.reason === "string" ? rec.reason : undefined,
  };
}

export async function listCapabilities(
  providerId: string,
): Promise<Capability[]> {
  const data = await request<{ capabilities?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/capabilities`,
  );
  if (!Array.isArray(data.capabilities)) return [];
  return data.capabilities
    .map(asCapability)
    .filter((c): c is Capability => c !== null);
}

function asStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((v): v is string => typeof v === "string") : [];
}

function asWebhookEndpoint(value: unknown): WebhookEndpoint | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    provider_id: typeof rec.provider_id === "string" ? rec.provider_id : "",
    environment_id: typeof rec.environment_id === "string" ? rec.environment_id : "",
    url: typeof rec.url === "string" ? rec.url : "",
    secret: typeof rec.secret === "string" ? rec.secret : "",
    enabled: typeof rec.enabled === "boolean" ? rec.enabled : false,
    events: asStringArray(rec.events),
    created_at: typeof rec.created_at === "string" ? rec.created_at : undefined,
  };
}

function asWebhookDelivery(value: unknown): WebhookDelivery | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    endpoint_id: typeof rec.endpoint_id === "string" ? rec.endpoint_id : "",
    outbox_event_id: typeof rec.outbox_event_id === "string" ? rec.outbox_event_id : "",
    status: typeof rec.status === "string" ? rec.status : "pending",
    attempts: typeof rec.attempts === "number" ? rec.attempts : 0,
    response_status: typeof rec.response_status === "number" ? rec.response_status : undefined,
    response_body: typeof rec.response_body === "string" ? rec.response_body : undefined,
    delivered_at: typeof rec.delivered_at === "string" ? rec.delivered_at : undefined,
    created_at: typeof rec.created_at === "string" ? rec.created_at : undefined,
  };
}

export async function listWebhooks(providerId: string): Promise<WebhookEndpoint[]> {
  const data = await request<{ endpoints?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/webhooks`,
  );
  if (!Array.isArray(data.endpoints)) return [];
  return data.endpoints
    .map(asWebhookEndpoint)
    .filter((e): e is WebhookEndpoint => e !== null);
}

export async function listWebhookDeliveries(providerId: string): Promise<WebhookDelivery[]> {
  const data = await request<{ deliveries?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/webhook-deliveries`,
  );
  if (!Array.isArray(data.deliveries)) return [];
  return data.deliveries
    .map(asWebhookDelivery)
    .filter((d): d is WebhookDelivery => d !== null);
}

/** 重放终态（dead_letter / failed）的 webhook 投递，立即重新入队投递。 */
export async function replayWebhookDelivery(
  providerId: string,
  deliveryId: string,
  actor?: string,
): Promise<WebhookDelivery | null> {
  const data = await request<{ delivery?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/webhook-deliveries/${encodeURIComponent(deliveryId)}/replay`,
    {
      method: "POST",
      body: JSON.stringify(actor ? { actor } : {}),
    },
  );
  return asWebhookDelivery(data.delivery);
}

export async function listRegions(): Promise<Region[]> {
  const data = await request<{ regions?: unknown }>("/v1/operator/regions");
  if (!Array.isArray(data.regions)) return [];
  return data.regions
    .map(asRecord)
    .filter((r): r is Record<string, unknown> => r !== null)
    .map((r) => ({
      id: typeof r.id === "string" ? r.id : "",
      code: typeof r.code === "string" ? r.code : "",
      jurisdiction: typeof r.jurisdiction === "string" ? r.jurisdiction : "",
    }));
}

function asHostedAuthConfig(value: unknown): HostedAuthConfig | null {
  const rec = asRecord(value);
  if (!rec) return null;
  return {
    id: typeof rec.id === "string" ? rec.id : "",
    name: typeof rec.name === "string" ? rec.name : "",
    client_id: typeof rec.client_id === "string" ? rec.client_id : "",
    enabled: typeof rec.enabled === "boolean" ? rec.enabled : false,
    redirect_uris: asStringArray(rec.redirect_uris),
    created_at:
      typeof rec.created_at === "string" ? rec.created_at : undefined,
    updated_at:
      typeof rec.updated_at === "string" ? rec.updated_at : undefined,
  };
}

function asHostedAuthCreateResult(value: unknown): HostedAuthCreateResult | null {
  const base = asHostedAuthConfig(value);
  if (!base) return null;
  const rec = asRecord(value);
  const result: HostedAuthCreateResult = { ...base };
  if (rec && typeof rec.client_secret === "string" && rec.client_secret) {
    result.client_secret = rec.client_secret;
  }
  if (rec && typeof rec.issuer_url === "string" && rec.issuer_url) {
    result.issuer_url = rec.issuer_url;
  }
  return result;
}

/** 列出 workspace 指定环境下的 OIDC 应用（operator 控制面）。 */
export async function listHostedAuthApps(
  providerId: string,
  env: "test" | "live",
): Promise<HostedAuthConfig[]> {
  const data = await request<{ apps?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/auth/zitadel/apps?env=${env}`,
  );
  if (!Array.isArray(data.apps)) return [];
  return data.apps
    .map(asHostedAuthConfig)
    .filter((a): a is HostedAuthConfig => a !== null);
}

/** 创建 OIDC 应用（operator 控制面；client_id 立即返回）。 */
export async function createHostedAuthApp(
  providerId: string,
  env: "test" | "live",
  input: { name: string; redirect_uris: string[] },
): Promise<HostedAuthCreateResult | null> {
  const data = await request<{ app?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/auth/zitadel/setup?env=${env}`,
    { method: "POST", body: JSON.stringify(input) },
  );
  return asHostedAuthCreateResult(data.app);
}

/** 轮换 OIDC 应用客户端密钥（明文只返回一次，R17）。 */
export async function rotateHostedAuthSecret(
  providerId: string,
  env: "test" | "live",
): Promise<HostedAuthCreateResult | null> {
  const data = await request<{ app?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/auth/zitadel/rotate-secret?env=${env}`,
    { method: "POST" },
  );
  return asHostedAuthCreateResult(data.app);
}

/** 更新 OIDC 应用回调地址（不更换客户端密钥）。 */
export async function updateHostedAuthRedirectURIs(
  providerId: string,
  env: "test" | "live",
  redirectUris: string[],
): Promise<HostedAuthConfig | null> {
  const data = await request<{ app?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/auth/zitadel/redirect-uris?env=${env}`,
    { method: "PUT", body: JSON.stringify({ redirect_uris: redirectUris }) },
  );
  return asHostedAuthConfig(data.app);
}

/** 删除 OIDC 应用（ZITADEL 项目一并移除）。 */
export async function disableHostedAuth(
  providerId: string,
  env: "test" | "live",
): Promise<void> {
  await request<unknown>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/auth/zitadel?env=${env}`,
    { method: "DELETE" },
  );
}

export async function createProvider(
  input: CreateProviderInput,
): Promise<CreateProviderResult> {
  const data = await request<Record<string, unknown>>("/v1/operator/providers", {
    method: "POST",
    body: JSON.stringify(input),
  });
  return {
    provider: asProvider(data.provider),
    testEnvironment: asEnvironment(data.test_environment),
    apiKey: extractApiKey(data.api_key),
  };
}

export async function transitionLifecycle(
  providerId: string,
  to: LifecycleTarget,
  reason?: string,
): Promise<LifecycleResult> {
  const data = await request<Record<string, unknown>>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/lifecycle`,
    { method: "POST", body: JSON.stringify({ to, ...(reason ? { reason } : {}) }) },
  );
  return {
    provider: asProvider(data.provider),
    environment: asEnvironment(data.environment ?? data.live_environment),
    apiKey: extractApiKey(data.api_key),
  };
}

/** 激活 Provider（资源分配事件）：分配区域/Cell、创建测试环境并签发 API Key。 */
export async function activateProvider(
  providerId: string,
  homeRegionCode: string,
  reason?: string,
): Promise<CreateProviderResult> {
  const data = await request<Record<string, unknown>>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/activate`,
    {
      method: "POST",
      body: JSON.stringify({ home_region_code: homeRegionCode, ...(reason ? { reason } : {}) }),
    },
  );
  return {
    provider: asProvider(data.provider),
    testEnvironment: asEnvironment(data.test_environment),
    apiKey: extractApiKey(data.api_key),
  };
}
