import { test, expect, api } from "./helpers";

/**
 * E2E-17: 运营商台增强 (§8 M3) — Providers / 审核 / Cell 运维三个页签。
 */
test.describe("Ops enhancement", () => {
  test("provider table stays intact and new tabs render", async ({ page }) => {
    await page.goto("/ops");
    await expect(page.getByRole("heading", { name: "Providers" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "审核" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Cell 运维" })).toBeVisible();

    await page.getByRole("tab", { name: "审核" }).click();
    await expect(page.getByText("待审核", { exact: true })).toBeVisible();
    await expect(page.getByText("风险审核记录", { exact: true })).toBeVisible();
    await expect(page.getByText("支持会话", { exact: true })).toBeVisible();

    await page.getByRole("tab", { name: "Cell 运维" }).click();
    await expect(page.getByText("Cells", { exact: true })).toBeVisible();
    await expect(page.getByText("故障切换", { exact: true })).toBeVisible();
    await expect(page.getByText("Cell 迁移", { exact: true })).toBeVisible();
  });

  test("create a cell from the Cell ops tab", async ({ page }) => {
    const regions = await api("GET", "/v1/operator/regions");
    expect(regions.status).toBe(200);
    const regionId = regions.body.regions[0].id as string;
    const code = `e2e-${Date.now()}`;

    await page.goto("/ops");
    await page.getByRole("tab", { name: "Cell 运维" }).click();
    await page.getByRole("button", { name: "新建 Cell" }).click();
    await page.getByRole("dialog").getByLabel("Region").selectOption(regionId);
    await page.getByRole("dialog").getByLabel("Code").fill(code);
    await page.getByRole("dialog").getByRole("button", { name: "创建 Cell" }).click();
    await expect(
      page.getByRole("dialog").getByText("Cell 已创建", { exact: true }),
    ).toBeVisible();
  });

  test("support sessions render approvals and operator can revoke", async ({
    page,
    freshProvider,
  }) => {
    const providerRes = await api(
      "GET",
      `/v1/operator/providers/${freshProvider.id}`,
    );
    expect(providerRes.status).toBe(200);
    const env = (
      providerRes.body.environments as Array<{ id: string; kind: string }>
    ).find((e) => e.kind === "test");
    expect(env).toBeTruthy();

    const emergencyReason = `e2e-emergency-${Date.now()}`;
    const emergency = await api(
      "POST",
      `/v1/operator/providers/${freshProvider.id}/support-sessions`,
      {
        environment_id: env!.id,
        access_type: "emergency",
        reason: emergencyReason,
        requested_scopes: ["read"],
        duration_seconds: 1800,
      },
    );
    expect(emergency.status).toBe(201);

    const revokeReason = `e2e-revoke-${Date.now()}`;
    const requested = await api(
      "POST",
      `/v1/operator/providers/${freshProvider.id}/support-sessions`,
      {
        environment_id: env!.id,
        access_type: "standard",
        reason: revokeReason,
        requested_scopes: ["read"],
        duration_seconds: 1800,
      },
    );
    expect(requested.status).toBe(201);
    const sessionId = requested.body.id as string;
    const approved = await api(
      "POST",
      `/v1/support-sessions/${sessionId}/approve`,
      undefined,
      freshProvider.apiKey,
    );
    expect(approved.status).toBe(200);
    expect(approved.body.status).toBe("active");

    await page.goto("/ops/reviews");
    const supportSection = page.locator("section").filter({ hasText: "支持会话" });

    await supportSection.getByRole("searchbox", { name: "搜索列表" }).fill(emergencyReason);
    const emergencyRow = supportSection.locator("tbody tr", { hasText: emergencyReason });
    await expect(emergencyRow.getByRole("button", { name: "一审" })).toBeVisible();
    await expect(emergencyRow.getByRole("button", { name: "二审" })).toBeDisabled();
    await emergencyRow.getByRole("button", { name: "一审" }).click();
    await expect(page.getByRole("dialog")).toContainText("第一审批");
    await page.getByRole("dialog").getByRole("button", { name: "取消" }).click();

    await supportSection.getByRole("searchbox", { name: "搜索列表" }).fill(revokeReason);
    const revokeRow = supportSection.locator("tbody tr", { hasText: revokeReason });
    await expect(revokeRow).toContainText("active");
    await revokeRow.getByRole("button", { name: "吊销" }).click();
    await expect(page.getByRole("dialog")).toContainText("吊销支持会话");
    await page.getByLabel("输入 operator 确认").fill("operator");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "吊销会话" })
      .click();

    await expect(page.getByRole("dialog")).toBeHidden();
    await expect
      .poll(async () => {
        const verify = await api(
          "GET",
          `/v1/operator/providers/${freshProvider.id}/support-sessions`,
        );
        if (verify.status !== 200 || !Array.isArray(verify.body.support_sessions)) {
          return null;
        }
        const session = (
          verify.body.support_sessions as Array<{ id: string; status: string }>
        ).find((s) => s.id === sessionId);
        return session?.status ?? null;
      })
      .toBe("revoked");
  });
});
