/* eslint-disable @typescript-eslint/no-explicit-any, react-hooks/rules-of-hooks */
import {
  test as base,
  expect,
  type Browser,
  type BrowserContext,
  type Page,
} from "@playwright/test";

/**
 * Test helpers for the Provider Console E2E suite.
 *
 * The Web console uses Server Actions authenticated via OPERATOR_TOKEN
 * (set server-side). Tests interact purely through the browser UI —
 * exactly like a human operator would — and additionally call the
 * Platform API directly to set up prerequisites and verify outcomes.
 */

const API_BASE = process.env.API_URL || "http://localhost:8082";
const OPERATOR_TOKEN =
  process.env.OPERATOR_TOKEN || "dev-operator-token-change-me";

/** Unique slug generator to avoid collisions between parallel tests. */
let slugCounter = 0;
export function uniqueSlug(prefix: string): string {
  slugCounter += 1;
  const ts = Date.now().toString(36);
  const rand = Math.random().toString(36).slice(2, 6);
  return `${prefix}-${ts}-${rand}-${slugCounter}`;
}

/** Call the Platform API directly (for setup/verification only). */
export async function api(
  method: string,
  path: string,
  body?: Record<string, unknown>,
  token?: string,
): Promise<{ status: number; body: any }> {
  const authToken = token || OPERATOR_TOKEN;
  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${authToken}`,
      "Content-Type": "application/json",
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  let json: any = null;
  try {
    json = text ? JSON.parse(text) : null;
  } catch {
    json = text;
  }
  return { status: res.status, body: json };
}

/** Create a provider via API and return its ID and API key. */
export async function createProviderViaAPI(
  slug: string,
  name = slug + " name",
): Promise<{ id: string; apiKey: string }> {
  const r = await api("POST", "/v1/operator/providers", {
    slug,
    name,
    home_region_code: "cn-shanghai",
  });
  expect(r.status, `create provider: ${JSON.stringify(r.body)}`).toBe(201);
  return {
    id: r.body.provider.id as string,
    apiKey: r.body.api_key as string,
  };
}

/** 提交 approved 风险审核（候选 28 go-live 门禁；重复提交会 409，可忽略）。 */
async function submitApprovedRiskReview(providerId: string): Promise<void> {
  const r = await api(
    "POST",
    `/v1/operator/providers/${providerId}/risk-review`,
    {
      risk_score: 20,
      checks: {
        email_and_company_domain: true,
        tos_dpa: true,
        custom_domain_ownership: true,
        payment_tax_connection: true,
        webhook_destination: true,
        initial_quota: true,
        security_contact: true,
      },
      decision: "approved",
      reason: "go-live checklist verified",
      reviewed_by: "op-e2e",
    },
  );
  expect(
    [201, 409].includes(r.status),
    `risk review: ${r.status} ${JSON.stringify(r.body)}`,
  ).toBe(true);
}

/** Transition a provider's lifecycle via API. */
export async function transitionViaAPI(
  providerId: string,
  to: string,
): Promise<void> {
  if (to === "LIVE_ACTIVE") {
    await submitApprovedRiskReview(providerId);
  }
  const r = await api(
    "POST",
    `/v1/operator/providers/${providerId}/lifecycle`,
    { to },
  );
  expect(r.status, `transition to ${to}: ${JSON.stringify(r.body)}`).toBe(200);
}

/** 通过 UI 登录（Operator Token 表单），登录成功后位于 /console。 */
export async function loginViaUI(page: Page): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("Operator Token").fill(OPERATOR_TOKEN);
  await page.getByRole("button", { name: "登录" }).click();
  await page.waitForURL(/\/ops|\/console/, { timeout: 15_000 });
}

type SessionState = Awaited<ReturnType<BrowserContext["storageState"]>>;

/** Worker 级登录：57 条用例只调一次登录，避免 dev 限流桶被打满。 */
let sessionStatePromise: Promise<SessionState> | null = null;

async function getWorkerSession(browser: Browser): Promise<SessionState> {
  if (!sessionStatePromise) {
    sessionStatePromise = (async () => {
      const context = await browser.newContext();
      const page = await context.newPage();
      await loginViaUI(page);
      const state = await context.storageState();
      await context.close();
      return state;
    })();
  }
  return sessionStatePromise;
}

/** Extended test fixture with a fresh provider created before the test. */
type Fixtures = {
  freshProvider: { id: string; slug: string; apiKey: string };
  /** 每个测试复用 worker 级会话 cookie，独立 context。 */
  page: Page;
};

export const test = base.extend<Fixtures>({
  freshProvider: async ({}, use) => {
    const slug = uniqueSlug("e2e");
    const { id, apiKey } = await createProviderViaAPI(slug);
    await use({ id, slug, apiKey });
  },
  page: async ({ browser }, use) => {
    const sessionState = await getWorkerSession(browser);
    const context = await browser.newContext({ storageState: sessionState });
    const page = await context.newPage();
    // Workaround for Next.js 16.2 standalone + HTTP/1.1 streaming RSC bug:
    // the browser aborts directly-consumed streaming RSC responses — both
    // client-side navigation (GET ?_rsc=) and Server Actions (POST with the
    // Next-Action header) — as net::ERR_ABORTED with a silent router
    // rollback. Buffering and forwarding these responses (non-streaming)
    // restores navigation/UI updates. Production should sit behind an
    // HTTP/2 reverse proxy instead.
    await page.route("**/*", async (route) => {
      const req = route.request();
      const url = req.url();
      if (url.includes("/_next/static") || url.includes("/_next/image")) {
        await route.continue();
        return;
      }
      const isRscNav = req.method() === "GET" && /[?&]_rsc=/.test(url);
      const isServerAction =
        req.method() === "POST" && !!req.headers()["next-action"];
      if (!isRscNav && !isServerAction) {
        await route.continue();
        return;
      }
      try {
        const response = await route.fetch();
        await route.fulfill({ response });
      } catch {
        try {
          await route.continue();
        } catch {
          // Route was already fulfilled/aborted; nothing to do.
        }
      }
    });
    await use(page);
    try {
      await context.close();
    } catch {
      // Context may already be disposed when the test failed mid-navigation.
    }
  },
});

export { expect };
