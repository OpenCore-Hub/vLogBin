import { test, expect, api } from "./helpers";

/**
 * E2E-12: First-run onboarding — verifies the four-step pipeline from
 * §2.2 (application → plan → customer → usage) reflects real data and that
 * the progress strip steps are clickable.
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

async function seedPublishedPlan(apiKey: string): Promise<void> {
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

test.describe("First-run onboarding", () => {
  test("four-step progress reflects data and steps are clickable", async ({
    page,
    freshProvider,
  }) => {
    // Fresh provider: no app / plan / customer / usage yet.
    await page.goto("/console");
    await expect(page.getByText("完成度 0%", { exact: true })).toBeVisible();

    const stripLinks = [
      "创建第一个应用",
      "创建第一个套餐",
      "创建第一个客户",
      "上报第一条用量事件",
    ];
    for (const title of stripLinks) {
      await expect(
        page.getByRole("link", { name: title }),
      ).toBeVisible();
    }

    // Seeding a published plan advances the second step.
    await seedPublishedPlan(freshProvider.apiKey);
    await page.goto("/console");
    await expect(page.getByText("完成度 25%", { exact: true })).toBeVisible();
    await expect(page.getByText("已完成 1/4 步", { exact: true })).toBeVisible();

    // The first incomplete step is clickable and jumps to its page.
    await page.getByRole("link", { name: "创建第一个应用" }).click();
    await expect(page).toHaveURL(/\/console\/identity\/applications$/);
  });
});
