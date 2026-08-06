# Webhook 与事件流对外契约

## Webhook 投递

- 投递头：
  - `X-Webhook-Signature`：`HMAC-SHA256(timestamp || payload)`，hex lowercase
  - `X-Webhook-Timestamp`：Unix 秒
  - `X-Webhook-Event-Type`：outbox `event_type`
  - `X-Webhook-Schema-Version`：事件 schema 版本（当前 `1.0`）
- 语义：at-least-once；消费者必须幂等。
- 重试：5xx / 网络错误按指数退避重试，默认最多 3 次；4xx 视为成功投递（服务端不重试）。
- 死信：重试耗尽后 `webhook_deliveries.status = dead_letter`，保留 `response_status` / `response_body` 供诊断。
- 重放：operator 可将 terminal（`dead_letter` / `failed`）投递重放为 `pending`。
- 校验示例见下方各语言。

## 事件流

- 端点：`GET /v1/events?cursor=&limit=&type=&aggregate_type=`
- 顺序：同一 Provider/Environment 内按 `(created_at, id)` 升序。
- cursor：上一页 `next_cursor`；`has_more=false` 表示当前已到尾部。
- 消费端要求：at-least-once，记录已处理 `transaction_id` / 事件 ID 实现幂等。

## 校验与消费示例

### Node.js

```js
import { createHmac, timingSafeEqual } from "node:crypto";

function verify(secret, timestamp, payload, signature) {
  const expected = createHmac("sha256", secret)
    .update(timestamp + payload)
    .digest();
  const given = Buffer.from(signature, "hex");
  return given.length === expected.length && timingSafeEqual(given, expected);
}
```

### Python

```python
import hashlib
import hmac

def verify(secret: str, timestamp: str, payload: bytes, signature: str) -> bool:
    expected = hmac.new(secret.encode(), timestamp.encode() + payload, hashlib.sha256).digest()
    return hmac.compare_digest(expected, bytes.fromhex(signature))
```

### Go

```go
ok := vlogbin.VerifyWebhookSignatureWithin(
    secret,
    r.Header.Get("X-Webhook-Timestamp"),
    payload,
    r.Header.Get("X-Webhook-Signature"),
    5*time.Minute,
)
```

## 断点续读示例

```text
cursor := ""
for {
    page := stream(cursor, 100)
    for _, event := range page.events {
        if !alreadyProcessed(event.transaction_id) {
            handle(event)
            markProcessed(event.transaction_id)
        }
    }
    if !page.has_more {
        break
    }
    cursor = page.next_cursor
}
```

## 契约门禁

- `make contract` 校验 OpenAPI 路由、类型同步、AsyncAPI 事件覆盖与错误码目录。
- 集成测试覆盖事件流 cursor 无丢失/无重复、跨租户隔离、Webhook 签名/去重/重放/SSRF。
