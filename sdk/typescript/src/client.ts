export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId?: string;
  readonly retryAfter?: string;
  readonly details?: unknown;

  constructor(
    code: string,
    message: string,
    status: number,
    requestId?: string,
    retryAfter?: string,
    details?: unknown,
  ) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
    this.requestId = requestId;
    this.retryAfter = retryAfter;
    this.details = details;
  }
}

export interface RequestOptions {
  idempotencyKey?: string;
  query?: Record<string, string>;
}

export class VLogBinClient {
  constructor(
    private readonly baseUrl: string,
    private readonly apiKey: string,
  ) {}

  async request<T>(
    method: string,
    path: string,
    options: RequestOptions = {},
    body?: unknown,
  ): Promise<T> {
    const url = new URL(this.baseUrl + path);
    for (const [key, value] of Object.entries(options.query ?? {})) {
      url.searchParams.set(key, value);
    }
    const headers: Record<string, string> = {
      Authorization: `Bearer ${this.apiKey}`,
      "Content-Type": "application/json",
    };
    if (options.idempotencyKey) {
      headers["Idempotency-Key"] = options.idempotencyKey;
    }
    const res = await fetch(url, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!res.ok) {
      throw await apiErrorFromResponse(res);
    }
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }
}

async function apiErrorFromResponse(res: Response): Promise<ApiError> {
  let code = "api_error";
  let message = `Request failed with status ${res.status}`;
  let requestId: string | undefined;
  let retryAfter: string | undefined;
  let details: unknown;
  try {
    const body = (await res.json()) as {
      error?: {
        code?: string;
        message?: string;
        request_id?: string;
        retry_after?: string;
        details?: unknown;
      };
    };
    code = body.error?.code ?? code;
    message = body.error?.message ?? message;
    requestId = body.error?.request_id;
    retryAfter = body.error?.retry_after;
    details = body.error?.details;
  } catch {
    // Non-JSON error body.
  }
  return new ApiError(code, message, res.status, requestId, retryAfter, details);
}
