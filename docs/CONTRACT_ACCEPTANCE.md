# 公开平台契约验收映射

> 对应 SPEC §Testing Decisions 的 #1-40。每次正式商用上线门禁运行前，以本表核对覆盖状态；`待补` 项必须补齐自动化用例后才能签署上线。

| # | 验收要求 | 状态 | 证据 / 缺口 |
|---|---|---|---|
| 1 | Provider A 永远不能读/改/推断/关联 Provider B 数据 | 已覆盖 | `TestCrossTenantIsolation`、`TestBillingCrossTenantIsolation`、`TestCommercialContractAcceptance` |
| 2 | Test 凭证永远不能到达 Live 资源/发票/事件/支付/域名 | 已覆盖 | `TestEnvironmentIsolationEndToEnd`、`TestEnvironmentIsolation` |
| 3 | 请求体不能覆盖凭证推导的 Provider/Environment | 已覆盖 | `TestTenantCannotSelfEscalate`、`TestEnvironmentHeaderContract` |
| 4 | 相同用量重试只产生一次可计费效果 | 已覆盖 | `TestUsageIdempotency`、`TestCommercialContractAcceptance` |
| 5 | 相同 transaction_id 不同 payload 被拒绝并审计 | 已覆盖 | `TestUsagePayloadConflict` |
| 6 | 冲销/调整在出账前与出账后可复现 | 已覆盖 | `TestUsageReversal`、`TestUsagePostInvoiceReversal` |
| 7 | 已发布目录版本不可变 | 已覆盖 | `TestBillingCatalogImmutableAfterPublish`、`TestCommercialContractAcceptance` |
| 8 | 新版本发布后既有订阅保持 pinning | 已覆盖 | `TestBillingSubscriptionPinning`、`TestCommercialContractAcceptance` |
| 9 | 每条发票行可从不可变输入重放 | 待补 | invoice line traceability 已有迁移，缺黑盒“从输入重算出账结果”测试 |
| 10 | Platform Commerce 记录永不出现在 Provider Commerce 视图/对账 | 已覆盖 | `TestCommerceDomainIsolation` |
| 11 | B2C 用户无多余 Organization 隔离 | 待补 | 身份适配层尚未有公开契约测试 |
| 12 | B2B 成员/委托管理员/SSO/SCIM 保持在 Provider 边界 | 部分 | SCIM 跨租户测试存在；委托管理员/SSO 边界缺专项 |
| 13 | 不同 Provider 同邮箱用户获得无关 subject 与 session | 待补 | 本地模式身份为固定 operator，缺多身份黑盒测试 |
| 14 | Cell 迁移期间 issuer 与公开域名保持稳定 | 部分 | failover/cell migration 有测试；issuer/域名稳定性未专项断言 |
| 15 | OAuth scope/API key 限制/轮换/过期/吊销按文档生效 | 已覆盖 | `TestOperatorDevelopersWebhookLifecycle`、凭证轮换测试 |
| 16 | Webhook 签名/时间窗/防重放/重试/暂停/重放正确 | 已覆盖 | `TestWebhookDeliveryReplay`、`TestWebhookRetryOnFailure`、签名单测 |
| 17 | Webhook 目标不能解析/跳转到私网 | 已覆盖 | `TestWebhookSSRFPrevention` |
| 18 | 事件流消费者可从 cursor 无丢无重恢复 | 已覆盖 | `TestEventStreamCursorPagination`、`TestEventStreamCursorResumption` |
| 19 | Soft quota 允许文档化有界 overage | 待补 | 当前无 soft quota overage 契约测试 |
| 20 | Hard quota 用 reserve/commit/release 防并发超用 | 已覆盖 | `TestQuotaConcurrentReserve`、`TestQuotaReserveCommitRelease` |
| 21 | Redis 丢失不丢失权威权益/额度状态 | 部分 | Redis 限流测试覆盖；权益/额度 Redis 不参与真相源，需文档化断言 |
| 22 | Lago 故障时已接受用量保留在 Outbox 并恢复无重复 | 已覆盖 | `TestOutboxRelayOutagePreservesUsage`、`TestOutboxRelayDeliversUsage` |
| 23 | Kafka 故障造成有界持久积压而非业务数据丢失 | 待补 | 当前无 Kafka；需明确有界积压契约与容量测试 |
| 24 | PSP 重复/乱序事件不重复支付/发票状态 | 部分 | PSP 生命周期/加密测试存在；支付状态对账重复事件缺专项 |
| 25 | RLS 阻断直连与 worker 来源的跨 Provider 访问 | 已覆盖 | `TestCrossTenantIsolation`、`TestOutboxUniquenessAndIdempotentRetry` |
| 26 | JIT 会话过期、保持 scope、出现在 Provider 审计 | 已覆盖 | `TestSupportSessionExpiry`、`TestSupportSessionCrossTenantIsolation` |
| 27 | Provider 暂停按能力阻断且不损坏留存财务数据 | 待补 | lifecycle 暂停测试存在，能力级阻断未专项 |
| 28 | Migration dry-run 报告无效身份/客户/订阅/余额 | 已覆盖 | `TestMigrationDryRunValidation` |
| 29 | 中断迁移 resume 无重复身份/订阅 | 部分 | `TestMigrationDuplicateRecordsSkipped`；resume 全链路缺专项 |
| 30 | 失败 cutover 可回滚且无双活跃计费源 | 已覆盖 | `TestMigrationRollback`、`TestMigrationCutoverLock` |
| 31 | Offboarding 导出完整且 checksum 可验证 | 部分 | `TestDataExportRequest` 存在；checksum 验证缺公开契约断言 |
| 32 | 删除只保留法律要求财务记录并产生证据 | 部分 | `TestDeletionProof` 存在；保留边界与法律映射待法务确认 |
| 33 | Analytics 故障不影响身份/额度/计费/支付决策 | 待补 | 架构已隔离，缺故障注入黑盒测试 |
| 34 | 共享 Cell 噪声邻居风暴下 Provider 公平 | 待补 | 无 load/fairness 测试 |
| 35 | Cell 容量 2x 日常峰值 / 3x 账期峰值 | 待补 | 无压测 |
| 36 | Failover 先 fence 再接受 standby 写入 | 已覆盖 | `TestFailoverFullLifecycle`、`TestFailoverDuplicateActive` |
| 37 | 恢复后未确认 Usage/Outbox 正确重放 | 已覆盖 | failover 集成测试含重放断言 |
| 38 | Cell-by-Cell 部署在 SLO 回归后停止 | 待补 | 无部署编排/发布停止测试 |
| 39 | 公共 API 兼容性检查拒绝同 major 内破坏性变更 | 待补 | deprecation middleware 存在，缺 OpenAPI breaking-change checker |
| 40 | 弃用 API 可观测且客户收到迁移窗口 | 部分 | deprecation header/log 存在；指标与迁移指南未完整 |
