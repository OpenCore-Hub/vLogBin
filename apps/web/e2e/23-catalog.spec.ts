import { test, expect, api } from "./helpers";

/**
 * E2E-23: Catalog control plane — version metadata, metrics, plans, prices
 * and entitlements render from a published catalog.
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
      entitlements: [
        { key: "max_users", value_type: "numeric", value: 10 },
      ],
    },
  ],
};

async function seedPublishedCatalog(apiKey: string): Promise<void> {
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
}

test.describe("Catalog control plane", () => {
  test("renders version, metrics, plans, prices and entitlements", async ({
    page,
    freshProvider,
  }) => {
    await seedPublishedCatalog(freshProvider.apiKey);
    await page.goto("/console/catalog?env=test");

    await expect(page.getByRole("heading", { name: "Catalog" })).toBeVisible();
    await expect(page.getByText("api_calls", { exact: true })).toBeVisible();
    await expect(page.getByText("starter", { exact: true })).toBeVisible();
    await expect(page.getByText("max_users", { exact: true })).toBeVisible();
  });
});
