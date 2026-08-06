# vLogBin API 错误码目录

> 所有错误响应使用统一信封：`{"error": {"code", "message", "request_id", "retry_after?"}}`。
> 新增错误码时必须同步本目录；`make contract` 会校验覆盖。

| 错误码 | HTTP | 触发条件 | 客户端建议 |
|---|---|---|---|
| `cell_draining` | 409 | Cell 正在排空，拒绝写入 | 重试前等待迁移完成 |
| `cidr_not_allowed` | 403 | API Key 来源 IP 不在允许 CIDR | 检查网络出口或更新 CIDR |
| `concurrent` | 409 | 相同幂等键请求正在执行 | 等待首次请求完成后重试 |
| `conflict` | 409 | 资源状态冲突 | 读取最新状态后重试 |
| `credential_expired` | 401 | API Key 已过期 | 轮换或重新签发凭证 |
| `credential_revoked` | 401 | API Key 已吊销 | 使用有效凭证 |
| `cutover_locked` | 409 | 迁移 cutover 锁激活 | 等待迁移完成或回滚 |
| `domain_taken` | 409 | 自定义域名已被占用 | 更换域名 |
| `environment_mismatch` | 400 | `X-Environment` 与凭证环境不符 | 使用与凭证一致的环境 |
| `forbidden` | 403 | 无权限执行操作 | 检查角色/scope |
| `insufficient_scope` | 403 | 凭证缺少所需 scope | 重新签发含所需 scope 的 Key |
| `internal` | 500 | 服务端内部错误 | 使用 request_id 联系平台支持 |
| `invalid_capability` | 400 | 能力标识非法 | 使用合法 capability |
| `invalid_cell_id` | 400 | Cell ID 非法 | 传入合法 UUID |
| `invalid_credential_id` | 400 | 凭证 ID 非法 | 传入合法 UUID |
| `invalid_cursor` | 400 | 分页游标非法 | 使用 API 返回的 cursor |
| `invalid_customer` | 400 | 客户标识非法 | 检查 customer external_id |
| `invalid_domain_id` | 400 | 域名 ID 非法 | 传入合法 UUID |
| `invalid_duration` | 400 | 时长参数非法 | 传入正数时长 |
| `invalid_env` | 400 | `env` 参数不是 test/live | 使用 test 或 live |
| `invalid_environment_id` | 400 | 环境 ID 非法 | 传入合法 UUID |
| `invalid_format` | 400 | 导出格式非法 | 使用 csv 或 json |
| `invalid_from` | 400 | 时间起点非法 | 使用 RFC3339 |
| `invalid_from_cell_id` | 400 | 源 Cell ID 非法 | 传入合法 UUID |
| `invalid_id` | 400 | 路径 ID 非法 | 传入合法 UUID |
| `invalid_idempotency_key` | 400 | 幂等键格式非法 | 使用 1-255 可打印 ASCII |
| `invalid_json` | 400 | 请求体不是合法 JSON | 修复 JSON 后重试 |
| `invalid_key` | 400 | 资源 key 非法 | 检查资源标识 |
| `invalid_portal_token` | 401 | Portal Token 无效 | 重新获取 portal token |
| `invalid_provider_id` | 400 | Provider ID 非法 | 传入合法 UUID |
| `invalid_region_id` | 400 | Region ID 非法 | 传入合法 UUID |
| `invalid_request` | 400 | 请求参数校验失败 | 按 message 修正参数 |
| `invalid_reservation_id` | 400 | 额度预占 ID 非法 | 传入合法 UUID |
| `invalid_scheduled_at` | 400 | 计划时间非法 | 传入合法时间 |
| `invalid_subscription_id` | 400 | 订阅 ID 非法 | 传入合法 UUID |
| `invalid_timestamp` | 400 | 事件时间戳非法 | 使用 RFC3339Nano |
| `invalid_to` | 400 | 时间终点非法 | 使用 RFC3339 |
| `invalid_to_cell_id` | 400 | 目标 Cell ID 非法 | 传入合法 UUID |
| `invalid_token` | 401 | 认证 Token 无效 | 重新登录或换 Key |
| `invalid_transition` | 409 | 生命周期迁移不合法 | 使用合法迁移路径 |
| `invalid_user_sub` | 400 | 用户 subject 非法 | 检查 user_sub |
| `invalid_webhook_id` | 400 | Webhook ID 非法 | 传入合法 UUID |
| `lifecycle_conflict` | 409 | 生命周期状态冲突 | 刷新状态后重试 |
| `live_review_required` | 409 | 首次 go-live 需通过风险审核 | 提交 approved risk review |
| `missing_key` | 400 | 缺少必需 key 参数 | 补充参数 |
| `missing_portal_token` | 401 | 缺少 Portal Token | 附带 Bearer token |
| `missing_provider_id` | 400 | 缺少 Provider ID | 补充 Provider ID |
| `not_found` | 404 | 资源不存在 | 检查资源 ID/路径 |
| `portal_not_configured` | 503 | Portal 未配置 | 联系平台配置 Portal |
| `provider_not_writable` | 409 | Provider 状态不允许写入 | 恢复 Provider 可写状态 |
| `quota_exceeded` | 422 | 硬额度不足 | 提升额度或释放预占 |
| `rate_limited` | 429 | 触发限流 | 按 Retry-After 退避 |
| `replay_invalid_state` | 409 | 投递状态不允许重放 | 仅重放 terminal 投递 |
| `risk_review_conflict` | 409 | 风险审核已提交 | 使用现有审核记录 |
| `tenant_context_override` | 403 | 请求试图覆盖 tenant 上下文 | 仅使用凭证推导上下文 |
| `unauthorized` | 401 | 未认证或凭证无效 | 登录或使用有效 Key |
| `upstream_timeout` | 504 | 上游请求超时 | 稍后重试 |
| `usage_already_invoiced` | 409 | 已出账用量不能直接冲销 | 使用 credit note |
| `usage_conflict` | 409 | transaction_id 已用于不同 payload | 使用新的 transaction_id |
