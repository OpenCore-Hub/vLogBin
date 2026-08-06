# JIT Support / 审计 / WORM 契约

## JIT Support Access

- Standard：operator 请求 → Provider 审批 → active → 到期自动 expire。
- Emergency：operator 请求 → 双人审批（first + second）→ active（最长 1 小时）。
- 所有请求/审批/吊销均写入审计，并带 `request_id` 可关联。
- 会话 scope 限制在请求声明的 scopes 内。

## 审计

- 审计事件使用哈希链：`prev_hash` / `event_hash` 防篡改，`audit_chain_verify` 可定位断点。
- 保留策略：`AUDIT_RETENTION_DAYS` 显式配置才启用 sweeper；审计是合规证据，默认不自动删除。
- 错误响应、请求日志、审计事件共享 `request_id`。

## WORM

- 审计锚点发布到 S3 兼容 WORM 对象存储：`audit_anchors_published_total` 持续增长 = 归档健康。
- 发布协议幂等：取批 → 事务外上传 → 守卫 mark；崩溃恢复不重复、不丢失。

## 契约门禁

- `TestSupportSessionAuditCorrelation`：request/approve 审计事件 + `request_id`。
- `TestSupportSessionEmergencyTwoPerson`、`TestSupportSessionExpiry`、`TestSupportSessionCrossTenantIsolation`。
- `TestAuditChainHashAppended`、`TestAuditChainTamperDetection`、`TestAuditAnchorArchivePublishesAndIsIdempotent`。
