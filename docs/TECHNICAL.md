# vLogBin 平台技术开发文档

## 1. 项目概述

vLogBin 是一个多 Provider 身份和计费基础设施平台，基于 ZITADEL（身份）和 Lago（计费）构建，提供完整的 Provider 生命周期管理、计费核心链路、企业功能和多区域部署能力。

## 2. 技术栈

| 组件 | 技术 | 版本 |
|---|---|---|
| 后端语言 | Go | 1.25 |
| HTTP 路由 | chi | v5 |
| 数据库 | PostgreSQL | 16 |
| 数据库迁移 | goose | v3 |
| SQL 代码生成 | sqlc | v1.27 |
| 连接池 | pgx/v5 | v5 |
| 日志 | slog | stdlib |
| 加密 | AES-256-GCM | stdlib |
| 测试 | testcontainers-go | — |
| 容器 | Docker (multi-stage) | alpine 3.20 |

## 3. 项目结构

```
apps/api/
├── cmd/server/
│   └── main.go                    # 应用入口、依赖注入、优雅关闭
├── internal/
│   ├── config/                    # 配置管理（环境变量）
│   ├── crypto/                    # AES-256-GCM 加密器
│   ├── domain/                    # 领域模型、常量、验证函数
│   ├── httpapi/                   # HTTP 层
│   │   ├── handlers_*.go          # 请求处理器（按功能域分组）
│   │   ├── middleware.go          # 中间件链
│   │   ├── deprecation.go         # API 弃用中间件
│   │   └── httpapi.go             # 路由注册、错误处理
│   ├── keys/                      # API Key 生成和哈希
│   ├── outbox/                    # 事务性 Outbox 中继
│   ├── ratelimit/                 # 固定窗口限流
│   ├── service/                   # 业务逻辑层
│   │   ├── service.go             # Service 结构体、Option 模式
│   │   ├── auth.go                # Hosted Auth (ZITADEL)
│   │   ├── billing_*.go           # 计费核心
│   │   ├── support.go             # JIT Support Access
│   │   ├── quota.go               # 硬额度账本
│   │   ├── team.go                # 委派管理
│   │   ├── events.go              # 事件流
│   │   ├── migration.go           # 迁移工具
│   │   ├── custom_domains.go      # 自定义认证域
│   │   ├── notifications.go       # 邮件/SMS 配置
│   │   ├── sla_tiers.go           # 分层 SLA
│   │   ├── scim.go                # SCIM 2.0
│   │   ├── data_export.go         # 数据导出/删除证明
│   │   ├── cells.go               # Cell 管理
│   │   ├── cell_failover.go       # 热备故障切换
│   │   ├── cell_migration.go      # Cell 迁移
│   │   ├── analytics.go           # 分析平面
│   │   └── metered_billing.go     # 按量计费
│   ├── store/                     # 数据访问层
│   │   ├── store.go               # Store 结构体、WithTenant/WithOperator
│   │   └── storegen/              # sqlc 生成的代码
│   ├── tenant/                    # 租户上下文
│   ├── webhook/                   # Webhook 投递
│   ├── zitadel/                   # ZITADEL OIDC + Management API
│   └── integration/               # 集成测试
├── db/
│   ├── migrations/                # 数据库迁移（0001-0025）
│   └── queries/                   # SQL 查询定义
├── sqlc.yaml                      # sqlc 配置
├── Dockerfile                     # 多阶段构建
└── go.mod
```

## 4. 架构设计

### 4.1 分层架构

```
HTTP Layer (httpapi)
    ↓ chi router + middleware
Service Layer (service)
    ↓ Option pattern + WithTenant/WithOperator
Store Layer (store)
    ↓ pgx/v5 connection pool
PostgreSQL (RLS-enforced)
```

### 4.2 租户隔离（RLS）

所有多租户表使用 Row Level Security 双因子隔离：

```sql
CREATE POLICY tenant_isolation ON table_name
    USING (
        current_setting('app.is_operator', true) = 'on'
        OR (provider_id::text = current_setting('app.provider_id', true)
            AND environment_id::text = current_setting('app.environment_id', true))
    );
```

- `WithTenant`：设置 `app.provider_id` + `app.environment_id`（Provider 上下文）
- `WithOperator`：设置 `app.is_operator = 'on'`（平台操作员上下文）

### 4.3 事务性 Outbox

所有状态变更通过 Outbox 模式保证事件投递：

1. 业务操作 + outbox 事件 + 审计日志在同一事务中
2. 后台 relay 协程提取 pending 事件并投递
3. Webhook worker 签名并投递到 Provider 端点
4. 失败重试（指数退避，默认 5 次上限）后进入死信队列

**死信队列（0033 正式化，P0 门禁"DLQ 启用"）**：死信是**真终态**——`status='dead_letter'` + `last_error` 记录最终失败原因（`outbox_events.status` CHECK 含 `dead_letter`；`0033` 同时把历史软死信 `status='failed' AND next_attempt_at IS NULL` 升级为真状态，使对账立即可见）：

- **入口**：① max attempts 耗尽（`retryOrDeadLetter`，主路径）；② payload 合法 JSON 但无法 unmarshal 为 `billing.UsageEvent`（`payload` 列是 jsonb，非法 JSON 在存储层即被拒绝，故该路径只可能来自 schema 不兼容——立即死信，重试无意义）
- **终态保证**：死信行**不重放**（`CountUnconfirmedOutbox` 只计 pending/failed，failover 排除死信）、**不 claim**（`ClaimDueOutboxEvents` 条件天然排除）、**保留期内不删**、超过保留窗口由 `DeleteExpiredOutboxEvents` 清理（`DeleteExpiredWebhookDeliveries` 先清理引用它的投递记录）
- **完整性信号**：`CountDeadLetterOutbox`（reconciliation `outbox_dead_letter` check）+ 指标 `outbox_dead_letter_total`（累计死信速率）。**此 check 曾因软死信而恒 0（形同虚设）**——0033 后任何死信都立即可见并触发告警；webhook 侧 `webhook_deliveries` 自 0009 起即用真 `dead_letter` 状态

### 4.4 Option 模式

Service 通过 Option 函数配置可选依赖：

```go
svc := service.New(store, baseDomain,
    service.WithBillingAdapter(adapter),
    service.WithURLValidator(webhook.ValidateURL),
    service.WithCryptoEncryptor(encryptor),
    service.WithDNSResolver(resolver),
    service.WithZITADELManagement(mgmtClient, issuer),
)
```

### 4.5 审计哈希链（Tamper-Evident Audit Log）

审计事件是合规证据，采用**哈希链**实现篡改可检测（`0031_audit_hash_chain.sql`）：

1. **追加即哈希**：`audit_events_hash` BEFORE INSERT 触发器在应用层写入之前计算 `prev_hash`/`event_hash`（SHA-256，`audit_event_hash(prev, row)` 规范化 12 字段，0x1f 分隔）；应用层无法伪造链，因为每行哈希都依赖前一行
2. **链串行化**：`audit_chain_tail` 单行尾指针（`CHECK (id=1)`），触发器 `FOR UPDATE` 锁尾行后追加，并发插入不会 fork
3. **保留边界**：0031 替换的 `purge_audit_events` 记录 `pruned_through_id`，链头前移；验证从**幸存首行**重新锚定，跨保留窗口仍可验证
4. **验证**：`audit_chain_verify(from, to)`（operator-only，SECURITY DEFINER + `app.is_operator='on'`）重算每行哈希，返回损坏行 id（`broken_at`）与原因；HTTP：`GET /v1/operator/audit/chain/verify`
5. **锚点**：`anchor_audit_chain(operator)` 记录尾指针到 `audit_chain_anchors`，`events_covered` 为增量；`audit-chain-anchor-sweeper` 按 `AUDIT_CHAIN_ANCHOR_INTERVAL`（默认 24h）周期锚定；HTTP：`GET /v1/operator/audit/chain`（尾+最后锚点）、`POST /v1/operator/audit/chain/anchor`

**诚实性边界**：DB 内链是 tamper-evident（篡改可检测）而非 tamper-proof（不可篡改）——攻击者拿到 DB 写权限仍可改行，但 verify 必然报警。完整防篡改由**外部 WORM 锚定**补齐（§4.9）：锚点导出到不可变对象存储后，篡改链的任何行都会与已归档的不可变锚点分叉，tamper-evident 升级为 tamper-proof。

### 4.6 Worker 监督（panic 恢复 + 指数退避重启）

平台运行 13+ 后台 worker（outbox relay、webhook 投递、各 sweeper…）。此前任何 worker 内未捕获的 panic 都会**静默终止该 goroutine**，worker 永久停摆，只在指标陈旧后暴露为事故。`internal/worker/supervisor.go` 统一解决（`cmd/server/main.go` 全部 worker 注册经 `sup.Run(ctx, name, fn)`）：

1. **panic 恢复**：`recover` 捕获后记录 `worker panicked` 日志（含 `debug.Stack` 全栈）+ `worker_panics_total{name}` 指标，然后走重启路径
2. **重启语义**：**ctx 取消是唯一优雅退出**；`fn` 返回（无论 error 还是 nil）而 ctx 仍存活一律视为异常退出，按指数退避重启（`worker_restarts_total{name, reason}`，reason ∈ `panic`/`exit`）——worker 契约统一为"run 直到被关闭"
3. **退避策略**：`BackoffInitial`(1s) × `BackoffFactor`(2) 递增、`BackoffMax`(30s，`WORKER_BACKOFF_MAX` 可配) 封顶；worker 存活 ≥ `ResetAfter`(5m) 重置退避，单次偶发崩溃不会把 worker 永久钉在高退避
4. **关闭兼容**：supervisor 关闭 `done` channel，`stopWorkers` 的并发等待逻辑零改动；退避期间 ctx 取消立即停止（`sleep` select ctx.Done）
5. **pprof 特例**：stop 钩子先 cancel ctx 再 `Shutdown`，监听返回时 ctx 已取消 → 优雅退出不误触发重启；真实 serve 错误（端口冲突等）仍照常重启

指标可直接告警：`worker_restarts_total` 持续增长 = 崩溃循环，`worker_panics_total` 突增 = 代码缺陷。

### 4.7 PSP 主密钥轮换（Master Key Rotation）

PSP 凭证用 AES-256-GCM 加密（`internal/crypto`）。主密钥一旦泄露必须能轮换，且**不能丢失任何存量凭证**。`Encryptor` 因此持有两类密钥：

1. **active 密钥**（`PSP_MASTER_KEY`）：所有新加密只用它——新密文永远能被当前密钥解读，避免"新数据用旧密钥封存"的隐患
2. **previous 密钥列表**（`PSP_MASTER_KEY_PREVIOUS`，逗号分隔）：仅用于解密回退。`Decrypt` 先试 active，GCM 认证失败后按序试各 previous，全部失败才报错

**密文格式零变化**（base64(nonce ‖ sealed)），轮换不需要任何数据迁移：

```
轮换前：PSP_MASTER_KEY=old
轮换后：PSP_MASTER_KEY=new，PSP_MASTER_KEY_PREVIOUS=old
```

存量密文经 previous 密钥解密成功时递增 `credential_decrypt_fallback_total`——该指标持续非零即表明仍有旧密钥数据在被读取，**排期重加密收敛**（重新写入凭证使其改用 active 密钥封存），归零后即可从配置移除旧密钥，完成整轮轮换。previous 密钥在构造时严格校验（非 hex / 非 32 字节立即报错启动失败），防止笔误静默弃用全部存量密文。

**收敛闭环（重加密 worker）**：fallback 指标只告警，真正收敛靠 `internal/service/reencrypt.go` 的后台 worker（`REENCRYPT_SWEEP_INTERVAL` 启用，`NewReencryptionSweeper` 接入候选 21 的监督框架）。它以 **operator 上下文**分批（`REENCRYPT_BATCH_SIZE`，默认 100 行/表/事务）扫描三张加密表（`psp_credentials`、`notification_configs`、`provider_auth_configs`）：

- 每行经 `NeedsReencryption` **纯检测**（active 可解 → 跳过；previous 可解 → 解密+active 重封存；全失败 → 计为不可收敛行跳过）
- worker 用 `DecryptWithoutFallback` 解密，**不触发** `credential_decrypt_fallback_total`——该指标保持"请求路径仍在读旧密文"的纯净语义
- 指标：`credentials_reencrypted_total{table}`（收敛进度）+ `credentials_reencrypt_errors_total{table}`（不可收敛行——密钥丢失或数据损坏，**唯一阻塞完整收敛的信号**，需人工介入）
- **幂等**：二次扫描对已收敛行零变更，不抖写；`psp_credentials` 更新用 `NULLIF` 保证 NULL webhook 密文写回仍为 NULL

完整轮换流程：轮换（`PSP_MASTER_KEY`=新、`PSP_MASTER_KEY_PREVIOUS`=旧）→ 启用 worker → `credentials_reencrypted_total` 增长至停 → `credential_decrypt_fallback_total` 归零 → 从 `PSP_MASTER_KEY_PREVIOUS` 移除旧密钥并重启 → 收敛完成。

### 4.8 Readiness 依赖健康汇总

`/ready` 是 K8s 切流依据：200 才把流量打到 pod。初版只 ping DB，存在运维盲区——限流后端为 Redis 时，Redis 故障下限流 **fail-open**（请求放行但无限流保护），而 `/ready` 依旧 200，LB 不会摘除该实例，故障被静默吞掉。

`ready` handler 因此升级为**依赖聚合**：

```
/ready → 并行检查 → 200 ready | 503 unavailable
  ├─ database   (必查，store.Ping)
  └─ ratelimit  (接口断言 dependencyPinger，内存恒 up，Redis 真实往返)
```

- 每次检查受 `readyTimeout`（默认 2s）总超时兜底；依赖间并行，互不拖累
- 响应 body 携带结构化明细（仅服务人类排障）：`dependencies.{name}.{status, latency_ms, error?}`；HTTP 状态码语义不变，探针零感知
- 指标 `readiness_checks_total{dependency,status}` 让 ready 抖动可归属告警：`sum by (dependency) (rate(readiness_checks_total{status="down"}[5m])) > 0` 即定位到具体依赖
- Redis 探测失败不触发限流 onErr 回调——Ping 是健康语义，与请求路径的调用失败严格区分

### 4.9 WORM 审计归档（外部锚定，Tamper-Proof Audit Chain）

§4.5 的哈希链在 DB 内是 tamper-evident：`audit_chain_verify` 必然报警，但**不能阻止**拿到 DB 写权限的攻击者篡改后快速恢复。`0032_audit_anchor_archive.sql` + `internal/archive` + `internal/service/audit_archive.go` 把每个锚点（`tail_event_id` / `tail_hash` / operator / 覆盖范围）发布到 **S3 兼容 WORM 对象存储**（MinIO 或 AWS S3 object lock bucket）——归档副本不可变，DB 超级用户也无法改写，tamper-evident 升级为 **tamper-proof**：

1. **迁移**：`audit_chain_anchors` 加 `published_at timestamptz` / `object_key text` + 部分索引 `idx_audit_chain_anchors_unpublished ON (id) WHERE published_at IS NULL`；`platform_app` 授予 SELECT, UPDATE（UPDATE 仅用于 mark 已发布，无 DELETE）
2. **发布协议（崩溃安全 + 幂等）**：
   - ① 短只读事务取一批 `published_at IS NULL` 锚点（`ListAuditAnchorsForPublish`，`AUDIT_ARCHIVE_BATCH_SIZE` 默认 100）
   - ② **事务外**上传对象（确定性 key `audit/anchors/{anchor_id}.json`，含 sha256 自校验字段）——上传失败不触碰 DB
   - ③ 短事务 `UPDATE ... SET published_at = now(), object_key = $2 WHERE id = $1 AND published_at IS NULL`（`MarkAuditAnchorPublished`）——若命中 0 行说明并发已发布，计 `already_published` 跳过
   - **崩溃恢复 = 重传同一 key**（S3 PUT 幂等）+ 补 mark；绝无"对象已传但 DB 未标"或"DB 已标但对象缺失"的半状态
3. **worker**：`audit-archiver` 经 `NewAuditArchiveSweeper` 接入候选 21 监督框架，`AUDIT_ARCHIVE_SWEEP_INTERVAL` 启用（默认 0 = 禁用，需显式配置四要素）；每轮 sweeper 完成即上报计数
4. **指标**：`audit_anchors_published_total{result}`（published / already_published，已发布计数是外部锚定管线的**完整性信号**）+ `audit_archive_errors_total{op}`（list / upload / mark）。告警：`audit_anchors_published_total{result="published"}` 停止增长而锚点持续产生 = 归档落后或对象存储不可用；`audit_archive_errors_total{op="upload"}` 非零 = 凭证/桶锁策略/网络问题
5. **配置**：`AUDIT_ARCHIVE_S3_{ENDPOINT,BUCKET,ACCESS_KEY,SECRET_KEY,REGION,USE_SSL}`（详见 DEPLOYMENT.md）；启用归档时四要素缺失则**启动失败**（fail-closed，不静默降级）

**安全边界**：归档对象一旦写入 WORM 桶即不可删除/覆盖（桶必须开启 object lock / retention）；`audit_chain_anchors` 本身仍可被 operator 追加（正常锚定），但**不可篡改历史锚点**——外部副本与之比对即暴露。Archiver 只做 PUT，无需读/列权限。

## 5. 开发流程

### 5.1 添加新功能

1. **创建数据库迁移**：`db/migrations/00XX_feature.sql`
2. **编写 SQL 查询**：`db/queries/feature.sql`（sqlc 注释格式）
3. **生成代码**：`sqlc generate`
4. **添加领域常量**：`internal/domain/lifecycle.go`
5. **实现服务层**：`internal/service/feature.go`
6. **实现 HTTP 处理器**：`internal/httpapi/handlers_feature.go`
7. **注册路由**：`internal/httpapi/httpapi.go`
8. **编写集成测试**：`internal/integration/feature_test.go`

### 5.2 sqlc 查询格式

```sql
-- name: CreateFeature :one
INSERT INTO features (name, value) VALUES ($1, $2)
RETURNING *;

-- name: GetFeatureByID :one
SELECT * FROM features WHERE id = $1;
```

### 5.3 迁移格式

```sql
-- +goose Up
CREATE TABLE ...;
-- +goose StatementBegin
DO $$ BEGIN ... END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS ...;
```

### 5.4 测试

```bash
# 运行全部测试
go test ./... -count=1 -timeout 180s

# 运行特定功能测试
go test ./internal/integration/ -run TestFeature -v

# 运行测试环境
docker-compose -f docker-compose.test.yml up -d
```

## 6. 编码规范

1. **错误处理**：使用 `mapErr` 转换 pgx 错误，使用 `errors.Is` 检查
2. **租户校验**：使用 `checkTenantOwnership` helper
3. **状态变更**：必须同时调用 `emitOutboxTx` + `insertAuditTx`
4. **空切片**：JSON 响应中空数组返回 `[]` 而非 `null`
5. **RLS 策略**：使用 `::text` 比较（不用 `::uuid`），包含 `WITH CHECK`
6. **权限**：`GRANT SELECT, INSERT, UPDATE, DELETE` 给 `platform_app`
7. **Scope**：使用 `requireScope` 中间件保护端点
8. **Sweeper**：使用通用 `ExpirySweeper` 而非重复结构体
9. **状态机**：使用 `requireStatus` helper 校验状态转换

## 7. 安全设计

| 机制 | 实现 |
|---|---|
| API Key 认证 | SHA-256 哈希存储，前缀用于查找 |
| OIDC 验证 | RS256 JWT + JWKS 缓存 + 5 分钟节流 |
| PSP 凭证加密 | AES-256-GCM，随机 nonce；主密钥可轮换（active 加密 + previous 回退解密，存量密文零迁移） |
| Webhook 签名 | HMAC-SHA256 + 时间戳防重放 |
| SSRF 防护 | DNS 解析 + IP 范围阻断 + TOCTOU 再检查 |
| 限流 | 四级固定窗口（Provider/Environment/Credential/Endpoint）+ 认证前 per-IP 全局兜底；内存 / Redis（`RATE_LIMIT_BACKEND`）双后端，Redis 后端 Lua 原子窗口 + 故障 fail-open |
| 删除证明 | HMAC-SHA256 派生密钥签名 |
