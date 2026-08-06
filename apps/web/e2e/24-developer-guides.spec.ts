import { test, expect } from "./helpers";

/**
 * E2E-24: Developers SDK + event spec pages render their reference content.
 */
test.describe("Developer guides", () => {
  test("SDK and event spec pages render reference content", async ({
    page,
  }) => {
    await page.goto("/console/developers/sdk");
    await expect(page.getByRole("heading", { name: "SDK" })).toBeVisible();
    await expect(page.getByText("usage/ingest", { exact: true }).first()).toBeVisible();

    await page.goto("/console/developers/events-spec");
    await expect(
      page.getByRole("heading", { name: "事件规范" }),
    ).toBeVisible();
    await expect(
      page.getByText("usage.accepted", { exact: true }).first(),
    ).toBeVisible();
  });
});
