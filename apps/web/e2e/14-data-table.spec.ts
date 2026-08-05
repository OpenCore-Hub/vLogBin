import { test, expect, api } from "./helpers";

/**
 * E2E-14: DataTable URL-ized filtering (R16) — search, sort, pagination and
 * browser back are all reflected in the URL on the Customers page.
 */
async function seedCustomer(
  providerId: string,
  env: "test" | "live",
  externalId: string,
  displayName: string,
  accountType: "business" | "individual",
) {
  const r = await api(
    "POST",
    `/v1/operator/providers/${providerId}/customers?env=${env}`,
    {
      external_id: externalId,
      account_type: accountType,
      display_name: displayName,
    },
  );
  expect(r.status, `seed customer ${displayName}: ${JSON.stringify(r.body)}`).toBe(201);
}

test.describe("DataTable URL filtering", () => {
  test("search, sort, pagination and back navigation stay URL-ized", async ({
    page,
    freshProvider,
  }) => {
    await seedCustomer(freshProvider.id, "test", "alpha-1", "Alpha Corp", "business");
    await seedCustomer(freshProvider.id, "test", "beta-1", "Beta LLC", "individual");

    await page.goto("/console/billing/customers?env=test");
    await expect(page.getByText("Alpha Corp", { exact: true })).toBeVisible();
    await expect(page.getByText("Beta LLC", { exact: true })).toBeVisible();

    // Search updates ?q= and filters rows.
    const search = page.getByLabel("搜索列表");
    await search.fill("Alpha");
    await expect(page).toHaveURL(/q=Alpha/);
    await expect(page.getByText("Alpha Corp", { exact: true })).toBeVisible();
    await expect(page.getByText("Beta LLC", { exact: true })).toHaveCount(0);

    // Clear the search and both rows return.
    await page.getByRole("button", { name: "清空搜索" }).click();
    await expect(page).not.toHaveURL(/q=/);
    await expect(page.getByText("Beta LLC", { exact: true })).toBeVisible();

    // Sorting writes ?sort=&dir= to the URL and browser back restores state.
    await page.getByRole("button", { name: "客户", exact: true }).click();
    await expect(page).toHaveURL(/sort=name/);
    await expect(page).toHaveURL(/dir=asc/);
    await page.getByRole("button", { name: "客户", exact: true }).click();
    await expect(page).toHaveURL(/dir=desc/);
    await page.goBack();
    await expect(page).toHaveURL(/dir=asc/);
    await page.goBack();
    await expect(page).not.toHaveURL(/sort=/);

    // Pagination is URL-driven with a per-page selector.
    await page.goto("/console/billing/customers?env=test&pageSize=1");
    await expect(page.locator("tbody tr")).toHaveCount(1);
    await expect(page.getByText("第 1 / 2 页", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "下一页" }).click();
    await expect(page).toHaveURL(/page=2/);
    await expect(page.getByText("第 2 / 2 页", { exact: true })).toBeVisible();
    await expect(page.locator("tbody tr")).toHaveCount(1);
  });
});
