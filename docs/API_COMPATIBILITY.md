# API 兼容与弃用生命周期

## 政策

- 同一 major 内只允许兼容性增加；破坏性变更必须创建新 major。
- 破坏性版本至少并行运行 12 个月，Sunset 日期必须晚于 Deprecated 日期 12 个月。
- Webhook/Event payload 通过 `X-Webhook-Schema-Version` 声明 schema 版本。
- 弃用端点必须输出 `Deprecation`、`Sunset`、`Link` 头，并累计 `http_api_deprecated_usage_total{path}`。

## 弃用流程

1. 在 `apps/api/internal/httpapi/deprecation.go` 的 `deprecationRegistry` 登记：
   - `PathPattern`：`METHOD /path` 或 `METHOD /path/*`
   - `DeprecatedAt`：公开弃用日期
   - `SunsetAt`：`DeprecatedAt + 12 个月` 之后
   - `Replacement`：新端点（可选）
2. 发布迁移指南，说明替换端点、请求/响应差异与迁移示例。
3. 持续观察 `http_api_deprecated_usage_total`；非零意味着仍有客户依赖旧端点。
4. Sunset 前至少 30 天发送通知（站内公告 / 邮件 / 文档横幅）。
5. Sunset 到期后移除端点，并在 `api-version` 的 `deprecated_endpoints` 中删除。

## 迁移指南模板

```markdown
## 迁移指南：<旧端点> → <新端点>

- 弃用日期：YYYY-MM-DD
- 移除日期（Sunset）：YYYY-MM-DD
- 替换端点：<method> <path>
- 差异说明：
  - <字段/语义变化>
- 迁移示例：
  - <旧请求>
  - <新请求>
- 兼容窗口：弃用后至少 12 个月
```

## 契约门禁

`make contract` 会校验 OpenAPI 路由覆盖、类型同步与 AsyncAPI 事件覆盖；弃用注册表由
`ValidateDeprecationRegistry` 在启动时强制 12 个月窗口。
