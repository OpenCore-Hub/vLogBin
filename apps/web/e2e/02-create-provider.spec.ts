import { test, expect, uniqueSlug, api } from "./helpers";

/**
 * E2E-02: Create Provider form — human fills out the "New Provider"
 * form and submits it, then sees the API key and navigates to the
 * detail page.
 */
test.describe("Create Provider form", () => {
  test("navigate to create form via New Provider button", async ({ page }) => {
    await page.goto("/ops");
    await page.getByRole("link", { name: "新建 Provider" }).click();
    await expect(page).toHaveURL("/ops/new");

    await expect(page.getByLabel("Slug")).toBeVisible();
    await expect(page.getByLabel("名称")).toBeVisible();
    await expect(page.getByLabel("所属区域")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "创建 Provider" }),
    ).toBeVisible();
  });

  test("region dropdown is populated", async ({ page }) => {
    await page.goto("/ops/new");
    const select = page.getByLabel("所属区域");
    const options = select.locator("option");
    expect(await options.count()).toBeGreaterThan(1);
  });

  test("slug validation rejects invalid input with inline error", async ({
    page,
  }) => {
    await page.goto("/ops/new");
    await page.getByLabel("Slug").fill("Bad Slug!");
    await page.getByLabel("名称").fill("Invalid slug test");
    await page.getByLabel("所属区域").selectOption({ index: 1 });
    await page.getByRole("button", { name: "创建 Provider" }).click();

    // zod 校验失败：字段错误提示可见，且未跳转。
    await expect(page.getByText("小写字母、数字与中划线")).toBeVisible();
    await expect(page).toHaveURL("/ops/new");
  });

  test("successfully create a provider via the form", async ({ page }) => {
    const slug = uniqueSlug("form");

    await page.goto("/ops/new");
    await page.getByLabel("Slug").fill(slug);
    await page.getByLabel("名称").fill(slug + " display name");
    await page.getByLabel("所属区域").selectOption({ index: 1 });

    await page.getByRole("button", { name: "创建 Provider" }).click();

    await expect(page.getByText("Provider 创建成功")).toBeVisible();

    const apiKeyText = page.locator("code").filter({ hasText: "pk_test_" });
    await expect(apiKeyText).toBeVisible();
    const apiKey = (await apiKeyText.textContent()) || "";
    expect(apiKey.startsWith("pk_test_")).toBe(true);

    await page.getByRole("link", { name: "查看 Provider 详情" }).click();
    await expect(page).toHaveURL(/\/ops\/[a-f0-9-]+/);
    await expect(page.getByText(slug + " display name")).toBeVisible();

    const pageUrl = page.url();
    const providerId = pageUrl.split("/ops/")[1];
    const r = await api("GET", `/v1/operator/providers/${providerId}`);
    expect(r.status).toBe(200);
    expect(r.body.provider.lifecycle_state).toBe("TEST_ACTIVE");
    expect(r.body.provider.slug).toBe(slug);
  });

  test("duplicate slug shows error in the form", async ({ page }) => {
    const slug = uniqueSlug("dup");
    await api("POST", "/v1/operator/providers", {
      slug,
      name: "first",
      home_region_code: "cn-shanghai",
    });

    await page.goto("/ops/new");
    await page.getByLabel("Slug").fill(slug);
    await page.getByLabel("名称").fill("second");
    await page.getByLabel("所属区域").selectOption({ index: 1 });
    await page.getByRole("button", { name: "创建 Provider" }).click();

    // 注意：Next.js Dev Tools 也会注入一个空的 role="alert"，
    // 必须按内容过滤，只匹配表单里的错误提示。
    const alert = page.getByRole("alert").filter({ hasText: "创建失败" });
    await expect(alert).toBeVisible({ timeout: 15_000 });
    const alertText = (await alert.textContent()) || "";
    expect(alertText.length).toBeGreaterThan(0);
  });

  test("Cancel button returns to providers list", async ({ page }) => {
    await page.goto("/ops/new");
    await page.getByRole("link", { name: "取消" }).click();
    await expect(page).toHaveURL("/ops");
  });
});
