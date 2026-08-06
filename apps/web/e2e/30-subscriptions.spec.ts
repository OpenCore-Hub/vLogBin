import { test, expect, api } from "./helpers";

/**
 * E2E-30: Subscriptions control plane — subscription list renders real
 * seeded subscriptions with plan and status.
 */
const CATALOG_CONTENT = {
  metrics: [
    {
      code: "api_calls",
      name: "API Calls",
      aggregation_type: "count",
      billable: true,
    },
  ],
  plans: [
    {
      code: "starter",
      name: "Starter",
      interval: "monthly",
      currency: "USD",
      prices: [
        {
          charge_model: "fixed",
          properties: { amount_cents: 1000, currency: "USD" },
        },
      ],
    },
  ],
};

async function seedSubscription(apiKey: string): Promise<string> {
  const v = await api("POST", "/v1/catalog/versions", {}, apiKey);
  expect(v.status, `create catalog version: ${JSON.stringify(v.body)}`).toBe(201);
  const versionId = v.body.version.id;

  const c = await api(
    "PUT",
    `/v1/catalog/versions/${versionId}/content`,
    CATALOG_CONTENT,
    apiKey,
  );
  expect(c.status, `replace content: ${JSON.stringify(c.body)}`).toBe(200);

  const val = await api(
    "POST",
    `/v1/catalog/versions/${versionId}/validate`,
    undefined,
    apiKey,
  );
  expect(val.status, `validate: ${JSON.stringify(val.body)}`).toBe(200);

  const pub = await api(
    "POST",
    `/v1/catalog/versions/${versionId}/publish`,
    undefined,
    apiKey,
  );
  expect(pub.status, `publish: ${JSON.stringify(pub.body)}`).toBe(200);

  const customerExt = `sub-cust-${Date.now()}`;
  const cust = await api(
    "POST",
    "/v1/customers",
    {
      external_id: customerExt,
      account_type: "business",
      display_name: "Subscription Customer",
    },
    apiKey,
  );
  expect(cust.status, `create customer: ${JSON.stringify(cust.body)}`).toBe(201);

  const subExt = `sub-${Date.now()}`;
  const sub = await api(
    "POST",
    "/v1/subscriptions",
    {
      external_id: subExt,
      customer_external_id: customerExt,
      catalog_version_id: versionId,
      plan_code: "starter",
    },
    apiKey,
  );
  expect(sub.status, `create subscription: ${JSON.stringify(sub.body)}`).toBe(201);
  return subExt;
}

test.describe("Subscriptions control plane", () => {
  test("renders seeded subscription with plan and status", async ({
    page,
    freshProvider,
  }) => {
    const subExt = await seedSubscription(freshProvider.apiKey);
    await page.goto("/console/billing/subscriptions?env=test");

    await expect(page.getByRole("heading", { name: "订阅" })).toBeVisible();
    await expect(page.getByText("订阅数", { exact: true })).toBeVisible();
    await expect(page.getByText(subExt, { exact: true })).toBeVisible();
    await expect(page.getByText("starter", { exact: true })).toBeVisible();
    await expect(page.getByText("active", { exact: true })).toBeVisible();
  });
});
