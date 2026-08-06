# Catalog / Subscription / Entitlement / Quota 契约

## Catalog 与 Subscription

- 已发布目录版本不可变：`PUT /catalog/versions/{id}/content`、重复 validate、重复 publish 均拒绝 409。
- Subscription 固定绑定 `catalog_version_id`；发布新版本不改变既有订阅。
- 每个 Invoice Line 保留 `metric_id` / `price_id` / `event_transaction_id` 可重放。

## Entitlement

- 单一真相源是“pinned catalog version + 订阅状态”计算的 `entitlement_snapshot`。
- 发布新版本不会改变既有订阅的权益快照。
- 订阅级 override 非过期时优先；过期后回落到 plan grant。

## Quota

- Hard Quota：reserve / commit / release / expire 由持久化账本强制执行；并发 reserve 不超过 limit。
- 无额度行或额度不足按文档返回 `quota_exceeded` / 404。
- 预占带 `reservation_id` 幂等；同 ID 重试返回原预占。
- 当前未开放 Soft Quota overage；有界 overage 语义待后续能力开启后再发布契约。

## 契约门禁

- `TestBillingCatalogImmutableAfterPublish`、`TestBillingSubscriptionPinning`。
- `TestEntitlementSnapshotSingleSourceOfTruth`。
- `TestQuotaReserveCommitRelease`、`TestQuotaConcurrentReserve`、`TestQuotaExceeded`、`TestQuotaIdempotency`、`TestQuotaExpiry`。
