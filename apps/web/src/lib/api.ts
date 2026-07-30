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
