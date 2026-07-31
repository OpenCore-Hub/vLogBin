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
4. 失败重试 3 次后进入死信队列

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
| PSP 凭证加密 | AES-256-GCM，随机 nonce |
| Webhook 签名 | HMAC-SHA256 + 时间戳防重放 |
| SSRF 防护 | DNS 解析 + IP 范围阻断 + TOCTOU 再检查 |
| 限流 | 四级固定窗口（Provider/Environment/Credential/Endpoint） |
| 删除证明 | HMAC-SHA256 派生密钥签名 |
