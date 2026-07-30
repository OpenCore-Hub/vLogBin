# ZITADEL + Lago 多 Provider SaaS 平台级生产架构设计方案

文档版本：v2.0  
文档状态：架构评审通过，进入分阶段实施设计  
评审日期：2026-07-30  
设计容量：1,000 Provider、1,000 万终端身份、峰值 5,000 Usage EPS  
合规基线：SOC 2 Type II、ISO 27001、GDPR、PCI SAQ A  

---

## 0. 架构评审结论

v1 适合“一个经营主体运营自己的 B2B SaaS”，不适合直接作为面向市场开放的多 SaaS 厂商基础设施。

v2 必须完成以下结构性修改：

1. 增加 Provider 和 Environment 两个一级领域边界；
2. 从单集群升级为 Global Control Plane + Regional Cell；
3. 同时支持 B2B Organization 与 B2C Individual Account；
4. 将平台向 Provider 收费与 Provider 向终端客户收费拆成两个账务域；
5. ZITADEL 与 Lago 退居内部引擎，对外只开放平台稳定 API；
6. 增加目录版本、不可变计量调整、实时硬额度和财务重放能力；
7. 增加厂商准入、迁移、退出、支持访问、数据可携和事件出口；
8. 共享档采用强逻辑隔离，企业档支持独立 Cell 和数据库；
9. 身份、计费、分析和事件按 Provider Home Region 驻留；
10. 目标架构一次设计，能力按阶段启用。

未完成上述修改前，不建议以“开放平台”名义正式商用。

---

## 1. 产品定位

平台为多个彼此独立的 SaaS Provider 提供：

- 身份认证、企业 SSO、MFA、Passkey、SCIM 和委派管理；
- 客户、产品目录、订阅、用量、额度、发票、支付和催收编排；
- Test/Live 环境、API、SDK、Webhook 和企业事件流；
- 托管共享 Cell 与企业专属 Cell；
- 数据驻留、审计、迁移、导出和删除证明。

平台不承担：

- Provider 的会计总账；
- Provider 的 Merchant of Record 责任；
- 任意密码哈希的无损迁移承诺；
- 跨区域身份与账务双主写入；
- 对 ZITADEL 或 Lago 原生管理 API 的公开兼容承诺。

---

## 2. 核心架构原则

### 2.1 对外平台化

外部契约是 Platform Auth API、Billing API、Metering API、Entitlement API 和 Event API。ZITADEL、Lago、PSP、邮件和税务服务均为可替换内部适配器。

### 2.2 强制上下文

每个请求都必须解析以下上下文：

- provider_id；
- environment_id；
- home_region；
- cell_id；
- actor_id；
- customer_account_id；
- credential_id；
- request_id。

provider_id 和 environment_id 必须从已验证凭证推导，不能信任请求体传值。

### 2.3 财务级一致性

系统采用本地事务、Transactional Outbox、至少一次投递、幂等效果、不可变调整和周期性对账，不宣称跨系统 Exactly Once。

### 2.4 单写与可恢复

身份、订阅、目录、用量和发票分别有明确真相源。跨系统流程使用 Saga，不使用分布式数据库事务。

---

## 3. 目标总体架构

### 3.1 全局控制面

Global Control Plane 只保存最少路由和治理元数据：

- Provider Registry；
- Environment Registry；
- Home Region 与 Cell Registry；
- Domain Registry；
- Product/SLA Tier；
- Provider Lifecycle 与风险状态；
- Cell 容量、健康和迁移状态；
- 全局 API 版本及功能开关。

全局控制面不得保存终端密码、完整客户 PII、原始用量或财务明细。

### 3.2 区域 Cell

每个 Regional Cell 包含：

- API Gateway 和 Region Router；
- Auth Facade；
- ZITADEL 集群及独立 PostgreSQL HA；
- Tenant/Customer Account Service；
- Catalog、Subscription、Entitlement 和 Quota Service；
- Metering Ingestion、Outbox Relay 和 Reconciliation Worker；
- Lago API、专用 Worker、Event Processor、PostgreSQL HA 和 Redis HA；
- Provider Commerce 连接器；
- Webhook Inbox/Dispatcher 和 Event Delivery；
- Platform PostgreSQL HA、Redis HA、Kafka；
- 审计、指标、日志和追踪采集。

### 3.3 Cell 类型

共享 Cell：

- 多 Provider 共用运行集群；
- 数据库 RLS、复合租户键和 Provider 级加密上下文；
- Provider 级限流、队列公平调度和熔断；
- 适用于标准版。

专属 Cell：

- 独立 ZITADEL、Lago、数据库、队列和密钥；
- 可配置保留容量和更高 SLA；
- 适用于企业版、强监管或超大 Provider。

---

## 4. 多租户领域模型

推荐层级：

    Platform
    └── Provider
        └── Environment: test | live
            ├── Identity Project
            ├── Customer Account
            │   ├── Individual Account
            │   └── Business Account
            ├── Catalog
            ├── Subscription
            ├── Payment Connection
            └── Event Destination

### 4.1 ZITADEL 映射

- 共享 Cell 内 Provider 对应 ZITADEL Project；
- B2B Business Account 对应 ZITADEL Organization；
- 企业成员通过 Organization Membership 和 Project Grant 获权；
- B2C Individual Account 对应隔离的 User/Subject，不为每个个人创建 Organization；
- Provider 之间禁止按 Email 自动关联用户；
- 对外 Subject 使用 Provider/Client 维度的 pairwise subject；
- 需要独立 issuer、强隔离或独立合规边界的 Provider 进入专属 ZITADEL Instance。

### 4.2 Lago 映射

- Provider Environment 对应受隔离的 Lago Organization/执行空间；
- Customer Account 对应 Lago Customer；
- Subscription 必须绑定具体 Catalog Version；
- Provider 不直接获得 Lago 管理凭证；
- 平台 Billing Adapter 负责翻译、限流、审计和错误归一化。

---

## 5. Test 与 Live 环境

Test 和 Live 必须隔离：

- 凭证；
- issuer 与客户端；
- Customer、Subscription 和 Usage；
- Webhook/Event Destination；
- Catalog 和价格；
- Payment/Tax Connection；
- 配额和审计；
- Lago 与 ZITADEL 映射。

所有外部唯一键的有效范围至少是 provider_id + environment_id + external_id。测试事件不得通过字段标记混入 Live 财务数据。

---

## 6. 身份产品面

### 6.1 开放能力

- OIDC Authorization Code + PKCE；
- Client Credentials 和 private_key_jwt；
- Hosted Login；
- Headless API；
- MFA、Passkey、企业 SAML/OIDC；
- SCIM；
- Provider 品牌与终端企业委派管理；
- SDK 和标准 Discovery/JWKS。

### 6.2 域名和 issuer

- 默认提供 Provider 平台子域；
- 企业档支持经过 DNS 所有权验证的自定义认证域；
- 一个 Provider Environment 对应稳定 issuer；
- Cell 迁移时 issuer 不改变；
- Domain Control Plane 管理证书、域名占用、吊销、接管防护和到期告警。

### 6.3 管理层级

角色分四层：

1. Platform Operator；
2. Provider Administrator；
3. Business Account Administrator；
4. End User。

终端企业管理员可在 Provider 策略范围内管理成员、角色、域名、SSO 和 SCIM。

---

## 7. 开放 API 与开发者平台

### 7.1 凭证

- 生产调用以 OAuth Client Credentials 或 private_key_jwt 为主；
- 支持受限 API Key 作为快速接入方案；
- 凭证严格绑定 Environment；
- 支持 Scope、IP 条件、到期时间、并行换钥和即时撤销；
- 公共客户端仅使用可发布凭证，不得获得管理权限。

### 7.2 契约治理

- OpenAPI 管理同步 API；
- AsyncAPI 管理事件；
- 同一 major 只允许兼容性增加；
- 破坏性版本至少并行 12 个月；
- Webhook payload 包含 schema_version；
- 提供弃用遥测、迁移指南、契约测试和日落通知；
- SDK 从平台规范生成，不从 ZITADEL/Lago 原生规范生成。

### 7.3 限流

至少执行 Provider、Environment、Credential、Endpoint 四级限流，并支持突发额度、并发上限、月度配额和异常封禁。

---

## 8. 双账务域

### 8.1 Provider Commerce

Provider 向其终端 Customer Account 收费：

- Provider 是 Merchant of Record；
- Provider 连接自己的 PSP、税务和电子发票服务；
- 平台负责计量、定价、发票生命周期、付款编排和催收；
- PSP 是支付状态真相源；
- 税务连接器是税额与税号校验真相源；
- Provider 会计系统是总账真相源。

### 8.2 Platform Commerce

平台独立向 Provider 收费：

- 平台套餐；
- 身份 MAU；
- Usage Event 数；
- 发票数；
- 专属 Cell、保留容量和高级合规能力。

Platform Commerce 与 Provider Commerce 必须使用不同账户、权限、目录、发票模板、支付连接、审计和对账流程。

---

## 9. 产品目录与订阅

Provider 通过平台 Catalog API 自助配置指标、套餐和价格。平台验证后翻译到 Lago。

生命周期：

    DRAFT → VALIDATED → PUBLISHED → RETIRED

规则：

- 已发布版本不可原地修改；
- 调价和结构变化创建新版本；
- Subscription 固定绑定 catalog_version_id；
- 迁移通过 effective_at 生效；
- Invoice Line 必须追溯到 metric、price、tax、currency 和 catalog 的精确版本；
- 禁止用显示名称作为业务键。

---

## 10. 计量与纠错

### 10.1 写入链路

业务事实与 usage outbox 在同一业务事务提交。Outbox Relay 发布到 Kafka，Usage Worker 使用固定 transaction_id 投递 Lago。

### 10.2 幂等

- transaction_id 在 Provider Environment 内唯一；
- 重试不得生成新 ID；
- 相同 ID、不同 payload 进入安全告警和人工复核；
- 每个处理阶段保存 payload_hash、attempt 和状态。

### 10.3 纠错

- 已接受的原始事件不可覆盖或删除；
- 使用 reversal 或 adjustment 事件修正；
- 未出账周期允许重新聚合；
- 已出账周期使用贷项、补充账单或下一周期调整；
- 所有计算保留输入版本、汇率和聚合规则。

---

## 11. 权益与实时额度

### 11.1 权益真相源

Platform Entitlement Control Plane 是公开产品真相源。Lago 提供 Subscription 和 Billing 状态，平台根据版本化 Catalog 生成权益。

模型至少包括：

- Entitlement Definition；
- Plan Grant；
- Customer Override；
- Snapshot；
- Evaluation Decision；
- Source Version 和 Validity Window。

### 11.2 硬额度

可选 Quota Service 提供 reserve、commit、release 和过期回收。硬额度账本必须持久化，Redis 只能加速，不能作为唯一真相源。

普通用量使用异步软额度；高价值或严格配额使用预占模型。

---

## 12. Webhook 与事件出口

标准档提供签名 Webhook，企业档增加 EventBridge、SQS、Kafka 等事件目的地。

必须支持：

- 原始请求体验签；
- HMAC/JWT 密钥轮换；
- timestamp 和 replay 防护；
- Inbox/Outbox 去重；
- 指数退避、熔断和 DLQ；
- Provider 级并发隔离；
- 事件游标和按范围重放；
- DNS 重绑定、私网地址和 SSRF 防护；
- 目的地健康状态与暂停；
- 同一 AsyncAPI 契约。

---

## 13. 数据隔离

共享数据库必须：

- 所有主键、外键和唯一键传播 provider_id 与 environment_id；
- PostgreSQL RLS 默认拒绝；
- 每次事务显式设置租户会话上下文；
- 后台任务、对账和支持会话同样经过租户上下文；
- 日志、缓存键、对象存储路径和 Kafka Key 包含隔离维度；
- 使用 Provider 级加密上下文或密钥派生；
- 跨租户自动化测试作为发布阻断项。

企业档可迁移到独立数据库和专属 Cell。

---

## 14. Regional Cell 与灾备

- Provider 创建时选择 Home Region；
- 身份、客户 PII、用量、账单、密钥和审计留在 Home Region；
- Global Control Plane 只保存最少路由元数据；
- 单 Cell 跨三个可用区；
- 同司法地域设置热备 Cell；
- 数据库连续复制和对象存储跨区复制；
- 区域切换必须先执行写 fencing；
- 不采用身份和账务跨区域自动双主；
- 切换后重放未确认 Usage 和 Outbox；
- 灾难期间可冻结发票生成，恢复后重新核对再出账。

建议目标：

| 能力 | SLO | RPO | RTO |
|---|---:|---:|---:|
| 标准身份 API | 99.95% | ≤5 分钟 | ≤60 分钟 |
| 企业身份 API | 99.99% | ≤1 分钟 | ≤30 分钟 |
| 用量接收 | 99.99% | 接近 0 | ≤30 分钟 |
| 权益查询 | 99.99% | ≤5 分钟 | ≤30 分钟 |
| 发票生成 | 周期内 99.99% | ≤5 分钟 | ≤4 小时 |

---

## 15. Provider 生命周期与风险控制

状态：

    REGISTERED
    → TEST_ACTIVE
    → LIVE_REVIEW
    → LIVE_ACTIVE
    → RESTRICTED | SUSPENDED | OFFBOARDING

Live 开通至少验证：

- 邮箱与企业域名；
- 服务条款和数据处理协议；
- 风险评分；
- 自定义域所有权；
- Payment/Tax Connection；
- Webhook 目的地；
- 初始配额；
- 安全联系人。

邮件、短信、事件吞吐、自定义域和支付能力分别授权，不能只使用一个总开关。

---

## 16. 通知与外部连接

- Test 使用平台受限邮件/SMS通道；
- Live 支持 Provider 自带 ESP/SMS；
- 平台托管通道按信誉 Cell、域名、配额和内容策略隔离；
- Payment、Tax、Email、SMS Connection 均为 Provider Environment 级资源；
- 凭证保存在 KMS/Vault；
- 外部连接器独立限流、熔断和重试；
- 平台避免接触完整卡数据，维持 PCI SAQ A 边界。

---

## 17. 存量迁移与退出

### 17.1 迁入

Migration Plane 提供：

- Schema 校验和 dry-run；
- 用户、组织、成员、客户、订阅和余额批量导入；
- JIT 身份迁移或外部 IdP 过渡；
- 断点续传和差异报告；
- 双写观察窗口；
- 切换锁和回滚；
- 迁移审计。

### 17.2 退出

Provider Offboarding 流程：

1. 冻结新增写入；
2. 完成最终计量和账单；
3. 生成身份关系、配置、用量、账单和审计导出；
4. Provider 校验导出；
5. 撤销凭证、域名和事件目的地；
6. 删除身份和非留存业务数据；
7. 财务数据按法定周期保留；
8. 备份到期后清除；
9. 生成删除证明。

身份凭证和私钥不得明文导出。

---

## 18. 分析平面

身份审计、Usage、Subscription、Invoice 和 Payment 事件投影到 ClickHouse 或数据湖。

分析平面用于：

- Provider Dashboard；
- MAU、转化、流失和收入分析；
- Usage 明细和成本分析；
- CSV/Parquet 导出；
- 异常计费检测；
- 平台自身计费计量。

分析数据是可重建派生数据，不参与 Token 验证、硬额度、发票金额或支付状态裁决。

---

## 19. 支持访问与审计

禁止静默 impersonation 和永久全租户后台。

Support Access Plane 必须：

- 使用 JIT 临时授权；
- 由 Provider 管理员批准，紧急情况双人授权；
- 限定 Provider、Environment、Customer、Scope 和时长；
- 全程记录查询和修改；
- 高危操作再次确认；
- 将支持会话记录开放给 Provider；
- 到期自动撤销。

审计日志进入追加写或 WORM 存储，并支持 Provider 事件流和 SIEM 导出。

---

## 20. 安全与合规

### 20.1 认证和密钥

- OIDC Authorization Code + PKCE；
- Refresh Token Rotation；
- Workload Identity；
- KMS/Vault；
- 短期凭证优先；
- 密钥双版本轮换；
- Token、密码、API Key 和支付信息禁止进入日志。

### 20.2 网络

- 默认拒绝 NetworkPolicy；
- Lago、数据库、Redis、Kafka 不暴露公网；
- Provider Webhook 出站经 Egress Gateway；
- 管理面使用零信任访问和强 MFA；
- 生产与非生产使用独立云账号、VPC、密钥和数据。

### 20.3 合规证据

- 资产与数据清单；
- 季度访问复核；
- 供应商风险管理；
- 变更审批与发布证据；
- 事件响应和灾备演练；
- DPA 和子处理商清单；
- 数据主体请求和删除证明；
- SBOM、镜像签名、SAST、依赖和 IaC 扫描；
- ZITADEL/Lago AGPL 使用方式经法务确认。

---

## 21. 容量与公平调度

24 个月目标：

- 1,000 Provider；
- 1,000 万终端身份；
- 峰值 5,000 Usage EPS；
- 多 Cell 横向扩展。

每个 Cell 必须定义：

- Provider 数上限；
- 身份和组织数上限；
- 登录 QPS；
- Usage EPS 与事件保留；
- 发票周期峰值；
- Kafka 分区和数据库容量；
- 热 Provider 识别阈值；
- Cell 拆分与迁移水位。

共享 Cell 使用 Provider 级公平队列、并发舱壁、突发额度和熔断。企业档提供保留容量和专属 Cell。

---

## 22. 可观测性与对账

统一传播：

- trace_id；
- request_id；
- provider_id；
- environment_id；
- cell_id；
- actor_id；
- customer_account_id；
- subscription_id；
- transaction_id；
- external_event_id。

核心告警：

- 跨 Provider 访问；
- 重复收费或发票；
- transaction_id payload 冲突；
- Outbox 最大滞留；
- Kafka Lag；
- 身份、目录、订阅和权益投影漂移；
- 支付与发票金额差异；
- Webhook/Event Destination 健康度；
- Cell 容量和热租户；
- 证书、域名与密钥到期。

每小时增量、每日全量对账：

- Provider/Environment ↔ ZITADEL Project/Instance；
- Customer Account ↔ Organization/User ↔ Lago Customer；
- Catalog Version ↔ Lago 执行配置；
- Subscription ↔ Entitlement Snapshot；
- Usage Accepted ↔ Aggregation；
- Invoice ↔ PSP Payment；
- Platform Commerce Provider Usage ↔ Provider Invoice。

---

## 23. 发布与升级

- GitOps；
- Preview、Staging、Canary 5%、25%、100%；
- OpenAPI/AsyncAPI 兼容性检查；
- 数据库 Expand-Migrate-Contract；
- 底层 ZITADEL 与 Lago 分开升级；
- 升级前使用生产脱敏副本演练；
- 升级后执行身份、开户、目录、用量、额度、发票、支付和事件回归；
- Cell 逐个升级，不全局同时发布；
- 失败自动停止后续 Cell。

---

## 24. 分阶段交付

### Phase 0：架构骨架

- Provider、Environment、Region、Cell 领域模型；
- 平台 API 契约；
- RLS 和租户上下文；
- Test/Live 分离；
- 双账务域边界；
- Outbox/Inbox、审计和密钥体系。

### Phase 1：共享 Cell 商用闭环

- Hosted Auth、OIDC、B2B/B2C；
- Catalog、Subscription、Usage、Invoice；
- Provider 自有 PSP；
- Webhook；
- 权益快照；
- Provider Sandbox 与 Live 审核；
- 基础迁移、对账、备份和 Runbook。

### Phase 2：企业能力

- 企业 SSO、SCIM、委派管理；
- 自定义认证域；
- 硬额度；
- 自带邮件/SMS；
- 企业事件流；
- 分层 SLA 和保留容量；
- 标准迁移工具。

### Phase 3：区域与专属能力

- 多 Regional Cell；
- 同地域热备；
- 专属 Cell；
- Cell 迁移；
- 数据驻留；
- 完整导出、退出与删除证明。

### Phase 4：规模化运营

- 独立分析平面；
- 平台按 MAU/Usage/Invoice 收费；
- 异常计费检测；
- 自动热租户迁移建议；
- FinOps 和容量预测。

---

## 25. 生产上线 P0 门禁

- [ ] Provider 与 Environment 是所有资源的强制隔离边界；
- [ ] Test 数据不可能进入 Live 发票；
- [ ] ZITADEL/Lago 管理 API 未直接公开；
- [ ] Provider 之间不能按 Email 自动关联身份；
- [ ] Platform Commerce 与 Provider Commerce 完全分离；
- [ ] Catalog、Metric、Price 和 Invoice Line 可版本追溯；
- [ ] Usage 幂等、冲销、调整和出账后修正已验证；
- [ ] 权益只有一个公开真相源；
- [ ] 硬额度使用持久化预占账本；
- [ ] RLS、后台任务和支持会话跨租户测试通过；
- [ ] Webhook 验签、去重、重放和 SSRF 防护通过；
- [ ] Provider 自有 PSP 凭证隔离和换钥验证；
- [ ] PITR 与同地域恢复演练通过；
- [ ] 每小时对账、DLQ 和财务差异处置启用；
- [ ] Live Provider 风险审核和能力授权启用；
- [ ] JIT Support Access 和 WORM 审计启用；
- [ ] 公开 API 12 个月兼容政策生效；
- [ ] SOC2、ISO27001、GDPR 控制证据开始持续采集；
- [ ] AGPL 与第三方许可证评审完成；
- [ ] P0/P1 Runbook 和 On-call 演练完成。

---

## 26. 最终技术决策

| 决策 | 结论 |
|---|---|
| 平台定位 | 多 SaaS Provider 身份与计费基础设施 |
| 拓扑 | Global Control Plane + Regional Cells |
| 隔离 | 共享 Cell + 企业专属 Cell |
| 身份映射 | Provider Project；B2B Organization；B2C User |
| 身份产品面 | Platform Auth API + 标准 OIDC |
| 环境 | Test/Live 严格隔离 |
| 计费产品面 | 受治理 Platform Billing API |
| 收款主体 | Provider 自收款 |
| 商业模型 | Provider Commerce + Platform Commerce |
| 目录 | 不可变发布版本 |
| 计量 | 不可变事件 + Adjustment/Reversal |
| 权益 | Platform Entitlement Control Plane |
| 配额 | 可选强一致 Quota Service |
| API 凭证 | OAuth 为主，兼容受限 API Key |
| 事件出口 | Webhook + 企业事件流 |
| 数据隔离 | RLS 共享库 + 企业专属库 |
| 地域 | Home Region 驻留、同地域热备单写 |
| 分析 | 独立可重建 Analytics Plane |
| 准入 | Sandbox 开放、Live 审核 |
| 迁移 | 标准迁入、导出与受控删除 |
| 合规 | SOC2 Type II + ISO27001 + GDPR + PCI SAQ A |
| 交付 | 目标架构一次设计，能力分阶段启用 |

---

## 27. 评审状态

结论：有条件通过。

v1 中关于幂等、Outbox、Webhook Inbox、数据库 HA、备份、可观测性和发布门禁的原则继续保留；单 SaaS 租户模型、单账务域、固定三数据库和单集群总体图已由本版本替代。

进入详细设计前必须输出以下配套文档：

1. Provider/Environment/Customer Account 领域模型；
2. Cell Routing 与迁移协议；
3. Auth/Billing/Metering OpenAPI；
4. AsyncAPI 与事件兼容策略；
5. RLS 与跨租户安全测试规范；
6. Catalog、Usage Adjustment 与 Invoice 可重放规范；
7. Platform Commerce/Provider Commerce 对账规范；
8. Regional DR Runbook；
9. Provider Live 准入和 Offboarding Runbook；
10. 24 个月容量与成本模型。
