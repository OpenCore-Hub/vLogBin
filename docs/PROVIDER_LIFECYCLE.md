# Provider 生命周期 / 准入 / Offboarding 契约

## 准入

- 首次 `LIVE_REVIEW → LIVE_ACTIVE` 必须已有 approved Risk Review。
- 能力授权按 `provider_capabilities` 独立授予；缺失能力不开放对应 API。
- PSP / Webhook 配置属于 Runbook 前置清单，上线审查确认后再放行（见 `docs/RUNBOOK.md`）。

## 暂停与受限

- `SUSPENDED`：写操作被 `provider_not_writable` 阻断；读操作保留用于审计/取证；恢复后积压按语义补投递。
- `RESTRICTED`：仍保持投递等受限能力，不破坏留存财务数据。

## Offboarding

1. 最终账单与全量导出：`POST /v1/data-exports`，返回可校验 `data_hash`。
2. 删除证明：`POST /v1/data-deletion`，返回 `proof_signature`。
3. 进入 `OFFBOARDING`：写操作全部阻断，读操作保留。
4. 凭证吊销 / 数据保留 / 备份过期由运营流程执行并审计。

## 契约门禁

- `TestProviderOffboardingEndToEnd`：导出 → 删除证明 → LIVE_ACTIVE → OFFBOARDING → 写阻断 / 读保留。
- `TestRiskReviewGoLiveGate` / `TestLifecycleStateMachineMatrix`。
- `TestWebhookDeliveryLifecycleAware`：SUSPENDED 积压与 RESTRICTED 继续投递语义。
