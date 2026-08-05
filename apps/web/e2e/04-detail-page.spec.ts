import { test, expect } from "./helpers";

/**
 * E2E-04: Provider detail page — the tabs and core sections render
 * without errors when a human navigates to the detail page.
 *
 * This catches the class of bug where a single failing API call
 * (Promise.allSettled rejection) breaks the page layout.
 */
test.describe("Provider detail page sections", () => {
  test("header shows name, slug and lifecycle badge", async ({
    page,
    freshProvider,
  }) => {
    await page.goto(`/ops/${freshProvider.id}`);

    await expect(
      page.getByRole("heading", { name: freshProvider.slug + " name" }),
    ).toBeVisible();
    await expect(
      page.getByText(`@${freshProvider.slug}`, { exact: true }).first(),
    ).toBeVisible();
    // header 与 overview 生命周期卡片各有一个 TEST_ACTIVE badge。
    await expect(page.getByText("TEST_ACTIVE").first()).toBeVisible();
  });

  test("overview tab shows metadata and provider ID", async ({
    page,
    freshProvider,
  }) => {
    await page.goto(`/ops/${freshProvider.id}`);

    // 限定当前 tabpanel：Radix Tabs 渲染全部面板（非激活带 hidden），
    // 而审计面板的 target_id 恰与该 ID 相同，否则 strict mode 冲突。
    const overviewPanel = page.getByRole("tabpanel");
    await expect(
      overviewPanel.getByText(freshProvider.id, { exact: false }),
    ).toBeVisible();
    await expect(page.getByText("生命周期操作")).toBeVisible();
    await expect(page.getByRole("heading", { name: "环境" })).toBeVisible();
    await expect(page.getByRole("heading", { name: "能力" })).toBeVisible();
  });

  test("environments section shows the test environment", async ({
    page,
    freshProvider,
  }) => {
    await page.goto(`/ops/${freshProvider.id}`);

    // 限定当前 tabpanel，避免匹配顶栏环境切换器里的"测试环境"
    //（根布局与 AppShell 各有 <main>，不能用 main 限定）。
    const overviewPanel = page.getByRole("tabpanel");
    await expect(overviewPanel.getByText("测试环境", { exact: true })).toBeVisible();
    await expect(overviewPanel.getByText("active", { exact: true })).toBeVisible();
  });

  test("empty sections show '暂无…' messages in each tab", async ({
    page,
    freshProvider,
  }) => {
    await page.goto(`/ops/${freshProvider.id}`);

    const emptyMessages = [
      ["目录", "暂无目录版本"],
      ["订阅", "暂无订阅"],
      ["客户", "暂无客户"],
      ["用量", "暂无用量事件"],
      ["账单", "暂无账单"],
    ] as const;

    for (const [tabName, emptyText] of emptyMessages) {
      await page.getByRole("tab", { name: new RegExp(tabName) }).click();
      await expect(page.getByText(emptyText)).toBeVisible();
    }
  });

  test("non-existent provider shows error, not crash", async ({ page }) => {
    await page.goto("/ops/00000000-0000-0000-0000-000000000000");

    const bodyText = await page.textContent("body");
    expect(bodyText).toBeTruthy();
    expect(bodyText!.length).toBeGreaterThan(0);
  });
});
