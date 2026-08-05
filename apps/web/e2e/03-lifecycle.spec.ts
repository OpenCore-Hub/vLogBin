import { test, expect, transitionViaAPI, api } from "./helpers";

/**
 * E2E-03: Lifecycle actions — human clicks the lifecycle buttons on
 * the provider detail page and observes the state changes (and that
 * illegal buttons are disabled).
 */
test.describe("Lifecycle actions", () => {
  test("TEST_ACTIVE: only 申请上线审核 button is enabled", async ({
    page,
    freshProvider,
  }) => {
    await page.goto(`/ops/${freshProvider.id}`);

    const reviewBtn = page.getByRole("button", { name: "申请上线审核" });
    const activateBtn = page.getByRole("button", { name: "激活生产环境" });
    const restrictBtn = page.getByRole("button", { name: "限制" });
    const suspendBtn = page.getByRole("button", { name: "暂停" });

    await expect(reviewBtn).toBeVisible();
    await expect(activateBtn).toBeVisible();
    await expect(restrictBtn).toBeVisible();
    await expect(suspendBtn).toBeVisible();

    await expect(reviewBtn).toBeEnabled();
    await expect(activateBtn).toBeDisabled();
    await expect(restrictBtn).toBeDisabled();
    await expect(suspendBtn).toBeDisabled();

    await expect(activateBtn).toHaveAttribute(
      "title",
      "当前状态 TEST_ACTIVE 不允许此操作",
    );
  });

  test("clicking 申请上线审核 transitions to LIVE_REVIEW state", async ({
    page,
    freshProvider,
  }) => {
    await page.goto(`/ops/${freshProvider.id}`);

    await page.getByRole("button", { name: "申请上线审核" }).click();

    await expect
      .poll(
        async () => {
          const r = await api("GET", `/v1/operator/providers/${freshProvider.id}`);
          return r.body?.provider?.lifecycle_state;
        },
        { timeout: 15_000, message: "provider should transition to LIVE_REVIEW" },
      )
      .toBe("LIVE_REVIEW");
  });

  test("full lifecycle walk: TEST_ACTIVE → REVIEW → ACTIVE → RESTRICTED → ACTIVE → SUSPENDED → ACTIVE", async ({
    page,
    freshProvider,
  }) => {
    const transitions: [string, string][] = [
      ["申请上线审核", "LIVE_REVIEW"],
      ["激活生产环境", "LIVE_ACTIVE"],
      ["限制", "RESTRICTED"],
      ["激活生产环境", "LIVE_ACTIVE"], // 曾经回归的 RESTRICTED → LIVE_ACTIVE
      ["暂停", "SUSPENDED"],
      ["激活生产环境", "LIVE_ACTIVE"],
    ];

    await page.goto(`/ops/${freshProvider.id}`);

    for (const [buttonLabel, expectedState] of transitions) {
      await expect(
        page.getByRole("button", { name: buttonLabel }),
      ).toBeEnabled({ timeout: 10_000 });

      await page.getByRole("button", { name: buttonLabel }).click();

      // 候选 28 go-live 门禁：进入 LIVE_REVIEW 后先提交 approved 风险审核，
      // 首次 LIVE_REVIEW → LIVE_ACTIVE 才可放行（后续 reactivation 复用记录）。
      if (expectedState === "LIVE_REVIEW") {
        await expect
          .poll(
            async () => {
              const r = await api(
                "GET",
                `/v1/operator/providers/${freshProvider.id}`,
              );
              return r.body?.provider?.lifecycle_state;
            },
            { timeout: 15_000 },
          )
          .toBe("LIVE_REVIEW");
        const r = await api(
          "POST",
          `/v1/operator/providers/${freshProvider.id}/risk-review`,
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

      await expect
        .poll(
          async () => {
            const r = await api(
              "GET",
              `/v1/operator/providers/${freshProvider.id}`,
            );
            return r.body?.provider?.lifecycle_state;
          },
          {
            timeout: 15_000,
            message: `provider should transition to ${expectedState} after clicking ${buttonLabel}`,
          },
        )
        .toBe(expectedState);

      await page.reload();
    }

    const r = await api("GET", `/v1/operator/providers/${freshProvider.id}`);
    expect(r.body.provider.lifecycle_state).toBe("LIVE_ACTIVE");
  });

  test("LIVE_REVIEW: 申请上线审核 is disabled (no self-transition)", async ({
    page,
    freshProvider,
  }) => {
    await transitionViaAPI(freshProvider.id, "LIVE_REVIEW");
    await page.goto(`/ops/${freshProvider.id}`);

    const reviewBtn = page.getByRole("button", { name: "申请上线审核" });
    await expect(reviewBtn).toBeDisabled();

    await expect(
      page.getByRole("button", { name: "激活生产环境" }),
    ).toBeEnabled();
    await expect(page.getByRole("button", { name: "限制" })).toBeEnabled();
    await expect(page.getByRole("button", { name: "暂停" })).toBeEnabled();
  });

  test("LIVE_ACTIVE: 申请上线审核 and 激活生产环境 are disabled", async ({
    page,
    freshProvider,
  }) => {
    await transitionViaAPI(freshProvider.id, "LIVE_REVIEW");
    await transitionViaAPI(freshProvider.id, "LIVE_ACTIVE");
    await page.goto(`/ops/${freshProvider.id}`);

    await expect(
      page.getByRole("button", { name: "申请上线审核" }),
    ).toBeDisabled();
    await expect(
      page.getByRole("button", { name: "激活生产环境" }),
    ).toBeDisabled();
    await expect(page.getByRole("button", { name: "限制" })).toBeEnabled();
    await expect(page.getByRole("button", { name: "暂停" })).toBeEnabled();
  });
});
