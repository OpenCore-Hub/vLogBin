/**
 * Server-side client for the platform operator API.
 *
 * Reads OPERATOR_TOKEN and PLATFORM_API_URL from the environment.
 * This module must only be imported from Server Components, Server Actions,
 * or Route Handlers — the operator token must never reach the browser.
 */

export interface Provider {
  id: string;
  slug: string;
  name: string;
  lifecycle_state: string;
  sla_tier?: string;
  home_region_id?: string;
  cell_id?: string;
  created_at?: string;
  updated_at?: string;
}

export interface Environment {
  id: string;
  provider_id: string;
  kind: string; // "test" | "live"
  status: string;
  issuer?: string;
  created_at?: string;
}

export interface Region {
  id: string;
  code: string;
  jurisdiction: string;
}

export interface CatalogVersion {
  id: string;
  version: number;
  state: string; // "draft" | "validated" | "published" | "retired"
  environment_id: string;
  environment_kind: string; // "test" | "live"
  metrics_count: number;
  plans_count: number;
  created_at?: string;
  published_at?: string;
}

export interface Subscription {
  id: string;
  external_id: string;
  customer_external_id: string;
  plan_code: string;
  catalog_version_id: string;
  status: string; // "active" | "terminated"
  environment_kind: string; // "test" | "live"
  started_at?: string;
  terminated_at?: string;
}

export interface Customer {
  id: string;
  external_id: string;
  account_type: string; // "individual" | "business"
  display_name: string;
  environment_id: string;
  environment_kind: string; // "test" | "live"
  created_at?: string;
}

export interface UsageEvent {
  id: string;
  transaction_id: string;
  kind: string; // "ingestion" | "reversal"
  metric_code: string;
  customer_external_id: string;
  environment_id: string;
  environment_kind: string;
  event_timestamp?: string;
  created_at?: string;
}

export interface Invoice {
  id: string;
  number: string;
  lago_id: string;
  issuing_date: string; // ISO date YYYY-MM-DD
  invoice_type: string; // "subscription" | "add_on" | "credit" | "one_off" | "progressive_billing"
  status: string; // "draft" | "finalized" | "voided" | "pending" | "failed"
  payment_status: string; // "pending" | "succeeded" | "failed"
  currency: string; // 3-letter ISO code
  total_amount_cents: number;
  customer_external_id: string;
  customer_account_id?: string;
  subscription_id?: string;
  catalog_version_id?: string;
  environment_id: string;
  environment_kind: string; // "test" | "live"
}

export interface Metric {
  id: string;
  code: string;
  name: string;
  aggregation_type: string;
  field_name?: string;
  billable: boolean;
}

export interface Plan {
  id: string;
  code: string;
  name: string;
  interval: string;
  currency: string;
}

export interface Price {
  id: string;
  charge_model: string; // "fixed" | "per_unit" | "tiered"
  metric_code?: string;
  properties?: unknown;
}

export interface EntitlementGrant {
  id: string;
  key: string;
  value_type: string;
  value?: unknown;
}

// Full metadata for a catalog version (richer than the list-row CatalogVersion).
export interface CatalogVersionMeta {
  id: string;
  provider_id: string;
  environment_id: string;
  version: number;
  state: string;
  created_at?: string;
  validated_at?: string;
  published_at?: string;
  retired_at?: string;
}

export interface CatalogVersionDetail {
  version: CatalogVersionMeta;
  metrics: Metric[];
  plans: Plan[];
  prices: Price[];
  entitlement_grants: EntitlementGrant[];
}

export type LifecycleTarget =
  | "LIVE_REVIEW"
  | "LIVE_ACTIVE"
  | "RESTRICTED"
  | "SUSPENDED";

export interface CreateProviderResult {
  provider: Provider | null;
  testEnvironment: Environment | null;
  apiKey: string | null;
}

export interface LifecycleResult {
  provider: Provider | null;
  environment: Environment | null;
  apiKey: string | null;
}

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
  return (process.env.PLATFORM_API_URL ?? "http://localhost:8080").replace(
    /\/+$/,
    "",
  );
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = process.env.OPERATOR_TOKEN;
  if (!token) {
    throw new ApiError(
      "missing_operator_token",
      "OPERATOR_TOKEN is not configured on the server.",
      500,
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

  return (await res.json()) as T;
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
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

export async function listCustomers(providerId: string): Promise<Customer[]> {
  const data = await request<{ customers?: unknown }>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/customers`,
  );
  if (!Array.isArray(data.customers)) return [];
  return data.customers
    .map(asCustomer)
    .filter((c): c is Customer => c !== null);
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

export interface Capability {
  id: string;
  provider_id: string;
  capability: string;
  status: string;
  granted_at?: string;
  revoked_at?: string;
  granted_by?: string;
  reason?: string;
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

export interface WebhookEndpoint {
  id: string;
  provider_id: string;
  environment_id: string;
  url: string;
  secret: string;
  enabled: boolean;
  events: string[];
  created_at?: string;
}

export interface WebhookDelivery {
  id: string;
  endpoint_id: string;
  outbox_event_id: string;
  status: string;
  attempts: number;
  response_status?: number;
  response_body?: string;
  delivered_at?: string;
  created_at?: string;
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

export async function createProvider(input: {
  slug: string;
  name: string;
  home_region_code: string;
}): Promise<CreateProviderResult> {
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
): Promise<LifecycleResult> {
  const data = await request<Record<string, unknown>>(
    `/v1/operator/providers/${encodeURIComponent(providerId)}/lifecycle`,
    { method: "POST", body: JSON.stringify({ to }) },
  );
  return {
    provider: asProvider(data.provider),
    environment: asEnvironment(data.environment ?? data.live_environment),
    apiKey: extractApiKey(data.api_key),
  };
}
