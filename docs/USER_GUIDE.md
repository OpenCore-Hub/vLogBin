# vLogBin 平台用户使用教程

## 1. 快速开始

### 1.1 创建 Provider 账户

平台操作员为您创建 Provider 账户后，您将获得：
- **Provider ID**：唯一标识符
- **API Key**：用于认证（格式：`vlb_live_` 或 `vlb_test_` 开头）

### 1.2 认证方式

所有 API 请求需要在 `Authorization` 头中携带 API Key：

```bash
curl -H "Authorization: Bearer vlb_live_xxxxxxxx" \
     -H "Content-Type: application/json" \
     https://api.vlogbin.com/v1/customers
```

### 1.3 第一个客户

```bash
# 创建客户
curl -X POST https://api.vlogbin.com/v1/customers \
  -H "Authorization: Bearer $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "external_id": "cust_001",
    "type": "business",
    "name": "Acme Corporation",
    "email": "billing@acme.com"
  }'

# 响应
# {"id":"uuid","external_id":"cust_001","name":"Acme Corporation",...}
```

### 1.4 发布计费目录

```bash
# 创建计费目录
curl -X POST https://api.vlogbin.com/v1/catalogs \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"version": "2025-07"}'

# 添加计划
curl -X POST https://api.vlogbin.com/v1/catalogs/{id}/plans \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "code": "pro-monthly",
    "name": "Pro Monthly",
    "price_cents": 9900,
    "interval": "monthly"
  }'

# 发布目录
curl -X POST https://api.vlogbin.com/v1/catalogs/{id}/publish \
  -H "Authorization: Bearer $API_KEY"
```

### 1.5 创建订阅

```bash
curl -X POST https://api.vlogbin.com/v1/subscriptions \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "external_id": "sub_001",
    "customer_external_id": "cust_001",
    "catalog_version_id": "catalog-version-uuid",
    "plan_code": "pro-monthly"
  }'
```

### 1.6 接入用量

```bash
curl -X POST https://api.vlogbin.com/v1/usage/ingest \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "transaction_id": "txn_001",
    "customer_external_id": "cust_001",
    "metric_code": "api_calls",
    "timestamp": "2025-07-31T10:00:00Z",
    "properties": {"quantity": 150}
  }'
```

## 2. 团队管理

### 2.1 邀请团队成员

```bash
# 邀请开发者
curl -X POST https://api.vlogbin.com/v1/team-members \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "email": "dev@acme.com",
    "display_name": "Dev User",
    "role": "developer"
  }'

# 响应包含一次性 API Key
# {"member":{...}, "api_key": "vlb_live_yyyyyyyy"}
```

### 2.2 角色权限

| 角色 | 权限 |
|---|---|
| admin | 全部权限（读/写/凭证管理/审计/支持审批/SCIM） |
| billing_admin | 读/写/审计 |
| developer | 读/写 |
| support_agent | 读/审计/支持审批 |

### 2.3 管理团队成员

```bash
# 列出团队成员
curl https://api.vlogbin.com/v1/team-members \
  -H "Authorization: Bearer $API_KEY"

# 变更角色
curl -X PATCH https://api.vlogbin.com/v1/team-members/{id} \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"role": "billing_admin"}'

# 暂停成员
curl -X POST https://api.vlogbin.com/v1/team-members/{id}/suspend \
  -H "Authorization: Bearer $API_KEY"

# 恢复成员（生成新 API Key）
curl -X POST https://api.vlogbin.com/v1/team-members/{id}/reactivate \
  -H "Authorization: Bearer $API_KEY"
```

## 3. 额度管理

### 3.1 设置额度限制

```bash
curl -X PUT https://api.vlogbin.com/v1/subscriptions/{id}/quota-limits/api_calls \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "limit_value": 100000,
    "period_type": "monthly"
  }'
```

### 3.2 预占额度

```bash
# 预占 500 次 API 调用
curl -X POST https://api.vlogbin.com/v1/subscriptions/{id}/quota/reserve \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "quota_key": "api_calls",
    "amount": 500,
    "reservation_id": "res_001"
  }'

# 提交预占
curl -X POST https://api.vlogbin.com/v1/subscriptions/{id}/quota/commit \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"reservation_id": "reservation-uuid"}'

# 释放预占
curl -X POST https://api.vlogbin.com/v1/subscriptions/{id}/quota/release \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"reservation_id": "reservation-uuid"}'
```

## 4. Webhook 配置

### 4.1 注册 Webhook

```bash
curl -X POST https://api.vlogbin.com/v1/webhooks \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "url": "https://your-app.com/webhooks/vlogbin",
    "events": []
  }'
```

### 4.2 验证签名

```python
import hmac, hashlib

def verify_signature(payload, signature, timestamp, secret):
    msg = f"{timestamp}.{payload}".encode()
    expected = hmac.new(secret.encode(), msg, hashlib.sha256).hexdigest()
    return hmac.compare_digest(signature, expected)
```

## 5. 数据分析

### 5.1 查看仪表盘

```bash
curl https://api.vlogbin.com/v1/analytics/dashboard \
  -H "Authorization: Bearer $API_KEY"
```

### 5.2 查看用量明细

```bash
curl "https://api.vlogbin.com/v1/analytics/usage-breakdown?days=30" \
  -H "Authorization: Bearer $API_KEY"
```

### 5.3 异常检测

```bash
curl https://api.vlogbin.com/v1/analytics/anomalies \
  -H "Authorization: Bearer $API_KEY"
```

## 6. 数据导出与删除

### 6.1 导出数据

```bash
curl -X POST https://api.vlogbin.com/v1/data-exports \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"export_type": "full"}'
```

### 6.2 请求删除（获取证明）

```bash
curl -X POST https://api.vlogbin.com/v1/data-deletion \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"reason": "provider offboarding"}'
```

## 7. 错误处理

### 7.1 错误格式

```json
{
  "error": {
    "code": "not_found",
    "message": "resource not found",
    "request_id": "uuid"
  }
}
```

### 7.2 常见错误码

| HTTP 状态码 | error code | 含义 |
|---|---|---|
| 400 | validation_error | 请求参数无效 |
| 401 | unauthorized | API Key 无效或缺失 |
| 403 | forbidden | 权限不足（scope 不匹配） |
| 404 | not_found | 资源不存在 |
| 409 | conflict | 状态冲突 |
| 409 | cutover_locked | 迁移切换锁活跃 |
| 409 | cell_draining | Cell 排空中（写冻结） |
| 422 | quota_exceeded | 额度超限 |
| 429 | rate_limited | 请求频率超限 |
| 500 | internal_error | 服务器内部错误 |

## 8. 限流

| 维度 | 默认限制 |
|---|---|
| 来源 IP（全局兜底，认证前生效）| 6000 req/min（`RL_IP_LIMIT` 可调，`0` 关闭）|
| Provider | 1000 req/min |
| Environment | 500 req/min |
| Credential | 100 req/min |
| Endpoint | 60 req/min |

> per-IP 兜底层防止通过轮换凭证绕过认证后限流，同时保护未认证端点（健康检查、指标）免受裸 DoS。

响应头：
- `X-RateLimit-Limit`：限制总数
- `X-RateLimit-Remaining`：剩余次数
- `X-RateLimit-Reset`：重置时间戳
