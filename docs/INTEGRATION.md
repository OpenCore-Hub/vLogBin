# vLogBin 平台集成接入文档

## 1. 概述

vLogBin 平台提供以下集成方式：

| 集成方式 | 用途 | 协议 |
|---|---|---|
| REST API | 同步操作（客户、订阅、用量等） | HTTPS + JSON |
| Webhook | 事件推送（异步通知） | HTTPS POST + HMAC 签名 |
| Event Stream | 事件拉取（游标分页） | HTTPS GET |
| SCIM 2.0 | 用户自动配置 | HTTPS + JSON (RFC 7644) |
| ZITADEL OIDC | 身份认证 | OAuth 2.0 + OpenID Connect |

## 2. REST API 集成

### 2.1 基础信息

| 环境 | Base URL |
|---|---|
| Production | `https://api.vlogbin.com/v1` |
| Staging | `https://api.staging.vlogbin.com/v1` |

### 2.2 认证

使用 Bearer Token（API Key）认证：

```
Authorization: Bearer vlb_live_xxxxxxxxxxxxxxxx
```

API Key 格式：
- Live 环境：`vlb_live_` 开头
- Test 环境：`vlb_test_` 开头

### 2.3 通用请求格式

```http
POST /v1/customers HTTP/1.1
Host: api.vlogbin.com
Authorization: Bearer vlb_live_xxxxxxxx
Content-Type: application/json
X-Request-ID: optional-uuid-for-tracing

{"external_id": "cust_001", "type": "business", "name": "Acme"}
```

### 2.4 通用响应格式

成功：
```json
{"id": "uuid", "external_id": "cust_001", ...}
```

错误（RFC 7807 风格）：
```json
{
  "error": {
    "code": "validation_error",
    "message": "email is required",
    "request_id": "550e8400-e29b-41d4-a716-446655440000"
  }
}
```

### 2.5 幂等性

所有写请求（`POST`/`PUT`/`PATCH`/`DELETE`）支持 **Stripe 风格 `Idempotency-Key` 头**（1-255 个可打印 ASCII 字符，客户端自行生成）：
- 首次携带该 key 的请求正常执行，其响应（状态码 + 响应体）被缓存；
- 相同 key + 相同身份（Provider 或 Operator）+ 相同方法 + 相同路径的后续请求，直接返回首次响应，并带 `Idempotency-Replayed: true` 头，**不会重复创建资源**；
- 同一 key 的并发重复请求返回 `409`（`error.code = "concurrent"`）；
- 响应缓存默认保留 24h（`IDEMPOTENCY_TTL` 可调），由后台 sweeper 清理过期记录；
- key 按认证身份隔离：不同 Provider 使用相同 key 互不影响；
- 5xx 响应不缓存，客户端可用相同 key 重试。

业务层幂等保持不变，与 HTTP 幂等键互补：
- **Usage 事件**：通过 `transaction_id` 幂等（相同 ID 返回已有记录）
- **Quota 预占**：通过 `reservation_id` 幂等
- **SCIM 用户**：通过 `externalId` 幂等

### 2.6 分页

列表端点支持 `limit` 参数（默认 100，最大 1000）。

事件流使用游标分页：
```
GET /v1/events?cursor=uuid&limit=100
```

响应包含 `next_cursor` 和 `has_more` 字段。

## 3. Webhook 集成

### 3.1 注册 Webhook 端点

```bash
POST /v1/webhooks
{
  "url": "https://your-app.com/webhooks/vlogbin",
  "events": []  // 空数组表示接收所有事件
}
```

### 3.2 Webhook 请求格式

```http
POST /webhooks/vlogbin HTTP/1.1
Host: your-app.com
Content-Type: application/json
X-Webhook-Signature: hex-hmac-sha256
X-Webhook-Timestamp: 1627742400
X-Webhook-Event-Type: customer.created
X-Webhook-Schema-Version: 1.0

{
  "id": "event-uuid",
  "event_type": "customer.created",
  "aggregate_type": "customer_account",
  "aggregate_id": "customer-uuid",
  "data": {...},
  "created_at": "2025-07-31T10:00:00Z"
}
```

### 3.3 签名验证

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "strconv"
)

func VerifyWebhook(payload []byte, signature string, timestamp string, secret string) bool {
    // 1. 检查时间戳（防重放，5分钟窗口）
    ts, _ := strconv.ParseInt(timestamp, 10, 64)
    if time.Now().Unix()-ts > 300 {
        return false
    }

    // 2. 计算签名
    msg := fmt.Sprintf("%s.%s", timestamp, string(payload))
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(msg))
    expected := hex.EncodeToString(mac.Sum(nil))

    // 3. 常量时间比较
    return hmac.Equal([]byte(signature), []byte(expected))
}
```

```python
import hmac, hashlib, time

def verify_webhook(payload: bytes, signature: str, timestamp: str, secret: str) -> bool:
    # 防重放（5分钟窗口）
    if abs(time.time() - int(timestamp)) > 300:
        return False
    
    # 计算签名
    msg = f"{timestamp}.".encode() + payload
    expected = hmac.new(secret.encode(), msg, hashlib.sha256).hexdigest()
    
    return hmac.compare_digest(signature, expected)
```

```javascript
const crypto = require('crypto');

function verifyWebhook(payload, signature, timestamp, secret) {
    // 防重放
    if (Math.abs(Date.now() / 1000 - parseInt(timestamp)) > 300) {
        return false;
    }
    
    const msg = `${timestamp}.${payload}`;
    const expected = crypto.createHmac('sha256', secret).update(msg).digest('hex');
    
    return crypto.timingSafeEqual(Buffer.from(signature), Buffer.from(expected));
}
```

### 3.4 事件类型

完整事件列表见 `docs/asyncapi.yaml`。常用事件：

| 事件类型 | 触发条件 |
|---|---|
| `customer.created` | 新客户创建 |
| `subscription.created` | 新订阅创建 |
| `usage.ingested` | 用量事件接收 |
| `invoice.synced` | 发票从 Lago 同步 |
| `quota.reserved` | 额度预占 |
| `quota.committed` | 额度提交 |
| `support.requested` | 支持会话请求 |
| `team.member_invited` | 团队成员邀请 |
| `budget.alert_exceeded` | 预算告警超限 |

### 3.5 重试策略

- 最多重试 3 次（指数退避）
- 3 次失败后进入死信队列
- 响应 2xx 状态码视为成功
- 响应超时（30 秒）视为失败

## 4. SCIM 2.0 集成

### 4.1 端点

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/scim/v2/Users` | 创建用户 |
| GET | `/scim/v2/Users` | 列表用户 |
| GET | `/scim/v2/Users/{id}` | 获取用户 |
| PUT | `/scim/v2/Users/{id}` | 更新用户 |
| PATCH | `/scim/v2/Users/{id}` | 部分更新 |
| DELETE | `/scim/v2/Users/{id}` | 删除用户 |
| POST | `/scim/v2/Groups` | 创建群组 |
| GET | `/scim/v2/Groups` | 列表群组 |
| PATCH | `/scim/v2/Groups/{id}` | 更新群组（成员管理） |
| DELETE | `/scim/v2/Groups/{id}` | 删除群组 |

### 4.2 认证

使用 Provider API Key（需要 `scim:manage` scope）：

```
Authorization: Bearer vlb_live_xxxxxxxx
```

### 4.3 用户创建示例

```bash
POST /scim/v2/Users
{
  "externalId": "user-001",
  "displayName": "Alice Smith",
  "email": "alice@example.com",
  "active": true
}

# 响应
{
  "schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
  "id": "uuid",
  "externalId": "user-001",
  "userName": "alice@example.com",
  "name": {"displayName": "Alice Smith"},
  "emails": [{"value": "alice@example.com", "primary": true}],
  "active": true,
  "meta": {
    "resourceType": "User",
    "created": "2025-07-31T10:00:00Z",
    "lastModified": "2025-07-31T10:00:00Z"
  }
}
```

### 4.4 PATCH 操作

```bash
PATCH /scim/v2/Users/{id}
{
  "Operations": [
    {"op": "replace", "path": "displayName", "value": "New Name"},
    {"op": "replace", "path": "active", "value": false}
  ]
}
```

### 4.5 IdP 配置

**Azure AD / Entra ID**：
1. 在企业应用中添加 SCIM 端点
2. Tenant URL: `https://api.vlogbin.com/scim/v2`
3. Secret Token: Provider API Key（需 `scim:manage` scope）

**Okta**：
1. 添加 SCIM 2.0 应用
2. Base URL: `https://api.vlogbin.com/scim/v2`
3. API Token: Provider API Key

## 5. Hosted Auth (ZITADEL OIDC) 集成

### 5.1 启用 Hosted Auth

```bash
POST /v1/hosted-auth/setup
{
  "project_name": "Acme Auth"
}
```

### 5.2 获取 OIDC 配置

```bash
GET /v1/hosted-auth/config
```

响应包含 `client_id`、`issuer`、`authorization_endpoint` 等。

### 5.3 OIDC 授权码流程

```
1. 重定向用户到授权端点:
   https://auth.vlogbin.com/oauth/v2/authorize?
     client_id=CLIENT_ID&
     redirect_uri=REDIRECT_URI&
     response_type=code&
     scope=openid+profile+email

2. 用户登录后回调到 redirect_uri，携带 code

3. 交换 token:
   POST https://auth.vlogbin.com/oauth/v2/token
   {
     "grant_type": "authorization_code",
     "code": "CODE",
     "client_id": "CLIENT_ID",
     "client_secret": "CLIENT_SECRET",
     "redirect_uri": "REDIRECT_URI"
   }
```

### 5.4 自定义认证域

```bash
# 注册域名
POST /v1/custom-domains
{"domain": "auth.acme.com"}

# 添加 DNS TXT 记录
# _vlogbin-verify.auth.acme.com TXT "vlogbin-verify-xxxx"

# 验证域名
POST /v1/custom-domains/{id}/verify
```

## 6. 事件流集成

### 6.1 消费事件

```bash
# 第一页
GET /v1/events?limit=100

# 响应
{
  "events": [...],
  "next_cursor": "event-uuid",
  "has_more": true
}

# 下一页
GET /v1/events?cursor=event-uuid&limit=100
```

### 6.2 过滤事件

```bash
# 按事件类型过滤
GET /v1/events?type=customer.created

# 按聚合类型过滤
GET /v1/events?aggregate_type=customer_account
```

### 6.3 消费者实现

```python
import requests

API_KEY = "vlb_live_xxxxxxxx"
BASE_URL = "https://api.vlogbin.com/v1"

def consume_events():
    cursor = None
    while True:
        url = f"{BASE_URL}/events?limit=100"
        if cursor:
            url += f"&cursor={cursor}"
        
        resp = requests.get(url, headers={"Authorization": f"Bearer {API_KEY}"})
        data = resp.json()
        
        for event in data["events"]:
            process_event(event)
        
        if not data["has_more"]:
            break
        
        cursor = data["next_cursor"]

def process_event(event):
    event_type = event["event_type"]
    if event_type == "customer.created":
        # 处理新客户
        pass
    elif event_type == "quota.exceeded":
        # 处理额度超限
        pass
```

## 7. 按量计费集成

### 7.1 设置定价规则

```bash
PUT /v1/metered-pricing-rules
{
  "metric_code": "api_calls",
  "pricing_model": "tiered",
  "base_price_cents": 2,
  "tier_config": [
    {"up_to": 1000, "price_cents": 2},
    {"up_to": 10000, "price_cents": 1},
    {"up_to": 100000, "price_cents": 0.5}
  ],
  "minimum_spend_cents": 100,
  "enabled": true
}
```

### 7.2 设置预算告警

```bash
POST /v1/budget-alerts
{
  "metric_code": "api_calls",
  "budget_cents": 50000,
  "threshold_pct": 80
}
```

## 8. SDK 契约与发布

官方 SDK 采用契约驱动维护：`docs/openapi.yaml` 是公开 API 契约源，
`sdk/operations.yaml` 声明官方支持的操作，`scripts/sync-sdk-operations.py`
生成 `sdk/generated/manifest.json`，再由 `scripts/check-sdk-contract.py`
校验 Go / TypeScript / Python 三语言实现对齐。

```bash
# 生成并更新 SDK 操作清单
python3 scripts/sync-sdk-operations.py

# 校验清单与三语言实现
make contract

# 构建并校验三语言 SDK 与包产物指纹
make sdk

# 发布 dry-run / 正式发布（正式发布需 NPM_TOKEN / PYPI_TOKEN）
scripts/publish-sdks.sh dry-run
scripts/publish-sdks.sh publish
```

包发布流程与版本兼容政策详见 `docs/SDK_RELEASE.md` 和 `docs/API_COMPATIBILITY.md`。

## 9. 测试环境

### 9.1 获取测试 API Key

联系平台操作员创建 Test 环境 Provider，获取 `vlb_test_` 开头的 API Key。

### 9.2 Sandbox 模式

Test 环境的数据不进入 Live 环境（RLS 隔离）：
- Test 环境的计费数据不会同步到 Lago
- Test 环境的 Webhook 不会投递到生产端点
- Test 环境的用量不会计入真实账单

### 9.3 集成测试

```bash
# 使用 Test 环境 API Key 运行集成测试
VLOGBIN_API_KEY=vlb_test_xxxxxxxx \
VLOGBIN_BASE_URL=https://api.staging.vlogbin.com \
./run_integration_tests.sh
```
