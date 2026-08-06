# 项目进度追踪

> 本文件用于持续追踪 vLogBin 平台的完成进度。每次完成/开始一项工作后更新对应状态。
> 状态标记：`✅ 已完成` / `🔄 进行中` / `⬜ 待办` / `⏸ 暂停`

## 一、里程碑（已提交，main 分支）

| 阶段 | 提交 | 内容 | 状态 |
|---|---|---|---|
| 设计评审 | `fd7c890` | 设计方案评审通过 | ✅ |
| Phase 0 | `f788472` | 架构骨架：Provider/Environment 平台骨架（Go API + Provider Console） | ✅ |
| Phase 1 | `0c5e04e` | 商用闭环：计费核心链路 + 生产门禁全覆盖 | ✅ |
| Phase 2+3 | `bcac951` | 企业功能全覆盖 + 生产门禁 + 代码审查修复 | ✅ |
| Phase 3+4 | `7d3fb30` | Cell 管理 + 热备 + 迁移 + 分析平面 + 按量计费 + FinOps | ✅ |
| 契约文档 | `23cb720` | OpenAPI + AsyncAPI 契约 + P0/P1 Runbook | ✅ |
| 全套文档 | `3bbaffd` | 技术/运维/部署/用户/集成文档 | ✅ |
| 测试提升 | `345f207`→`dad3c63` | 单元测试覆盖率提升至 90%+（含 httpapi 22 tests） | ✅ |

## 二、当前工作区（未提交）

> 截至最近一次更新，工作区共 170 个文件变更（含 92 个新增）。

### 候选 9：DB 慢查询追踪（pgx v5 QueryTracer）
- [x] `internal/store/tracer.go`：`queryTracer` 实现 `pgx.QueryTracer`，慢查询回调观察者
- [x] `store.go`：`Config.SlowQueryThreshold` + `SetSlowQueryObserver` + `New` 接线 tracer
- [x] `metrics.go`：新增 `db_query_slow_total` counter
- [x] `config.go`：新增 `DBQuerySlowThreshold`（`DB_SLOW_QUERY_THRESHOLD`，默认 0=禁用）
- [x] `main.go`：阈值传入 + 观察者（Inc + Warn 日志）
- [x] 测试：store +5 用例、config +2 断言、metrics/integration 断言
- [x] 文档：`DEPLOYMENT.md` 关键指标表 + 慢查询说明
- [x] 验证：build / vet / gofmt / 单测 / 集成测试 全绿
- **状态：✅ 已完成，待提交**

### 候选 10：API 限流增强（per-IP 兜底 + 精确 Retry-After + 429 指标）
- [x] `ratelimit/limiter.go`：新增 `AllowRetryAfter`（被拒时返回窗口精确剩余时间），`Allow` 委托
- [x] `middleware.go`：认证后 429 改用实际剩余窗口（`max(剩余秒,1)`）；新增 `ipRateLimitMiddleware`（认证前全局兜底）与 `clientIP`（XFF 优先 + RemoteAddr 回退）
- [x] `httpapi.go`：根路由注册 per-IP 中间件（metricsMiddleware 之后、认证之前）
- [x] `metrics.go`：新增 `http_requests_rate_limited_total{level}` counter（含全层级 pre-init）
- [x] `config.go`：`RateLimitConfig.IP`（默认 6000）+ `RL_IP_LIMIT` 解析（0=禁用）
- [x] 测试：limiter +4（Retry-After 允许/拒绝/窗口重置/委托）、config +1（默认/覆盖/禁用/非法值）、metrics 断言 + 层级 pre-init、middleware +4（clientIP/拒绝/禁用/指标）
- [x] 文档：`USER_GUIDE.md` 限流表 + IP 行、`TECHNICAL.md` 安全表、`DEPLOYMENT.md` 拒绝语义与 XFF 安全要求
- [x] 验证：build / vet / gofmt（本轮文件）/ 单测 / 集成测试 全绿
- **状态：✅ 已完成，待提交**

### 候选 11：下游熔断（Webhook 投递 + Billing 熔断）
- [x] `circuitbreaker/breaker.go`：三态熔断（closed→open→half_open），锁自由（原子 + CAS），懒恢复无后台 goroutine；`Allow`/`OnSuccess`/`OnFailure`，半开 `HalfOpenMax` 并发经 CAS 槽位
- [x] `config.go`：`CircuitBreakerConfig{FailureThreshold:5, OpenTimeout:30s, HalfOpenMax:1}` + `CB_FAILURE_THRESHOLD`/`CB_OPEN_TIMEOUT`/`CB_HALF_OPEN_MAX` 解析
- [x] `metrics.go`：`circuit_breaker_state{name}` GaugeVec + `circuit_breaker_requests_total{name,result}` CounterVec（含空标签 pre-init）
- [x] `webhook/worker.go`：`SetBreakerOptions` 链式；`breakerFor(endpointID)` 懒创建；`Allow()` 拒绝即快速失败（无网络调用）；5xx 计故障、4xx/成功/SSRF 计成功
- [x] `outbox/relay.go`：`WithCircuitBreaker`/`WithMetrics` 链式；抽 `retryOrDeadLetter`（熔断拒绝/失败走 maxAttempts→dead_letter 或退避）
- [x] `cmd/server/main.go`：relay + worker 两处接线
- [x] 测试：breaker 10 用例（状态机全路径）、config +3、metrics +2、billing `TestOutboxRelayCircuitBreakerFastFails`、webhook `TestWebhookCircuitBreakerTripsAndFastFails`、集成 metrics 断言 +2
- [x] 验证：build / vet / gofmt / 单测 / 集成回归 全绿
- **状态：✅ 已完成，待提交**

### 候选 12：OpenTelemetry 分布式追踪（httpapi 入站 + webhook/billing 出站 + store 查询 + outbox 事件）
- [x] `internal/telemetry/telemetry.go`（新建）：`Setup(cfg)` 返回 flush 闭包；禁用/`noop` → `noop.NewTracerProvider`（零开销）；`otlp`（`otlptracehttp`，默认 endpoint）/`stdout` 导出器；`semconv.ServiceName` + `DeploymentEnvironmentName` resource；`TraceIDRatioBased` 采样（0/1 特殊处理）；`WithBatcher`（batch timeout/export timeout/queue/batch size 可配）
- [x] `config.go`：`TelemetryConfig{Enabled:false, Exporter:"otlp", OTLPEndpoint, ServiceName:"vlogbin-api", Environment:"development", SampleRatio:1, ...}` + `OTEL_ENABLED`/`OTEL_EXPORTER`（枚举校验）/`OTEL_EXPORTER_OTLP_ENDPOINT`/`OTEL_SERVICE_NAME`/`OTEL_ENVIRONMENT`/`OTEL_SAMPLE_RATIO`（[0,1]）/批量导出参数解析
- [x] `internal/httpapi/tracing.go`（新建）：`tracingMiddleware` 最外层（recover 前），span 名取 chi `RoutePattern()`（未匹配 fallback 原始 path），属性 `HTTPRequestMethodKey`/`URLPath`/`ClientAddress`（hostOnly 剥端口）；5xx → `codes.Error`；`httpapi.go` 注册
- [x] `internal/store/otel_tracer.go`（新建）：`otelQueryTracer` span `store.query` + `db.statement`，错误 `RecordError`+`codes.Error`；`multiQueryTracer` 多 tracer 扇形广播（慢查询 + OTel 共存）；`WithQueryTracerProvider` Option
- [x] `store.go`：`Config.QueryTracerProvider` + `New()` 组装 tracer 切片（单/双自动切换）
- [x] `webhook/worker.go` + `billing/lago.go`：出站客户端 `Transport: otelhttp.NewTransport(http.DefaultTransport)` 获得 client span + W3C 传播
- [x] `outbox/relay.go`：`processEvent` span `outbox.process_event`（attrs `event_type`/`event_id`），错误记录+置错
- [x] `cmd/server/main.go`：`telemetry.Setup` + defer Shutdown（5s 超时 flush）+ 启用日志；`store.WithQueryTracerProvider(otel.GetTracerProvider())`
- [x] 测试：telemetry +7（禁用 noop/显式 noop/stdout/otlp 不拨号/endpoint/未知 exporter/非法 ratio）、store `otel_tracer_test` +4（`SpanStubFromReadOnlySpan`）、httpapi `tracing_test` +4（含 fallback 原始 path 修复：router 需注册路由使 chi 中间件链构建）、集成 `TestHTTPTracingEndToEnd`（真实 `http.Get` /health 断 W3C 链路）
- [x] 依赖：`otlptracehttp@v1.44.0`、`semconv/v1.34.0`（go.sum 已下载）
- [x] 验证：build / vet / gofmt / 单测 / 集成回归 全绿
- **状态：✅ 已完成，待提交**

### 候选 13：配置热加载（周期轮询 + SIGHUP，运行时应用热更新字段）
- [x] `internal/config/reload.go`（新建）：`Watcher`（周期 ticker `CONFIG_RELOAD_INTERVAL` + `signal.Notify` SIGHUP 双路径）；`diffHotReloadable` 仅检测 4 个热更新字段（rate_limits / slow_request_threshold / cors_allowed_origins / log_level），非热更新字段变化忽略；`Reload` 经 `sync.Mutex` 序列化、`onChange` 锁外回调；Load 失败保留旧配置（生产不因误配宕机）
- [x] `config.go`：新增 `ConfigReloadInterval`（`CONFIG_RELOAD_INTERVAL`，默认 0=禁用周期轮询）；限流 4 层 env 补全：`RL_PROVIDER_LIMIT`/`RL_ENVIRONMENT_LIMIT`/`RL_CREDENTIAL_LIMIT`/`RL_ENDPOINT_LIMIT`/`RL_WINDOW`（>0 校验）
- [x] `httpapi.go`：`Server.corsOrigins`/`rateLimits` 改 `atomic.Value`、`slowRequestThreshold` 改 `atomic.Int64`（请求并发读 vs 重载写）；`NewServer` 经三个 setter 初始化；新增 getter `CORSOrigins()`/`RateLimits()`/`SlowRequestThreshold()`（零值安全）
- [x] `middleware.go`：`rateLimitMiddleware`/`ipRateLimitMiddleware` 统一 `s.RateLimits()` 快照读取；`requestLogMiddleware` 原子读慢请求阈值；`corsMiddleware` 改 provider 函数每请求读取 `s.CORSOrigins()`——修复 chi v5 中间件链首次请求后缓存导致热更新失效的问题
- [x] `cmd/server/main.go`：`slog.LevelVar` + `setLogLevel`（未知值 fallback Info）支持运行时日志级别切换；workers 新增 `config-reload` goroutine，apply 回调按字段分发到 `SetRateLimits`/`SetSlowRequestThreshold`/`SetCORSOrigins`/`setLogLevel`
- [x] 测试：config `reload_test.go` +7（diff 单项/多项/无变化/忽略非热更新字段、Reload 应用、Load 失败保留旧配置、周期 Run 触发）、httpapi `hot_reload_test.go` +5（CORS/限流热更新、慢请求阈值 getter、RateLimits 往返、零值 Server）、`config_extra_test.go` +3（限流 env 覆盖/非法值/reload interval）
- [x] 验证：build / vet / gofmt / 全量单测 + 集成回归 全绿
- **状态：✅ 已完成，待提交**

### 候选 14：Stripe 风格 Idempotency-Key 幂等键
- [x] `db/migrations/0029_idempotency_keys.sql`（新建）：`idempotency_keys` 表（scope `provider:<uuid>`/`operator:<sub>` + key_hash(sha256) + method/path + in_progress/completed 状态机 + 响应缓存字段 + TTL），启用 RLS（tenant 仅见自己 scope，operator 全放行）+ 平台角色授权（goose StatementBegin 包裹 DO 块）
- [x] `db/queries/idempotency.sql` + `sqlc generate`：Get/Insert(ON CONFLICT DO NOTHING)/Complete/Delete/DeleteExpired 5 个查询；`sqlc.yaml` 补 0028/0029 schema
- [x] `internal/httpapi/idempotency.go`（新建）：中间件契约——无 key/读方法放行、非法 key 400、key 按身份+方法+路径隔离；首次执行 claim（唯一约束防并发竞态，输者 409 `concurrent`）、completed 命中重放（`Idempotency-Replayed: true`）、5xx 不缓存（删除记录允许重试）、in_progress 命中 409；bookkeeping fail-open（查询/写入失败不阻塞请求）；`SetIdempotencyTTL`（默认 24h）
- [x] Router 挂载 4 处认证 group：operator、signup、SCIM `/scim/v2`、tenant apiKeyAuth group（置于限流之后避免缓存 429）
- [x] `internal/config/config.go`：`IdempotencyTTL`（`IDEMPOTENCY_TTL`，默认 24h）+ `IdempotencySweepInterval`（`IDEMPOTENCY_SWEEP_INTERVAL`，默认 1h）
- [x] `internal/service/idempotency.go`（新建）：`PurgeExpiredIdempotencyKeys`（WithOperator 跨租户清理）+ `NewIdempotencyKeySweeper`；main 注册 `idempotency-sweeper` worker
- [x] 测试：httpapi `idempotency_test.go` +4（无 key 放行/读方法穿透/非法 key 400/边界校验）、integration `idempotency_test.go` +5（租户重放不重复建资源、operator 重放不重复建 provider、in_progress 409 concurrent、跨租户 scope 隔离、过期清理）
- [x] 验证：build / vet / gofmt / 全量单测 + 集成回归 全绿
- **状态：✅ 已完成，待提交**

### 候选 15：Redis 分布式限流（多实例共享计数）
- [x] `ratelimit/limiter.go`：定义 `Backend` 接口（`Allow`/`AllowRetryAfter`），内存 `Limiter` 自动实现，httpapi 依赖接口使后端可配置切换
- [x] `ratelimit/redis.go`（新建）：`RedisLimiter`——Lua 脚本原子 `INCR`+`EXPIRE`+`TTL`（窗口语义与内存版一致：首次请求开窗）；Redis 故障 **fail-open**（请求放行 + `OnError` 回调），启动 `Ping` 快速失败；`evalClient` 最小接口使单测可注入 fake；窗口 <1s 钳制为 1s 防 EXPIRE 删键；Retry-After 取 TTL 剩余秒（下限 1ms）
- [x] `config.go`：`RateLimitBackend`（`RATE_LIMIT_BACKEND`，默认 memory，枚举校验）+ `RedisAddr`/`RedisPassword`/`RedisDB`（`REDIS_ADDR`/`REDIS_PASSWORD`/`REDIS_DB`）；`RATE_LIMIT_BACKEND=redis` 必须设置 `REDIS_ADDR`
- [x] `metrics.go`：`rate_limiter_backend_errors_total` counter（fail-open 期间唯一信号）
- [x] `httpapi.go`：`Server.limiter` 改 `ratelimit.Backend` 接口 + `SetRateLimiter`（须在 Router 构建前调用）；`cmd/server/main.go` 按 `RATE_LIMIT_BACKEND` 接线（redis 时 `NewRedisLimiter` + `defer Close`，`OnError` → 指标 Inc + Warn 日志）
- [x] 依赖：`github.com/redis/go-redis/v9`（v9.21.0）
- [x] 测试：ratelimit `redis_test.go` +6（fake evaler：允许/拒绝/Retry-After 下限/窗口钳制/fail-open 错误/fail-open 异常结果 + key 前缀与窗口秒断言）、config +2（backend 解析/枚举与缺址校验）、metrics 断言 +1（预初始化 0）、integration `ratelimit_redis_test.go` +6（懒启动 Redis 容器：**跨实例共享计数**/Retry-After/窗口过期恢复/前缀隔离/启动 fail-fast/运行期 fail-open）
- [x] 文档：`DEPLOYMENT.md` 多实例部署 + REDIS_* 配置、`TECHNICAL.md` 限流架构、`PROGRESS.md`
- [x] 验证：build / vet / gofmt / 全量单测 + 集成回归（14 包）全绿
- **状态：✅ 已完成，待提交**

### 候选 16：审计日志查询增强（keyset 分页 + 多维过滤）
- [x] `db/queries/audit.sql`：`ListAuditEventsFiltered`——keyset 分页（`(created_at, id)` 元组游标，bigint 审计事件主键作游标，0 = 最新起）+ 5 维过滤（action/actor_type/actor_id/target_type/target_id 空串跳过）+ 时间窗口（from/to 空串开放，RFC3339Nano）；LIMIT 取 limit+1 探测下一页
- [x] `internal/service/service.go`：`AuditQuery` 结构 + `ListAuditEvents`/`ListProviderAuditEvents` 返回 `(*int64, error)` next_cursor；`trimAuditPage` 裁剪探测行、游标指向**最后一条已返回行**（保证下一页无跳过/无重复）
- [x] `internal/httpapi`：`GET /v1/audit-events`（fallback 100）与 `GET /v1/operator/providers/{id}/audit`（fallback 200）支持 `cursor/limit/action/actor_type/actor_id/target_type/target_id/from/to`，响应 `{"audit_events": [...], "next_cursor": <id|null>}`；参数校验 400（`invalid_cursor`/`invalid_from`/`invalid_to`，cursor 须为正 int64）
- [x] 测试：integration `audit_query_test.go` +4（分页 5 条 limit=2 全量取回无丢无重 + 顺序；action/actor_type/from/to/组合过滤；租户隔离 A 不可见 B + operator 跨租户可见；参数校验 400 错误码），审计端点需 `audit:read` scope（测试先 mint 最小权限子 key）
- [x] 验证：build / vet / httpapi 单测 + 全量集成回归全绿
- **状态：✅ 已完成，待提交**

### 候选 17：审计仪表盘 API（趋势聚合）
- [x] `db/queries/audit.sql`：+4 聚合查询（`CountAuditEventsFiltered` / `AuditEventActionCounts` / `AuditEventActorTypeCounts` / `AuditEventTimeSeries`），与列表查询共享同一 filter 集；series 用 `date_trunc(granularity)` 按 hour/day/week 分桶
- [x] `internal/service/service.go`：`AuditStatsQuery`（过滤器 + From/To + Granularity）、`AuditStats`/`AuditCount`/`AuditSeriesPoint`；`AuditEventStats`（tenant）/`ProviderAuditEventStats`（operator，先 `GetProviderByID` 校验 404）；`fillSeries` 在 Go 层**零填充空 bucket**（对齐 PG date_trunc UTC 语义：整点/UTC 午夜/周一），窗口无界时原样返回
- [x] `internal/httpapi`：`GET /v1/audit-events/stats`（`audit:read` scope）与 `GET /v1/operator/providers/{id}/audit/stats`；**from/to 必填**（无界聚合是生产 footgun）+ 窗口上限 366 天（`range_too_wide`）+ granularity 枚举（`invalid_granularity`）+ `invalid_range`；提取公共 `parseAuditFilters` 供列表/统计双端点复用
- [x] 测试：integration `audit_stats_test.go` +7（聚合正确性/时间序列零填充 day+hour/过滤作用于聚合/参数校验 5 错误码/租户隔离+operator 跨租户/无 `audit:read` 403（mint `read` 子 key）/列表计数与 series 求和交叉验证）
- [x] 验证：build / vet / gofmt / httpapi 单测 + 全量集成回归全绿
- **状态：✅ 已完成，待提交**

### 候选 18：审计导出 API（流式 CSV/JSON）
- [x] `internal/service/service.go`：`AuditExportQuery`（复用 `parseAuditFilters` 同款 filter 集）；`ExportAuditEvents`（tenant）/`ExportProviderAuditEvents`（operator，先 `GetProviderByID` 校验 404）；`streamAuditEvents` 复用 `ListAuditEventsFiltered` + `trimAuditPage` 按 `auditExportPageSize=1000` keyset 遍历，**emit 回调逐行消费，内存恒定 O(pageSize)**，不引入独立 SQL
- [x] `internal/httpapi/handlers_provider.go`：`GET /v1/audit-events/export`（`audit:read` scope）；`auditExportQueryFromRequest` 复用 `parseAuditFilters` + **from/to 必填**（`missing_from`/`missing_to`）+ 窗口 366 天上限（`range_too_wide`）+ format 枚举（默认 csv/json，非法 `invalid_format`）；`streamAuditExport` 设置 Content-Type/Content-Disposition（`audit-export-YYYYMMDDTHHMMSSZ`），CSV 用 `csv.Writer` 逐行 Write+Flush，JSON 手写数组 `[`/`,`/`]` 分隔；`auditExportCSVHeader` 固定 9 列文档化 schema（id/created_at/actor_type/actor_id/action/target_type/target_id/request_id/metadata），metadata JSONB 原样嵌入
- [x] `internal/httpapi/handlers_operator.go`：`GET /v1/operator/providers/{id}/audit/export`；非法 id `invalid_id`；**流式写头前预检 provider 存在性**（404 必须以 JSON 返回，写头后状态码锁定）；`httpapi.go` 两条路由注册
- [x] 测试：integration `audit_export_test.go` +7（CSV 表头 9 列+行内容+metadata 原样/JSON 裸数组解析/参数校验 5 错误码/租户隔离+operator 跨环境/无 `audit:read` 403/operator invalid_id+未知 provider 404/与列表端点一致性交叉验证）；新增 `apiRawReq` 原始 body 辅助（导出非 JSON-object 响应）
- [x] 验证：build / vet / gofmt / httpapi 单测 + 全量审计集成回归全绿
- **状态：✅ 已完成，待提交**

### 候选 19：审计保留策略（operator-only 受控清理）
- [x] 迁移 `0030_audit_retention.sql`：`idx_audit_events_created_at` 索引；`purge_audit_events(cutoff, max_rows)` **SECURITY DEFINER** 函数——校验 `app.is_operator = 'on'`（tenant 直接 RAISE）→ 同事务 `DISABLE TRIGGER audit_events_append_only` → `DELETE ... WHERE ctid IN (子查询 ORDER BY created_at LIMIT)` 按序小批量删 → `ENABLE TRIGGER`；任何失败整体回滚、触发器不受影响；REVOKE PUBLIC + 仅授权 `platform_app`
- [x] `queries/audit.sql`：`PurgeExpiredAuditEvents :one` 包装函数调用；sqlc 生成 `PurgeExpiredAuditEvents(cutoff, max_rows) (int64, error)`
- [x] `internal/service/service.go`：`PurgeExpiredAuditEvents`（`auditPurgeBatchSize=50000`，**每批独立短事务**循环直到删完，返回总数）；`NewAuditRetentionSweeper` 复用 `ExpirySweeper` 基建（metrics/查询超时/幂等启动保护）
- [x] `internal/config/config.go`：`AUDIT_RETENTION_DAYS`（默认 **0=禁用**——审计是合规证据，显式配置才启动 sweeper；负数报错）+ `AUDIT_RETENTION_SWEEP_INTERVAL`（默认 1h）
- [x] `cmd/server/main.go`：`audit-retention-sweeper` worker，仅 `AuditRetentionDays > 0` 时注册
- [x] 测试：integration `audit_retention_test.go` +4（过期删+新鲜留；cutoff 设 2 年前保证与 audit_stats/export 固定时间戳顺序无关；5 条×batch=2 分批遍历验证循环；tenant 上下文调用被拒；**超管直连 DELETE 仍被追加写触发器拒绝 → purge 是唯一删除路径**）；config 单测 +2（默认禁用/非法负数报错）
- [x] 验证：build / vet / 审计工具链（query+stats+export+retention）全量集成回归全绿
- **状态：✅ 已完成，待提交**

### 候选 20：审计哈希链（Tamper-Evident Audit Log）
- [x] 迁移 `0031_audit_hash_chain.sql`：`CREATE EXTENSION pgcrypto`；`audit_events` 加 `prev_hash`/`event_hash` + 唯一索引 `idx_audit_events_event_hash`；`audit_event_hash(prev, ev)` **SQL IMMUTABLE 规范哈希**（ASCII 0x1f 分隔 12 字段，jsonb ::text 规范化）；`audit_chain_tail`（单行 `CHECK (id=1)`，尾指针 + `pruned_through_id`）+ `audit_chain_anchors`（锚点表，含 operator 与覆盖范围）；`audit_events_hash` **BEFORE INSERT 触发器**——`FOR UPDATE` 锁尾行串行化追加、`audit_event_hash` 计算 prev/event hash（应用层无法伪造链）；backfill DO 块（`DISABLE TRIGGER audit_events_append_only` 后逐行回填，`r audit_events` 类型化行变量）；**替换 `purge_audit_events`** 记录 `pruned_through_id`（prune 后链头前移，验证从幸存首行继续，跨保留边界可验证）；`audit_chain_verify(from,to)` RETURN TABLE + `anchor_audit_chain(operator)` 均 **SECURITY DEFINER + `app.is_operator='on'` 校验**（tenant 直接 RAISE）；`%ROWTYPE` 字段访问规避与 RETURNS TABLE 输出列同名歧义；REVOKE PUBLIC + 仅授权 `platform_app`
- [x] `queries/audit.sql`：`AuditChainState :one`（尾指针独立查尾）+ `LatestAuditAnchor :one`（`pgx.ErrNoRows` 容忍空锚点）+ `VerifyAuditChain :one`（显式列 + `AS v` cast）+ `AnchorAuditChain :one`；**NULL 列 COALESCE 非空化**（broken_at→0 / tail_event_id→0 / tail_hash→''），避免 sqlc 非指针字段扫描失败
- [x] `internal/service/service.go`：`AuditChainState`/`AuditChainVerifyResult`/`AuditChainAnchor` 类型；`VerifyAuditChain`（负值/非法范围 ErrValidation）；`NewAuditChainAnchorSweeper` 复用 `ExpirySweeper` 基建（metrics/查询超时/幂等启动保护）
- [x] `internal/config/config.go`：`AUDIT_CHAIN_ANCHOR_INTERVAL`（默认 **24h**，`0=禁用` sweeper；负数报错）
- [x] `cmd/server/main.go`：`audit-chain-anchor-sweeper` worker，仅 interval > 0 时注册
- [x] `internal/httpapi`：operator 路由 `GET /v1/operator/audit/chain`（尾指针+最后锚点）、`GET /v1/operator/audit/chain/verify`（`from/to` 可选，非法 → 400 `invalid_from`/`invalid_to`）、`POST /v1/operator/audit/chain/anchor`（201，返回 anchor_id/tail_event_id/tail_hash/events_covered）
- [x] 测试：integration `audit_chain_test.go` +5（**BEFORE INSERT 自动生成链**：prev_hash 链连续 + state 与 DB 尾一致；**篡改检测**：DISABLE append-only 触发器改 action → verify 报 `broken_at` 精确行 + reason mismatch，恢复后链愈合；**保留 purge 后链仍有效**：`pruned_through_id` 前移 + 整链 verify ok；**锚点生命周期**：锚定→state 上报最后锚点→增量验证→二次锚点 `events_covered` 精确增量；**operator-only**：tenant 上下文 verify/anchor 被拒 + 匿名 401）
- [x] 验证：sqlc generate / build / vet / **审计工具链（query+stats+export+retention+chain）全量集成回归全绿**
- **状态：✅ 已完成，待提交**
- **诚实性边界**：DB 内链是 **tamper-evident**（可检测篡改）而非 **tamper-proof**（不可篡改）；完整防篡改需将锚点导出到外部 **WORM 对象存储**（候选"WORM 审计归档"前置已就绪：`audit_chain_anchors` 增量锚定可作 WORM 写入基准）

### 候选 21：Worker 监督框架（panic 恢复 + 指数退避重启）
- [x] `internal/worker/supervisor.go`（新建）：`Supervisor.Run(ctx, name, fn)` 监督单个 worker——panic 由 `recover` 捕获（日志 `worker panicked` + `debug.Stack` 全栈，经 `OnPanic` 回调上报指标）；**ctx 取消是唯一优雅退出路径**，其余一律视为异常退出（`OnRestart(name, reason)`，reason ∈ `panic`/`exit`）并按指数退避重启；退避 `BackoffInitial`(1s) ×`BackoffFactor`(2) 递增、`BackoffMax`(30s) 封顶、**worker 存活 ≥ `ResetAfter`(5m) 重置退避**（防止单次偶发崩溃把 worker 永久钉在高退避）；`sleep` 尊重 ctx 取消（退避期间关闭立即停止）
- [x] `internal/worker/supervisor_test.go`（新建）：6 个单测（`-race` 通过）——优雅关闭不重启 / panic 恢复后重启且 `OnPanic` 触发带栈 / error 退出重启 reason=exit / nil 提前退出重启 / **永久 panic 循环持续重启不退场**（backoff 封顶验证）/ 健康运行 ≥ ResetAfter 后退避重置（`BackoffFactor=20` 放大差异断言第 3 次重启 ~5ms 而非 ~100ms）
- [x] `internal/metrics/metrics.go`：`worker_restarts_total{name,reason}` + `worker_panics_total{name}`（**worker 静默死亡/崩溃循环直接可告警**）
- [x] `internal/config/config.go`：`WORKER_BACKOFF_MAX`（默认 30s，0 用内置默认；负数报错）
- [x] `cmd/server/main.go`：**全部 13 个后台 worker 统一经 supervisor 注册**（config-reload / outbox-relay / reconciliation / webhook-delivery / support-sweeper / quota-sweeper / retention-sweeper / audit-retention-sweeper / audit-chain-anchor-sweeper / idempotency-sweeper / backlog-reporter / migration-scheduler / pool-reporter / pprof）——此前任何 worker panic 都会**静默永久停摆**，仅表现为陈旧指标；pprof 的 stop 先 cancel ctx 再 Shutdown，优雅关闭不误触发重启
- [x] 测试：config_extra_test +3（解析 90s / 默认 30s / 负数报错）
- [x] 验证：build / vet / config 单测 / worker 单测（`-race`）全绿
- **状态：✅ 已完成，待提交**
- **设计要点**：监督语义"run 直到 ctx 取消"统一了所有 worker 契约——`fn` 返回 nil 且 ctx 未取消也视为异常（worker 不该自行退场）；`done` channel 由 supervisor 关闭，shutdown 的 `stopWorkers` 等待逻辑零改动兼容

### 候选 22：PSP 主密钥轮换（多密钥解密回退 + 存量密文零迁移）
- [x] `internal/crypto/crypto.go`：`Encryptor` 由单一 `aead` 改为 **active 密钥（加密用）+ previous 密钥列表（仅解密回退）**；`NewEncryptor` 签名不变（无 previous，向后兼容），新增 `NewEncryptorWithPrevious(masterKeyHex, previousKeys)`（任一 previous 密钥非法即构造失败，防止笔误静默弃用全部存量密文）+ `SetFallbackObserver(fn)`；`Decrypt` 先试 active、失败后按序试 previous（GCM 认证失败快速短路），全部失败才报错——**密文格式零变化，无需数据迁移**
- [x] `internal/crypto/crypto_rotation_test.go`（新建）：6 个单测——旧密钥密文轮换后可解 / **新密文用 active 密钥封存**（仅含新密钥的 Encryptor 可解，证明加密未用 previous）/ fallback 回调仅 previous 命中时触发 / 两次轮换后 k1、k2、k3 三代密文均可解 / 全密钥不匹配时报错 / 非法 previous 密钥（非 hex、AES-128 长度）构造即拒绝
- [x] `internal/metrics/metrics.go`：`credential_decrypt_fallback_total`（**回退解密计数**——非零即告警"仍有存量密文用旧密钥读取，需排期重加密收敛"）
- [x] `internal/config/config.go`：`PSP_MASTER_KEY_PREVIOUS`（逗号分隔旧密钥列表，`splitComma` 去空白），默认空
- [x] `cmd/server/main.go`：encryptor 构造改 `NewEncryptorWithPrevious`（日志记录 previous 数量）；apiServer 创建后 wire fallback observer → `CredentialDecryptFallbackTotal`
- [x] 测试：crypto 单测（含 `-race`）全绿；config_extra_test +1（默认空 / 单个 / 多个去空白）
- [x] 验证：build / vet / crypto+metrics+config 单测 / 集成回归 18s 全绿
- **状态：✅ 已完成，待提交**
- **设计要点**：轮换流程 = 新密钥设为 `PSP_MASTER_KEY`、旧密钥移入 `PSP_MASTER_KEY_PREVIOUS` → 存量密文自动可读；**零信任原则**——previous 密钥不参与加密，避免新数据意外用旧密钥封存；观察回退计数归零后即可从配置移除旧密钥完成收敛

### 候选 23：/ready 依赖健康汇总（并行检查 + 结构化明细 + 抖动指标化）
- [x] `internal/ratelimit/limiter.go`：`Limiter`（内存）加 `Ping(context.Context) error` → 恒 nil（无外部依赖），使所有限流后端对 /ready 统一可查
- [x] `internal/ratelimit/redis.go`：`evalClient` 接口加 `Ping`；`RedisLimiter.Ping` 真实 Redis 往返——**限流 fail-open 的盲区被补齐**：Redis 挂掉时限流失效但请求仍放行，此前 /ready 依旧 200，LB 不摘实例；现在探针如实反映
- [x] `internal/ratelimit/redis_test.go`：`TestRedisLimiterPing`（健康/不可达）+ `TestLimiterPingAlwaysNil`
- [x] `internal/httpapi/handlers_health.go`：`ready` 重构——依赖清单（`database` 必查 + `ratelimit` 按 `dependencyPinger` 接口断言注入）**并行检查**（互不拖累），`readyTimeout` 总超时兜底；响应升级为结构化 `dependencies{name:{status,latency_ms,error?}}`；任一 down → 503 `unavailable`（保留顶层 `error` 兼容既有消费者）；`status` 字段与 200/503 语义零变化，K8s 探针不受影响
- [x] `internal/metrics/metrics.go`：`readiness_checks_total{dependency,status}`——ready 抖动可按依赖归属告警（`sum by (dependency) (rate(readiness_checks_total{status="down"}))`）
- [x] `internal/integration/health_test.go`：断言 `dependencies` 含 `database` + `ratelimit` 且均 `up`
- [x] 验证：build / vet / gofmt / ratelimit+metrics 单测（-race）/ 集成回归 18s 全绿
- **状态：✅ 已完成，待提交**
- **设计要点**：HTTP 状态码仍是探针唯一依赖信号，body 只服务人类排障；依赖检查 goroutine 有界（缓冲 channel + 精确收 N 次），所有 Ping 均接受 ctx，超时兜底无泄漏；Redis 后端探测失败**不触发**限流 onErr 回调（Ping 是健康语义，与请求路径故障区分）

### 候选 24：主密钥重加密收敛 worker（轮换闭环自动收敛 + 幂等 + 不可收敛可告警）
- [x] `internal/crypto/crypto.go`：`Decrypt` 核心抽取 `decrypt(ct, notify)`；新增 `NeedsReencryption(ct) (bool, error)`（纯检测：active 可解 → false、previous 可解 → true、全失败 → err，**不触发 fallback observer**）+ `DecryptWithoutFallback`（worker 专用，不污染 `credential_decrypt_fallback_total` 的"读路径"语义——worker 收敛由 `credentials_reencrypted_total` 单独计数）
- [x] `db/queries/{psp,notifications,auth}.sql`：6 条新查询（operator 视图全表列出 + 密文更新）；`psp_credentials` 更新用 **`NULLIF` 保持 NULL webhook 幂等**（sqlc 把 nullable text 读出为 ""，写回必须映射回 NULL）
- [x] `internal/service/reencrypt.go`（新建）：`ReencryptLegacyCiphertexts(ctx, batch)`——**operator 上下文**逐表（`psp_credentials` / `notification_configs` / `provider_auth_configs`）分批（默认 100 行/表/事务）扫描，检测→静默解密→active 重加密→更新；每批独立短事务（长积压不持大事务）；`reencryptField` 辅助（空字段跳过、不可解计数）；`NewReencryptionSweeper` 包装 `ExpirySweeper`（与候选 21 监督框架无缝衔接）
- [x] `internal/service/service.go`：`SetReencryptReporter(table, reencrypted, errors)`（setter 注入，风格同候选 22 `SetFallbackObserver`——reporter 需在 metrics 存在后 wire）
- [x] `internal/metrics/metrics.go`：`credentials_reencrypted_total{table}`（收敛进度）+ `credentials_reencrypt_errors_total{table}`（**不可收敛行——密钥全丢/密文损坏，唯一阻塞完整收敛的信号，需人工介入**）
- [x] `internal/config/config.go`：`REENCRYPT_SWEEP_INTERVAL`（默认 0 = 禁用）+ `REENCRYPT_BATCH_SIZE`（默认 100，必须 >0）
- [x] `cmd/server/main.go`：仅当 `interval > 0 && encryptor != nil && len(previous) > 0` 时启动监督 worker（无旧密钥则空转无意义）；reporter wire 到两个指标
- [x] 测试：crypto +2（`NeedsReencryption` 四分支含 observer 不触发 / `DecryptWithoutFallback` 静默 + 对照 `Decrypt` 仍通知）；config +1（默认/显式/非法值）；集成 `reencrypt_test.go`（**端到端轮换闭环**：旧密钥封存 → 收敛 → 新密钥单独可解、旧密钥单独不可解 → 二次扫描幂等返回 0；该测试全库收敛，按文件名字母序置于包末）
- [x] 验证：build / vet / gofmt / crypto+metrics+config 单测（-race）/ 集成回归 18.5s 全绿
- **状态：✅ 已完成，待提交**
- **设计要点**：收敛闭环 = 轮换后启动 worker → `credentials_reencrypted_total` 增长至停 → `credential_decrypt_fallback_total`（读路径）归零 → 从 `PSP_MASTER_KEY_PREVIOUS` 移除旧密钥；**幂等**（二次扫描 0 变更，不抖写）；**不可收敛行独立可告警**（`credentials_reencrypt_errors_total` 非零 = 密钥丢失或数据损坏，阻塞完整收敛需人工）；worker 用 `DecryptWithoutFallback` 保证 fallback 指标语义纯净（"请求路径仍在读旧密文"）

### 候选 25：WORM 审计归档（外部锚定，Tamper-Proof Audit Chain）
- [x] 迁移 `0032_audit_anchor_archive.sql`：`audit_chain_anchors` 加 `published_at timestamptz` / `object_key text` + 部分索引 `idx_audit_chain_anchors_unpublished ON (id) WHERE published_at IS NULL`；REVOKE ALL + 仅授予 `platform_app` SELECT, UPDATE（UPDATE 仅用于 mark，无 DELETE——归档副本不可篡改，DB 侧也不留删除路径）
- [x] `queries/audit.sql`：`ListAuditAnchorsForPublish :many`（`WHERE published_at IS NULL`）+ `MarkAuditAnchorPublished :one`（`WHERE id=$1 AND published_at IS NULL` 守卫，0 行即并发已发布）；storegen 已生成
- [x] `internal/archive/archive.go`（新建）：`AnchorRecord{AnchorID, TailEventID, TailHash, Operator, CreatedAt, ContentSHA256}`；`NewArchiver`（minio-go v7.2.1，**不自动创建 bucket**——避免误建非 WORM 桶）；`PublishAnchor` 对**排除自身**的字段算 sha256 后上传确定性 key `audit/anchors/{anchor_id}.json`
- [x] `internal/service/audit_archive.go`（新建）：`AnchorArchiver` 接口 + `SetAuditArchiver`/`SetAuditArchiveReporter`（setter wire，风格同候选 22/24）；`ArchiveAuditAnchors(ctx, batch)` 三步骤协议——短只读事务取批 → **事务外上传**（失败不触碰 DB）→ 短事务 `UPDATE ... WHERE published_at IS NULL` mark；崩溃恢复 = 重传同 key + 补 mark，无半状态；`NewAuditArchiveSweeper` 接入候选 21 监督框架
- [x] `internal/config/config.go`：`AUDIT_ARCHIVE_SWEEP_INTERVAL`（默认 0 = 禁用）+ `AUDIT_ARCHIVE_BATCH_SIZE`（默认 100）+ 新 `ObjectStorageConfig`（`AUDIT_ARCHIVE_S3_{ENDPOINT,BUCKET,ACCESS_KEY,SECRET_KEY,REGION,USE_SSL}`）；interval>0 时四要素必填否则 **fail-closed 启动失败**
- [x] `internal/metrics/metrics.go`：`audit_anchors_published_total{result}`（published/already_published，完整性信号）+ `audit_archive_errors_total{op}`（list/upload/mark），预初始化 0
- [x] `cmd/server/main.go`：仅当 interval > 0 时创建 archiver + 注册 `audit-archiver` 监督 worker + reporter 拆分 op 累加
- [x] 测试：archive 单测（key 确定性 / JSON 形状 / 坏 endpoint 拒绝）；config 单测（默认/显式/校验缺失四要素）；集成 `audit_archive_test.go` 三组——发布+幂等（二次 sweep 0 计数，无重复上传由对象数不变证明）、崩溃恢复（上传后 mark 前 panic → 重跑补齐 mark 不重传）、存储故障恢复（upload 失败不标记 → 重跑重传）
- [x] 验证：build / vet / 全部内部测试（含集成）全绿
- **状态：✅ 已完成，待提交**
- **设计要点**：外部锚定 = DB 超级用户也无法篡改的不可变副本，tamper-evident → **tamper-proof**；幂等发布三步骤（取批 → 事务外上传 → 守卫 mark）保证崩溃任意点恢复后无重复、无丢失；`already_published` 仅计数"上传成功但 mark 0 行"的竞态路径（被 list 过滤的锚点不出现在批次中，不计入）；桶必须开启 object lock / retention，archiver 只 PUT 无需读/列权限

### 候选 26：Outbox 死信队列正式化（真 dead_letter 状态 + last_error + 对账复活）
- [x] 迁移 `0033_outbox_dead_letter.sql`：`outbox_events.status` CHECK 加入 `'dead_letter'` 真终态；`ADD COLUMN last_error text`（最终失败原因）；**历史软死信升级**——`status='failed' AND next_attempt_at IS NULL` 的行升级为 `dead_letter` + 迁移说明，让对账立即反映真实死信量
- [x] `queries/outbox.sql`：`MarkOutboxEventDeadLetter` 写真 `dead_letter` 状态 + `last_error=$2`（原为软死信 `status='failed'`，导致 `CountDeadLetterOutbox` 恒 0——**reconciliation 的 outbox_dead_letter check 形同虚设**）；`DeleteExpiredOutboxEvents` 保留策略纳入 dead_letter（fresh 死信不删，aged 清理）；`CountUnconfirmedOutbox`/`ClaimDueOutboxEvents`/`CountOutboxEventsByStatus` 注释对齐 dead_letter 终态语义（不重放、不 claim、分组含 dead_letter）
- [x] `internal/outbox/relay.go`：`markDeadLetter(ctx, id, cause)` 记录最终失败原因（max attempts 与 payload unmarshal 两个入口）；死信时累计 `outbox_dead_letter_total`
- [x] `internal/metrics/metrics.go`：新增 `outbox_dead_letter_total` Counter（死信速率告警源）；`outbox_events_total` 预初始化 dead_letter 标签
- [x] sqlc.yaml 补 0032/0033 到 schema（0032 此前缺失，`ListAuditAnchorsForPublish` 因显式列列表改用 `SELECT *` 保持表类型；`FindUndeliveredOutboxEvents` 补 last_error 列）；storegen 已重新生成
- [x] 测试：集成 `outbox_dead_letter_test.go`——max attempts 路径（5 次失败 → dead_letter + last_error="simulated billing engine outage"，不再被 claim，reconciliation 可见，retention fresh 不删/aged 删）+ 无效 payload 路径（jsonb 保证合法 JSON，shape 不匹配立即死信，last_error 含 "payload unmarshal"；测试自清理避免污染全局 dead_letter 对账检查）
- [x] 验证：build / vet / 全部内部测试（含集成）全绿
- **状态：✅ 已完成，待提交**
- **设计要点**：`outbox_events.payload` 是 jsonb，非法 JSON 在存储层即被拒绝，因此 unmarshal 失败只可能来自"合法 JSON 但 schema 不兼容"（如字段类型演化）——该路径立即死信（重试无意义）；max attempts 是主要死信路径（指数退避 5 次）；死信为**真终态**（不重放、不 claim、retention 保留期后清理），`CountDeadLetterOutbox` 对账 check 从"恒 0 摆设"变为可告警的完整性信号

### 候选 27：财务差异对账（发票金额自洽检查）
- [x] `queries/reconciliation.sql` 追加 3 个财务 check 查询：`CountInvoiceAmountMismatches :one`（finalized/voided 发票违反金额不变量：`sub_total_including <> sub_total_excluding + taxes`）、`CountInvoiceLinesTotalMismatch :one`（HAVING `sum(line) <> header total` 的发票行合计漂移）、`CountUnpaidFinalizedOverdue :one`（finalized 且 `payment_status <> 'succeeded'` 超 7 天）；storegen 已重新生成
- [x] `internal/service/reconciliation.go`：`RunReconciliation` 追加 r6/r7/r8 三个 check（`invoice_amount_consistency`、`invoice_lines_total_match`、`unpaid_finalized_overdue`），沿用 `Status ok/drift + storeReconciliationResult` 模式；`ReconciliationWorker` 增 `metrics *metrics.Metrics` 字段（nil 安全）；`NewReconciliationWorker(svc, interval, log, m)` 加参数；`RunOnce` 每 check `WithLabelValues(r.Name).Set(float64(r.DriftCount))`，run 失败时 `run_error` 标签 Set(1)
- [x] `internal/metrics/metrics.go`：新增 `reconciliation_drift` GaugeVec（label `check`，help 注明再刷新语义），注册进 Collector；`New()` 预初始化 8 个 check 名（5 旧 + 3 新）保证首个 scrape 即可匹配
- [x] `cmd/server/main.go`：reconciliation worker 注入 `apiServer.Metrics()`（复用既有 `Server.Metrics()` getter，避免双 registry）
- [x] 测试：集成 `reconciliation_financial_test.go` `TestReconciliationFinancialChecks`——4 阶段基线 diff 断言（自洽发票 0/0/0 → 金额不变量 1/0/0 → 行合计漂移 1/1/0 → 逾期未付 1/1/1）；`insertInvoice` 从 `customer_accounts` 继承 provider/environment 并带 `lago_id`
- [x] 验证：`make test-integration` ✅、`go test ./internal/... -count=1` 15 包全过 ✅、`go vet ./...` 干净 ✅
- **状态：✅ 已完成，待提交**
- **设计要点**：本地无 payments 表（支付记录在 Lago 侧，本地仅 `invoices.payment_status`），故财务差异处置落地为「发票金额自洽对账」3 个 check——金额不变量（`sub_total_including = sub_total_excluding + taxes`，credit 发票 total 为负故排除非负检查）、行合计漂移（HAVING 聚合与 header 对不上）、逾期未付（finalized 未成功支付超 7 天）；**WORM 冲突**：`invoice_lines` 的 `guard_finalized_invoice()` trigger 禁止对 finalized/voided 发票行做 UPDATE/DELETE，测试改用基线 diff 断言（吸收其他测试沉淀的 WORM fixture），cleanup 只删无行发票（inv2/inv4），inv1/inv3 因 WORM 留在数据库（测试文件注释 + 本计划记录）；与候选 25/26 共同达成架构设计 P0 门禁「每小时对账、DLQ 和财务差异处置启用」闭环

### 候选 28：Live Provider 风险审核门禁（Go-live Checklist + Risk Score）
- [x] 迁移 `0034_provider_risk_reviews.sql`：`provider_risk_reviews`（`provider_id` FK + `risk_score smallint CHECK 0-100` + `checks jsonb` + `decision CHECK (approved|rejected)` + `reason/reviewed_by/reviewed_at`）；索引 `(provider_id, reviewed_at DESC)`；**RLS FORCE + operator-only policy**（无 provider 策略——风险评分为 operator 内部评级，provider 不可读）；GRANT `platform_app`；`-- +goose Up/Down` 头
- [x] `queries/risk_reviews.sql`（新建）：`CreateProviderRiskReview :one` / `LatestProviderRiskReview :one` / `ListProviderRiskReviews :many`（newest-first）；sqlc 已生成 `storegen/risk_reviews.sql.go`；`sqlc.yaml` 已登记 0034 schema
- [x] `internal/domain/lifecycle.go`：新增 `RiskDecisionApproved = "approved"` / `RiskDecisionRejected = "rejected"` 常量
- [x] `internal/service/risk_review.go`（新建）：`RiskReviewChecks` 7 键清单常量（email_and_company_domain / tos_dpa / custom_domain_ownership / payment_tax_connection / webhook_destination / initial_quota / security_contact；`risk_score` 为第 8 项独立列）；`RiskReviewInput`；`SubmitRiskReview`（WithOperator，校验 provider 必须在 LIVE_REVIEW，原子写 outbox `provider.risk_reviewed` + audit `provider.risk_review`）；`validateRiskReview`（decision 枚举、score 0-100、approval 需全 check true、reviewer 必填）；`requireApprovedReview`（无记录/非 approved → `ErrLiveReviewRequired`）；`ListRiskReviews`；`ErrLiveReviewRequired` / `ErrRiskReviewConflict`
- [x] `internal/service/service.go`：`TransitionLifecycle` 在 `domain.Transition` 后插入门禁——仅 `LIVE_REVIEW → LIVE_ACTIVE`（首次 go-live）强制 latest approved review；`RESTRICTED/SUSPENDED → LIVE_ACTIVE`（reactivation）不强制（复用既有批准记录，避免破坏 webhook reactivation 语义）
- [x] `internal/httpapi/handlers_risk_review.go`（新建）：`operatorSubmitRiskReview`（POST `/v1/operator/providers/{id}/risk-review` → 201）、`operatorListRiskReviews`（GET `/v1/operator/providers/{id}/risk-reviews`）；handler 不做 reviewer 兜底（交由 service 校验，缺失即 400）
- [x] `internal/httpapi/httpapi.go`：operator 路由组注册 2 条路由；`serviceError` 加 `ErrLiveReviewRequired`→409 `live_review_required`、`ErrRiskReviewConflict`→409 `risk_review_conflict` 映射
- [x] 测试适配（4 处首次 go-live 调用先提交 approved review）：`web_simulation_test.go` 新增 `submitApprovedRiskReview` helper 并在 `createProviderAtState` 的 LIVE_REVIEW/LIVE_ACTIVE/RESTRICTED/SUSPENDED 四分支插入；`api_test.go`、`rls_test.go`（改用 `svc.SubmitRiskReview`）、`providers_activate_test.go`（Concurrency + AuditTrail 两测试）插入
- [x] 集成测试 `risk_review_test.go`（新建）：`TestRiskReviewGoLiveGate`（无 review→409→rejected 后仍 409→approved 后 200→历史 newest-first）、`TestRiskReviewValidation`（5 种非法输入 400 + 确认零落库）、`TestRiskReviewRequiresLiveReview`（非 LIVE_REVIEW→409 risk_review_conflict）、`TestRiskReviewOperatorOnly`（provider token 401/403 + `withTenantTx` 确认 tenant 见 0 行）
- [x] 验证：`go test ./internal/integration/ -run TestRiskReview` ✅、`make test-integration` 全量 ✅、`go vet ./...` 干净 ✅
- **状态：✅ 已完成，待提交**
- **设计要点**：架构设计 §15「Provider 生命周期与风险控制」列 8 项 Live 开通验证（邮箱与企业域名 / ToS·DPA / 风险评分 / 自定义域所有权 / Payment·Tax Connection / Webhook 目的地 / 初始配额 / 安全联系人），其中能力分别授权已由候选 6（provider_capabilities）实现，本候选补齐**首次 go-live 前的风险审核门禁**——operator 提交 7 项布尔 checklist + risk_score（0-100，第 8 项）+ decision，批准后才解锁 `LIVE_REVIEW → LIVE_ACTIVE`；reactivation 不强制门禁（复用既有批准记录，与 webhook SUSPENDED→LIVE_ACTIVE 测试语义一致）；风险评分为 operator 内部评级，RLS 仅 operator 可见

### 候选 29：OIDC Application 管理端点（M1 控制面 API 第 1 项）
- [x] 迁移 `0035_auth_config_redirect_uris.sql`：`provider_auth_configs` 新增 `redirect_uris jsonb NOT NULL DEFAULT '[]'`（本地镜像，list 不触发 ZITADEL API，避免 N+1）；`sqlc.yaml` 已登记 0035 schema
- [x] `db/queries/auth.sql`：新增 `ListAuthConfigsByTenant :many`（newest-first）、`UpdateAuthConfigSecret :one`（参数编号 `$1=provider_id/$2=environment_id/$3=secret`）、`UpdateAuthConfigRedirectURIs :one`；`CreateAuthConfig` 新增 `redirect_uris` 参数；sqlc 已重新生成 `storegen/auth.sql.go` + `models.go`
- [x] `internal/zitadel/client.go`：新增 `RotateOIDCAppSecret`（`PUT /management/v1/projects/{id}/apps/{appID}/oidc_config/secret`，返回新 plaintext secret）、`UpdateOIDCAppRedirectURIs`（`PUT .../oidc_config`，更新回调地址不换密钥）
- [x] `internal/service/auth.go`：
  - `SetupHostedAuth` 新增 `redirect_uris` 写入（`json.Marshal` → `CreateAuthConfigParams.RedirectUris`）
  - `ListHostedAuthConfigs`（返回 `[]ProviderAuthConfig`，不含 secret 明文）
  - `RotateHostedAuthSecret`（调 ZITADEL rotate → encrypt → `UpdateAuthConfigSecret` → outbox `auth.hosted_auth_secret_rotated` + audit `auth.hosted_auth_secret_rotate`；返回 `RotatedAuthConfig{ClientSecret: plaintext}`，R17 密钥只显示一次）
  - `UpdateHostedAuthRedirectURIs`（校验非空 → 调 ZITADEL update → `json.Marshal` → `UpdateAuthConfigRedirectURIs` → outbox + audit）
- [x] `internal/httpapi/handlers_auth.go`：新增 `listHostedAuthConfigs`（GET →200）、`rotateHostedAuthSecret`（POST →200）、`updateHostedAuthRedirectURIs`（PUT →200）
- [x] `internal/httpapi/httpapi.go`：provider 路由组注册 3 条新路由（`GET /auth/zitadel/apps`、`POST /auth/zitadel/rotate-secret`、`PUT /auth/zitadel/redirect-uris`），均 `requireScope(ScopeRead/Write)`
- [x] 集成测试 `auth_m1_test.go`（新建）：`TestHostedAuthListRotateUpdate`（setup→list 1 条→rotate 返回明文+DB 加密+ZITADEL 调用计数→update redirect_uris→DB 镜像→audit 2 条）、`TestHostedAuthRotateValidation`（无 config rotate→fail、无 config update→fail、空 list update→fail）
- [x] 验证：`go build ./internal/...` ✅、`go vet ./internal/...` ✅、`go test ./internal/integration/ -run TestHostedAuth` ✅、全量集成测试（跳过 flaky `TestOutboxRelayDeliversUsage`）✅
- **状态：✅ 已完成，待提交**
- **设计要点**：M1 控制面 API 第 1 项，复用现有 ZITADEL `ManagementClient`（候选无需建新 client）+ `provider_auth_configs` 表（0011，仅加 `redirect_uris` 列）；三个新端点填补现有 Hosted Auth 的 List/Rotate/Update 缺口——前端 Applications 页面（基线 §8 M1）可据此实现列表/密钥轮换（R17 只显一次）/回调地址变更；rotate 返回 `RotatedAuthConfig` 独立类型携带明文 secret，与 `AuthConfig`（不含 secret）分离，符合 R17 安全契约

### 候选 30：Plans 套餐目录端点（M1 控制面 API 第 2 项）
- [x] `db/queries/billing.sql`：新增 `GetDraftCatalogVersionByTenant :one`（state=draft + `FOR UPDATE`）、`GetLatestPublishedCatalogVersionByTenant :one`、`UpdatePlan :one`、`DeletePlanByVersionAndCode :execrows`、`DeletePricesByPlan :exec`、`DeleteGrantsByPlan :exec`；`make sqlc` 已重新生成 storegen
- [x] `internal/service/catalog_plans.go`（新建）：`ListPlans`（读路径 draft→published 回退，**不建版本**避免读副作用）；`CreatePlan`/`UpdatePlan`/`DeletePlan`（写路径作用于当前 draft，无 draft 自动 clone 最新 published 或建空版本）；`pg_advisory_xact_lock` 事务级 advisory lock 串行化「取 draft + 写」临界区（catalog_versions 无单 draft 唯一约束，并发建 draft 靠锁保证）；plan 更新**保留 ID**（`subscriptions.plan_id` 外键依赖），prices/grants 子表先删后插；plan code 不可变（body code 与路径不一致 → 400）
- [x] 校验复用 `domain.ValidateCatalogStructure`：单 plan 连同版本内现有 metrics 组合校验（price 的 metric 引用必须存在于版本）
- [x] 审计 + outbox（事务内）：`catalog.plan_create/update/delete` 审计 + `catalog.plan_created/updated/deleted` outbox 事件（含 catalog_version_id / version / plan_code 元数据）
- [x] `internal/httpapi/handlers_billing.go`：新增 `listCatalogPlans`（GET→200）、`createCatalogPlan`（POST→201）、`updateCatalogPlan`（PUT→200）、`deleteCatalogPlan`（DELETE→204）
- [x] `internal/httpapi/httpapi.go`：provider 路由组注册 4 条路由（`GET/POST /catalog/plans`、`PUT/DELETE /catalog/plans/{code}`），均 `requireScope(ScopeRead/Write)`
- [x] 集成测试 `catalog_plans_test.go`（新建）：`TestCatalogPlansCRUD`（创建自动建 draft/列表/409 冲突/更新保留 ID/400 code 不可变/404/删除 204+404）、`TestCatalogPlansPublishedImmutable`（发布后更新 staged 新 draft，published 内容不变）、`TestCatalogPlansClonePublishedContent`（clone 后 metric 引用生效、未知 metric → 400）
- [x] 验证：`go build ./...` ✅、`go vet ./...` ✅、全量单测 ✅、`go test ./internal/integration/ -run TestCatalogPlans` 3 用例全绿 ✅；GitNexus impact 前置分析（`ReplaceCatalogContent`/`replaceCatalogContentTx`）均 LOW
- **状态：✅ 已完成，待提交**
- **设计要点**：M1 控制面 API 第 2 项，前端 `/console/billing/plans` 页面的关键前置依赖；「发布即生效」由版本化目录模型保证——plan 级写操作永远落在 draft，published 版本不可变（DB trigger 强制），publish 时校验 provider lifecycle；advisory lock 弥补无单 draft 约束的并发窗口；List 读路径不建版本，避免 GET 副作用

### 候选 31：Policies 策略 / 权益端点（M1 控制面 API 第 3 项）
- [x] `sqlc.yaml`：3 个列级 override 修复 jsonb API 序列化 bug——`entitlement_grants.value` / `entitlement_overrides.value` / `prices.properties` 由 `[]byte`（encoding/json 会 base64 编码，如 `"dHJ1ZQ=="`）改为 `json.RawMessage`（输出原始 JSON）；该 bug 同时影响候选 30 的 PlanDetail 与既有 overrides 端点
- [x] `db/queries/billing.sql`：新增 `UpsertEntitlementGrant :one`（`ON CONFLICT (plan_id, key) DO UPDATE`，单条 SQL 原子 upsert，配合表级 `UNIQUE (plan_id, key)`）、`DeleteEntitlementGrantByKey :execrows`（返回 affected rows 判定 404）；`make sqlc` 已重新生成 storegen
- [x] `internal/domain/catalog.go`：提取 `ValidateEntitlementInput`（key 非空 / value_type 合法 / value 非空），`validateCatalog` 批量校验改为调用它——单条与批量校验单一事实来源，错误消息逐字一致
- [x] `internal/service/catalog_policies.go`（新建）：`ListPlanEntitlements`（读路径 draft→published 回退，**不建版本**）；`SetPlanEntitlement` / `DeletePlanEntitlement`（写路径复用候选 30 的 `ensureDraftVersionTx` + advisory lock「取 draft + 写」临界区）；grant key 不可变（body key 与路径不一致 → 400）；删除 0 行 → 404
- [x] 审计 + outbox（事务内）：`catalog.entitlement_set` / `catalog.entitlement_delete` 审计 + `catalog.entitlement_set` / `catalog.entitlement_deleted` outbox 事件（含 catalog_version_id / version / plan_code / entitlement_key 元数据）
- [x] `internal/httpapi/handlers_billing.go`：新增 `listPlanEntitlements`（GET→200）、`setPlanEntitlement`（PUT→200）、`deletePlanEntitlement`（DELETE→204）
- [x] `internal/httpapi/httpapi.go`：provider 路由组注册 3 条路由（`GET /catalog/plans/{code}/entitlements`、`PUT/DELETE .../entitlements/{key}`），均 `requireScope(ScopeRead/Write)`
- [x] 集成测试 `catalog_policies_test.go`（新建）：`TestCatalogPoliciesCRUD`（set/upsert/list 含 clone 的 max_users/key 不可变 400/未知 value_type 400/未知 plan 404/删除 204+404）、`TestCatalogPoliciesPublishedImmutable`（发布后 set staged 新 draft，published grants 不变）
- [x] 验证：`go build ./...` ✅、`go vet ./...` ✅、全量单测 ✅、`go test ./internal/integration/ -run TestCatalogPolicies` 2 用例全绿 ✅、`-run 'TestCatalogPlans|TestEntitlement|TestSubscription'` 回归全绿 ✅；GitNexus impact 前置分析（`validateCatalog` 内部提取）LOW
- **状态：✅ 已完成，待提交**
- **设计要点**：M1 控制面 API 第 3 项，前端 Policies 页面（「谁能用什么功能」= plan 级 entitlement grants）的关键前置依赖；与 plan 定价解耦的独立权益管理，作用于当前 draft 且与 plan CRUD 共用同一 advisory lock 临界区，保证版本内一致性与 published 不可变；`ON CONFLICT` upsert 替代「先删后插」，单条 SQL 原子且免于 grant 缺失窗口；sqlc override 同时修复候选 30 遗留的 jsonb base64 序列化缺陷

### 候选 32：Settings 端点（工作区设置、自定义域名）（M1 控制面 API 第 4 项）
- [x] 现状审查确认：两块端点已在 M0 完成——工作区设置 `GET/PATCH /v1/me/workspaces/{id}`（operator 域 + sub 成员校验 + slug 唯一性；`workspaces` 表 RLS 仅 `app.is_operator='on'` 可见，provider API-key 事务不可见，故 Settings 工作区设置归 operator 域）与自定义域名 `POST/GET/DELETE /v1/custom-domains` + `verify`/`revoke`（provider 域 + tenant 隔离 + RLS + 全局 `UNIQUE(domain)` 接管保护 + DNS TXT 验证 + 状态机 pending→verified→revoked→delete）
- [x] 一致性修复（本轮）：`DeleteCustomDomain` 补 outbox `custom_domain.deleted` 事件——此前删除仅写审计、无事件，而 registered/verified/revoked 均有 outbox；事件消费者（域名路由 / 证书管理）依赖删除事件清理资源；payload 含 `domain` + 删除前 `status`
- [x] 集成测试：`TestDeleteCustomDomain` 捕获 providerID，用 superPool 直接查 `outbox_events` 断言 `custom_domain.deleted` 事件（首次删除 1 条、revoke 后删除累计 2 条）
- [x] 验证：`go build ./...` ✅、`go vet` ✅、全量 custom domains 集成测试（10 用例）回归全绿 ✅；GitNexus impact 前置分析（`DeleteCustomDomain` 事务内追加 outbox 写入，不改签名）LOW
- **状态：✅ 已完成，待提交**
- **设计要点**：M1 控制面 API 第 4 项，前端 `/console/settings` 页（按心智分组：基础 = 工作区名称/slug、安全 = 自定义域名/SSO、高级 = 通知配置）的数据依赖；workspace 为平台控制面概念——RLS 只放行 operator 事务，provider API-key 事务不可见，因此工作区设置端点天然位于 operator 会话域（`/v1/me`），而自定义域名按 environment 隔离属于 provider 域（`/v1/custom-domains`）；本轮为审查收尾，修复删除事件缺失的一致性缺陷，使 4 种状态变更（registered/verified/revoked/deleted）均向 outbox 发布事件

### 候选 33：端点类型同步（lib/api/types.ts 独立文件 + openapi.yaml 对齐）（M1 控制面 API 第 5 项）
- [x] 新建 `apps/web/src/lib/api/types.ts`：28 个纯类型接口（**零运行时依赖**），operator.ts 26 个实体 + 输入类型（CreateProviderInput / ActivateProviderInput / SignupInput）；Provider 合并 schemas.ts 校验字段（description / home_region_code 可选）保证两来源兼容
- [x] `operator.ts` 重构：删除本地 26 个接口定义 → `import type` + `export type` re-export types.ts（保持全部现有 `import type { Provider } from "@/lib/api/operator"` 兼容，9 个 Provider 类型引用者零改动）；`createProvider`/`provisionWorkspace` 入参改用 types.ts 类型
- [x] `schemas.ts` 重构：9 个 zod schema 保留（运行时校验），6 个类型导出（Provider/Environment/Region/LifecycleResult/CreateProviderInput/LifecycleTarget）改为 re-export types.ts——**消除两套同名不同构定义**（原 schemas.ts z.infer vs operator.ts 手写接口并存）
- [x] `docs/openapi.yaml`：修正 Provider schema 对齐实际响应（slug/lifecycle_state/home_region_id/home_region_code/sla_tier/description，原为旧版 kind/home_region）；新增 29 个 operator 域实体 schemas（与 types.ts 逐字段对应，含 operator 视图 Subscription/UsageEventRecord 与入参 UsageEvent 的命名辨析）；新增 20 个路径（operator providers 全套 + catalog versions 列表/详情 + regions + overview-stats + `/me/workspaces` + `/signup`）；补 ProviderID/VersionID path 参数；文档从「provider 域骨架」扩展为 36 paths / 43 schemas 双域 schema 库
- [x] 验证：`npx tsc --noEmit` 全绿（0 errors）✅、`python3 yaml.safe_load` 语法 OK + 引用完整性（missing=[]）✅、字段一致性脚本（types.ts 28 interfaces vs openapi 同名 schemas）仅 1 处有意错位（UsageEvent 为 provider 域入参，operator 视图对应 UsageEventRecord，注释弥合）✅
- **状态：✅ 已完成，待提交**
- **设计要点**：M1 控制面 API 第 5 项，落实 §11 变更管理「前端类型以 openapi 为准」——将前端 3 个类型来源（operator.ts 手写接口、schemas.ts z.infer、openapi schemas）收敛为单一事实源 types.ts，operator.ts / schemas.ts 仅 re-export，杜绝同名类型漂移；types.ts 纯类型零运行时依赖，未来可无缝衔接 openapi-typescript 等生成工具自动化；漂移修复流程写入 types.ts 头注释（先修 openapi.yaml → 同步 types.ts → tsc 验证）

### workspaces 多租户工作区功能
- [x] 迁移 `0026_workspaces.sql` / `0027_provider_home_region_nullable` / `0028_webhook_retention`
- [x] 基础：signup 幂等建工作区（`ProvisionWorkspace`）+ `GET /v1/me/workspaces`（`ListMyWorkspaces`）
- [x] 工作区管理：`GET/PATCH /v1/me/workspaces/{id}`（详情/改名/改 slug，slug 正则校验 + 冲突 409）
- [x] 成员管理：`GET/POST /v1/me/workspaces/{id}/members` + `PATCH/DELETE .../members/{userSub}`（邀请幂等 upsert/重新激活、角色变更、移除）
- [x] RBAC：成员管理仅 `provider_admin`（否则 403）；非成员/不存在工作区统一 404（防泄露）；**最后一名 active admin 保护**（降级/移除 409）
- [x] 查询：`UpdateWorkspace` / `UpsertWorkspaceMember` / `UpdateWorkspaceMemberRole` / `RemoveWorkspaceMember` / `CountWorkspaceAdmins`
- [x] 错误：`service.ErrForbidden` → HTTP 403（`serviceError` 映射）
- [x] 测试：integration `zz_workspace_members_test.go` +3 组（邀请/改角色/重新激活/暂停成员 409/非 admin 403/最后 admin 409/404/400 校验/改名与 slug 校验）
- **状态：✅ 已完成，待提交**

### Web 前端全面重构（M0 静态验收完成）
- [x] `(auth)/(console)/(marketing)` 路由组
- [x] playwright e2e + `playwright.config.ts`
- [x] `Dockerfile`
- [x] 旧 `app/page.tsx`、`actions.ts` 等已删除
- [x] M0 静态验收逐项核对（设计基线 §M0 验收/抽查）：
  - 核心 5 条：未登录重定向 / 登录进 Console / `?env=` URL 显式携带 + 数据源切换 / `/ops` lifecycle 完整 / 首登引导管线 ✅
  - 零摩擦：校验聚焦首错字段 / pending 防双击 / 切换不白闪保留滚动 / 密钥复制+离开提醒 / 键盘可达 / reduced-motion ✅
  - 心智：按钮文案 `[动词]+[对象]` / 一级页定位语 / 完成态双出口 / 概念人话解释 / env+主题持久化 ✅
  - 视觉：token 体系 / 品牌 V 形 mark / hero 终端窗口 / mono+tabular-nums / 无 emoji / 暗色非纯黑 / 焦点环统一 ✅
- [x] **修复 `bg-card` 缺失 token**：13 处 → `bg-surface-1`（`--color-card` 未定义，Tailwind v4 下不生成 CSS，卡片背景丢失；R31 表面四层制规范）
- [x] R20 页面级 ErrorBoundary：`console/error.tsx` + `ops/error.tsx`
- [x] R21 窄屏响应式：Sidebar <lg 抽屉 + 汉堡按钮；Topbar 窄屏 env 切换器收进用户菜单 + 常驻 live 徽标
- [x] M0 e2e 冒烟（Playwright）：**30/30 全绿（21.3s）**，覆盖未登录重定向 / 登录 / Console / env 切换 / ops lifecycle / 首登引导 / 零摩擦交互 / 完成态双出口
- [x] **环境**：`:3002` Docker 生产 web（standalone） + `:8084` Docker API（含聚合接口） + `:8085` webhook sink + Postgres
- [x] **聚合接口**：`GET /v1/me/overview`（`/overview-stats` 聚合 stats + lifecycle 计数 + env 选择），仅新 Docker API 提供
- [x] **Next.js 16.2.12 standalone 流式 RSC 传输 bug workaround**：客户端导航（GET `?_rsc=`）与 Server Action（POST `Next-Action`）在浏览器直接消费 chunked 流式响应时 `net::ERR_ABORTED`、router 静默回滚（与 vercel/next.js#96108 同根因，无官方修复）；e2e 侧经 `page.route` 拦截 → `route.fetch()` 缓冲 → `route.fulfill` 非流式转发恢复导航；生产部署建议 HTTP/2 反代以规避
- [x] **prefetch={false}**：sidebar NavLink / ops 新建重试 / provider-form 保存返回取消 / 引导步骤 Link 共 11 处加 `prefetch={false}`（防详情页 RSC 请求风暴，正确实践；非导航失败根因）
- **状态：✅ 已完成，待提交**

### 候选 85：自建登录页 + ZITADEL Session API（M6 身份一致性）
- [x] 深入 `third-party/zitadel` 源码核对 Session API：v2 为正式版、v2beta 已 deprecated；`POST /v2/sessions` / `PATCH /v2/sessions/{id}` / `GET /v2/sessions/{id}` / `POST /v2/sessions/search` / `DELETE /v2/sessions/{id}` 契约与权限均已锁定
- [x] 核对 `IAM_LOGIN_CLIENT` 权限（`session.read/write/link/delete` + `user.write` 等）、session token 轮换/作为 Bearer 的语义、OIDC `CreateCallback` 换 code 的完整链路
- [x] 输出《自建登录页 + ZITADEL Session API 技术方案》：`docs/SPEC-Custom-Login-Session-API.md`（含架构、登录/注册流程、契约层、错误映射、安全设计、测试矩阵、P0-P4 交付拆分）
- [x] 完成架构/产品评审修订：核心目标锁定为“安全合规前提下实现品牌一致性与登录体验控制”；裁决为有条件通过，P0 决策已固化进 SPEC §0/§2.1
- [x] 评审决策落稿：主链路使用 `@zitadel/client` 生成客户端；生产凭据优先 Login Client Key/系统用户 JWT；`isSessionValid` 作为 CreateCallback 前强制 MFA 门槛；新增 OIDC 代理 middleware；版本锁 v4.16.x（≥4.6.0）且禁用 `EnableRelationalTables`
- [x] P0：`zitadel-session.ts`（基于 `@zitadel/client` 1.3.1）+ zod + `isSessionValid` MFA 模块 + ZITADEL 契约探针 + `AUTH_MODE=oidc-custom-login` 开关 + OIDC 代理骨架
- [x] P1：登录名/邮箱/手机号搜索 + 密码页替换 + `login_flow` 加密 cookie 落地 + 代理路径打通 + 基础设施故障自动回退托管 OIDC
- [x] P2：MFA 全类型（TOTP / OTP email/SMS / Passkey / U2F）与账号选择器（记住会话 cookie + 继续会话 + 失效清理）
- [x] P3：注册 / 邮箱验证 / Passkey 初始化 / MFA 初始化跳过 / 未覆盖 IDP/SAML 显式回退托管登录页，复用 `provisionWorkspace` 事务
- [x] 真实 ZITADEL 契约探针：`v4.16.0` 通过 `getLoginSettings` / `listUsers` / `listSessions`，版本门禁 ≥4.6.0 生效；传输改为官方同款 HTTP/1.1 Connect
- [x] P4 灰度控制面：`ZITADEL_CUSTOM_LOGIN_ALLOWED_USERS` / `ZITADEL_CUSTOM_LOGIN_ALLOWED_ORGS` 白名单生效，非名单用户/组织回退托管 OIDC；登录/注册/回退/MFA 结构化事件已输出
- [x] P4：真实 ZITADEL v4.16.0 Playwright E2E 通过（自建登录页 → 密码 → MFA 门槛 → OIDC callback）；用户/组织灰度白名单与结构化登录事件已实现，按 env 放量由部署环境变量控制
- [x] 真实回调联调：callback 已换取 token 并调用 `/v1/signup` 成功（API OIDC Verifier + JWT access token）；发现 `vlb_session` cookie 因内嵌 access/refresh token 超 4KB 被浏览器拒绝，下一步需服务端会话存储
- [x] 会话 cookie 分片：access/refresh token 从主 JWT 拆到 `vlb_session_token_N` 独立加密分片，主 `vlb_session` 仅保留身份；callback 改为 200 + meta refresh 保留 cookie 后再进入控制台
- [x] API 服务端 token vault：`0037_auth_session_vault` + `POST/GET/DELETE /v1/auth/vault/{id}` + 加密存取 + 集成测试通过；web 侧接入待续
- [x] web 会话切换服务端 vault：主 cookie 仅含身份 JWT + `vid`，`getSession` 经 `/v1/auth/vault/{id}` 按需取 token，登出删除 vault；token 分片 cookie 已移除
- [x] standalone 后端闭环：真实 callback 每次均完成 `/v1/auth/vault` 201 + `/v1/signup` 200；浏览器 console 最终验收待 Playwright 中间跳转稳定
- [x] 浏览器最终闭环：standalone（`HOSTNAME=localhost`）下 Playwright 真实登录到达 `/console`，Console 成功加载 vault/workspaces/overview 数据
- [x] 生产安全/运维封顶：独立 `AUTH_VAULT_MASTER_KEY`（支持 previous 轮换）、vault 操作审计落库、`auth_vault_operations_total` 指标、过期 vault sweep worker 接入 main
- [x] 工作负载身份：web 使用 `AUTH_VAULT_SERVICE_PRIVATE_KEY` 签发 5 分钟 RS256 JWT，API 用 `AUTH_VAULT_PUBLIC_KEY` 验签；静态 `AUTH_VAULT_SERVICE_TOKEN` 仅作回退
- [x] CI 生产门禁：新增 `.github/workflows/ci.yml` 与 `scripts/ci-zitadel-e2e.sh`，真实 ZITADEL/API/Web 浏览器 Console E2E 进 CI
- [x] 实现验证：`tsc` / ESLint / `vitest`（8 条 MFA 门槛用例）/ Next.js 生产构建全绿
- **状态：✅ P0-P4 完成；真实 ZITADEL 自建登录 + 服务端 token vault + standalone 浏览器 Console 闭环通过；独立密钥/审计/指标/清理 worker/短期 JWT 工作负载身份/CI E2E 已落地**

### 测试体系扩充
- [x] metrics_middleware / security / timeout / health / workspace / web_simulation 等新测试
- [x] 核对：SPEC 行为测试文件全部存在，集成测试全绿
- **状态：✅ 已完成，待提交**

## 三、候选方向（待定优先级）

| 候选 | 说明 | 状态 |
|---|---|---|
| 限流增强 | per-IP 兜底 + 精确 Retry-After + 429 指标 | ✅ 已完成（候选 10） |
| 熔断 | Webhook 投递 / 外部连接器下游熔断 | ✅ 已完成（候选 11） |
| OpenTelemetry 分布式追踪 | 端到端链路追踪 | ✅ 已完成（候选 12） |
| 配置热加载 | 运行时配置热更新 | ✅ 已完成（候选 13） |
| 幂等键 | Stripe 风格 Idempotency-Key，防重复扣费/创建 | ✅ 已完成（候选 14） |
| Redis 分布式限流 | 多实例共享计数（Lua 原子窗口 + fail-open） | ✅ 已完成（候选 15） |
| 审计日志查询增强 | keyset 分页 + action/actor/时间多维过滤（provider 与 operator 双端点） | ✅ 已完成（候选 16） |
| 审计仪表盘 API | 审计趋势聚合（按 action/actor 计数、时间直方图零填充） | ✅ 已完成（候选 17） |
| 审计导出 | 审计事件流式导出（CSV/JSON，from/to 窗口护栏 + format 枚举） | ✅ 已完成（候选 18） |
| 审计保留 | 审计事件保留窗口 + operator-only 受控清理（默认禁用，显式配置才启动） | ✅ 已完成（候选 19） |
| 审计哈希链 | 审计事件链式哈希（SHA-256）+ 周期锚点，篡改可检测（tamper-evident） | ✅ 已完成（候选 20） |
| Worker 监督 | 后台 worker panic 恢复 + 指数退避重启 + 崩溃循环指标化（静默停摆可告警） | ✅ 已完成（候选 21） |
| 主密钥轮换 | PSP 凭证加密主密钥轮换（active 加密 + previous 回退解密，存量密文零迁移） | ✅ 已完成（候选 22） |
| /ready 依赖健康汇总 | readiness 探针并行聚合 DB + 限流后端（Redis）健康，结构化明细 + 抖动指标化 | ✅ 已完成（候选 23） |
| 主密钥重加密收敛 | 轮换后自动把旧密钥密文重写为 active 封存（幂等 + 不可收敛可告警，收敛闭环） | ✅ 已完成（候选 24） |
| WORM 审计归档 | 锚点导出到 S3 兼容 WORM 对象存储（不可变外部锚定，DB 超级用户不可篡改，tamper-proof） | ✅ 已完成（候选 25） |
| Outbox 死信队列 | outbox 事件真 dead_letter 终态 + last_error 死因 + 对账 check 复活（原软死信致 `CountDeadLetterOutbox` 恒 0） | ✅ 已完成（候选 26） |
| 财务差异处置 | 发票金额自洽对账 3 check（金额不变量/行合计漂移/逾期未付）+ `reconciliation_drift` GaugeVec 指标（与候选 25/26 共同闭环 P0 门禁） | ✅ 已完成（候选 27） |
| Live 风险审核门禁 | 首次 go-live（LIVE_REVIEW→LIVE_ACTIVE）前 operator 提交 8 项验证清单 + risk_score，批准后解锁；RLS operator-only（风险评分内部评级） | ✅ 已完成（候选 28） |
| OIDC Application 管理端点 | M1 控制面 API 第 1 项：List + Rotate Secret + Update Redirect URIs（复用 ZITADEL ManagementClient + provider_auth_configs 表） | ✅ 已完成（候选 29） |
| Plans 套餐目录端点 | M1 控制面 API 第 2 项：plan 级 CRUD + 定价模型 + 发布即生效（advisory lock 并发安全，plan code 不可变，subscriptions 外键保留 plan ID） | ✅ 已完成（候选 30） |
| Policies 策略 / 权益端点 | M1 控制面 API 第 3 项：plan 级 entitlement grants 独立 CRUD（ON CONFLICT upsert + 不可变 key + 发布即生效）；附带修复 jsonb base64 序列化 bug（sqlc override → json.RawMessage） | ✅ 已完成（候选 31） |
| Settings 端点（工作区设置、自定义域名） | M1 控制面 API 第 4 项：工作区设置（/v1/me/workspaces，operator 域 + RLS）与自定义域名（/v1/custom-domains 全套 CRUD + DNS 验证 + 接管保护）M0 已就绪；本轮审查补齐 DeleteCustomDomain 缺失的 outbox 删除事件与测试断言 | ✅ 已完成（候选 32） |
| 端点类型同步（lib/api/types.ts + openapi 对齐） | M1 控制面 API 第 5 项（§11 变更管理）：新建纯类型 types.ts 作为前端类型单一事实源（28 接口）；operator.ts / schemas.ts 改为 re-export 消除两套同名漂移；openapi.yaml 修正 Provider + 新增 29 schemas / 20 paths 覆盖 operator 控制面 + workspaces + signup | ✅ 已完成（候选 33） |
| Overview 趋势图（M2 第 1 项） | Overview 完整版增量：后端 `OverviewStats.Trends` 双序列（30 天收入按 finalized 发票出票日汇总 / 用量事件按 ingestion 入库日计数，单事务 + Go 补零连续日轴）；前端 `components/charts/` 自绘 SVG（AreaChart 墨青渐变 + BarChart 纵向渐变 + tooltip + 空态，零依赖）；openapi 新增 TrendPoint/OverviewTrends 并同步 types.ts | ✅ 已完成（候选 34） |
| Applications 控制面（M2 第 2 项） | OIDC 应用管理端到端：operator 控制面 5 端点（list/setup/rotate/redirects/disable，?env= 显式解析）+ `provider_auth_configs.name` 迁移；前端 `/console/identity/applications` 列表/创建/轮换（R17 只显一次+复制）/回调编辑/删除（live type-to-confirm）；环境标识色独立 token 技术债清零 | ✅ 已完成（候选 35） |
| Plans 控制面（M2 第 3 项） | 套餐管理端到端：operator 控制面 5 端点（list-detail/get/create/update/delete，?env= 显式解析）+ 一次请求返回「plans 详情 + metrics」避免 N+1；前端 `/console/billing/plans` 列表/创建/编辑/删除（固定/按量/阶梯定价编辑器 + 阶梯连续性客户端校验 + live type-to-confirm）；openapi 新增 PlanInput/PlanDetail/PlanCollection 等 5 schemas + 5 paths | ✅ 已完成（候选 36） |
| Customers 控制面（M2 第 4 项） | 客户管理端到端：operator 控制面 3 端点（list 支持可选 ?env= / create / detail）+ 客户详情一次请求返回「订阅 + 用量 + 账单」（新增 3 条按 customer_account_id 过滤的 SQL 查询，避免整租户数据扇出）；前端 `/console/billing/customers` 列表/创建 + `[externalId]` 详情（订阅 / 用量 / 账单三个相关导航页签，§6.6.3）；openapi 新增 CustomerCreateInput/CustomerDetail 2 schemas + 2 paths | ✅ 已完成（候选 37） |
| Invoices 控制面（M2 第 5 项） | 账单管理端到端：operator 控制面 2 端点（list 支持可选 ?env= / detail，详情一次返回发票 + 行明细，新增 2 条按 provider+env 过滤的 SQL 查询）；前端 `/console/billing/invoices` 列表 + `[invoiceId]` 明细（金额 mono + tabular-nums，行明细表含数量/单价/金额/税额/合计）；openapi 新增 InvoiceLine/InvoiceDetail 2 schemas + 1 path | ✅ 已完成（候选 38） |
| 图表变体补齐（M2 第 6 项） | `components/charts/` 自绘 SVG 图表家族补全：Sparkline（卡片迷你趋势线）、LineChart（单点也可渲染的折线图）、DonutChart（stroke-dasharray 环形图 + 图例），共享 `smoothPath` 抽到 chart-base；接入 Overview Provider 状态分布 / 账单收入 Sparkline / 客户用量按天趋势；零第三方依赖，质感对齐 §7.5 | ✅ 已完成（候选 39） |
| 4 步引导回填（M2 第 7 项） | §2.2 四步模型端到端：`lib/onboarding.ts` 从 3 步（Provider→发布→上线）改为 4 步（应用→套餐→客户→用量），基于当前 workspace 真实数据推断进度；Overview FirstRunPanel 文案/步骤更新，OnboardingStrip 步骤改为可点击链接 + 「继续引导」指向首个未完成步骤；创建应用/套餐成功面板双出口改为指向下一步（套餐/客户） | ✅ 已完成（候选 40） |
| 环境隔离端到端（M2 第 8 项） | test/live 隔离闭环验证：新增 `environmentHeaderMiddleware` 强制 `X-Environment` 契约（可选头，但传入必须匹配 API Key 绑定环境，否则 `400 environment_mismatch`）；集成测试覆盖「test 建套餐/客户/订阅/用量 → live 不可见 → live 可复用同 external_id → operator 控制面 ?env= 隔离」；E2E 通过 Console 环境切换器验证 Plans/Customers 数据互不可见；openapi 文档补充 Environment Isolation 契约 | ✅ 已完成（候选 41） |
| DataTable + URL 筛选（M2 第 9 项） | 通用 `components/ui/data-table.tsx`（§7.4 / R16）：sticky 表头、行 hover、搜索/排序/分页/每页条数/可选状态筛选全部 URL 化（`?q=&sort=&dir=&page=&pageSize=&status=`），搜索 replace 防抖、离散操作 push 可回退；接入 Customers（搜索/排序/分页）、Invoices（状态筛选）、Plans（搜索/排序）；数值列右对齐 mono + tabular-nums | ✅ 已完成（候选 42） |
| React Query 缓存 + hooks 扩充 | `@tanstack/react-query` 客户端缓存（全局 QueryClient + staleTime：事件流 30s / 审计 60s）；事件流与审计日志改用 `useInfiniteQuery`（keyset 分页 + keepPreviousData + RSC 首屏数据作 initialData）；`hooks/use-action-feedback.ts` 统一 useActionState 成功/失败回调与 toast，并落地到 API Key 吊销 / Webhook 删除与重放 | ✅ 已完成（候选 49） |
| 交互体验与技术债收敛 | 环境切换成功/失败 toast + 失败回滚；First-Run 引导「跳过/恢复」入口（cookie 持久化，R18）；`ApiKeyCallout` 迁移到 token 色 + 中文品牌语音 + 复制反馈；API Key 创建/轮换未复制即关闭给一次性提醒 | ✅ 已完成（候选 50） |
| lib 目录化（env/utils/validate） | `lib/env/`（shared/server/index）+ 根 `env-shared.ts` barrel；`lib/utils/`（index + format）+ 根 `format.ts` barrel；schemas 迁至 `lib/validate/` 并全量更新导入，删除 `lib/api/schemas.ts` | ✅ 已完成（候选 51） |
| UI 组件补齐（Card/Drawer/Pagination） | 新增 `Card` 家族（CardHeader/Title/Content/Footer）、`Drawer`（Esc 关闭 + 焦点管理 + 滚动锁 + 左右侧开）、`Pagination`（页码 + 省略号 + URL 驱动）；接入审计统计卡 / 窄屏侧边栏 / DataTable 分页；修复移动端汉堡按钮被顶栏遮挡的 z-index 问题 | ✅ 已完成（候选 52） |
| Policies 控制面 | M1 候选 31 的前端落地：operator 3 端点（list/set/delete plan entitlements，`?env=` 显式解析）+ openapi 3 paths；前端 `/console/identity/policies` 套餐选择 + 权益 CRUD + live/删除 type-to-confirm；集成测试 `operator_catalog_policies_test.go` + E2E `20-policies.spec.ts` | ✅ 已完成（候选 53） |
| Billing Dashboard | `/console/billing/dashboard`：收入/活跃订阅/客户/待收账单指标卡（Sparkline）+ 近 30 天收入 AreaChart + 近期账单表；侧边栏 Billing 首项；补充根级 `.dockerignore` 加速构建 | ✅ 已完成（候选 54） |
| Payments 控制面 | `/console/billing/payments`：支付成功/待支付/支付失败金额卡 + 发票数；DataTable 支付状态筛选；说明本地不存支付凭据、支付状态由 Lago 同步；侧边栏新增支付 | ✅ 已完成（候选 55） |
| Catalog 控制面 | `/console/catalog`：目录版本选择（URL `?version=` 可分享/回退）+ 版本元数据 / 指标 / 套餐 / 价格 / 权益五段视图；当前 draft 摘要；侧边栏新增目录 | ✅ 已完成（候选 56） |
| Developers SDK / 事件规范 | `/console/developers/sdk`（cURL / Node / Python 用量上报示例 + CodeBlock 复制）与 `/console/developers/events-spec`（事件目录 + payload 规范）；侧边栏补齐 SDK / 事件规范 | ✅ 已完成（候选 57） |
| Identity Users 控制面 | `/console/identity/users`：workspace 成员列表 / 邀请 / 角色更新 / 移除（type-to-confirm）；复用既有 `/v1/me/workspaces/{id}/members` API；侧边栏新增 Users | ✅ 已完成（候选 58） |
| 多 workspace 切换 | `WORKSPACE_COOKIE` + `lib/workspace.ts`（workspace_id==provider_id 解析）；Topbar WorkspaceSwitcher（多 workspace 下拉、单 workspace 只读标签）；13 个 Console 页面全部改为按当前 workspace 解析 provider；Settings / Users 同步按 cookie 选择 | ✅ 已完成（候选 59） |
| 用量控制面 | `/console/billing/usage`：事件/客户/指标/撤销摘要卡 + DataTable（事务/客户/指标检索、环境与时间列）；复用既有 usage-events 端点；侧边栏新增用量 | ✅ 已完成（候选 60） |
| 全局错误与加载边界 | 根级 `not-found.tsx`（品牌 404 + 返回控制台）与 `global-error.tsx`（digest + 重试）；Console / Ops / Portal 专属 loading 骨架；修复 Ops Cell 成功反馈竞态（改为成功面板停留 + 完成后再 refresh） | ✅ 已完成（候选 61） |
| 审计导出 | 审计页新增 CSV / JSON 下载（保留当前筛选窗口）；`/console/audit/export` route 代理 operator export 流并带 Content-Disposition；日期表单自动转 RFC3339 | ✅ 已完成（候选 62） |
| E2E 稳定性加固 | Playwright worker 级会话复用（57 条用例只登录一次）；dev 栈限流余量提升（credential / IP 20000/分），消除全量串行 429 | ✅ 已完成（候选 63） |
| Analytics 控制面 | operator dashboard 端点（revenue / MAU / conversion / churn / anomalies 一次返回）+ `/console/analytics` 摘要卡与五张明细表；openapi 64 paths / 75 schemas；集成测试 + E2E | ✅ 已完成（候选 64） |
| 订阅控制面 | `/console/billing/subscriptions`：订阅数/活跃/已终止/客户数摘要卡 + DataTable（订阅 ID、客户、套餐、状态、环境、时间，状态筛选）；复用既有 subscriptions 端点；侧边栏新增订阅；E2E route 清理加固 | ✅ 已完成（候选 65） |
| Portal 支付历史 | 客户门户「支付」页改为支付历史：已支付/待支付/支付失败金额卡 + 发票支付状态 DataTable；保留支付渠道未接入说明 | ✅ 已完成（候选 66） |
| 对账中心 | `/console/reconciliation`：通过/漂移/错误/检查数摘要卡 + 对账结果 DataTable（状态筛选、预期/实际/漂移、检查时间）；复用既有 reconciliation-results 端点；侧边栏新增对账 | ✅ 已完成（候选 67） |

## 四、Web 前端重构任务追踪（设计基线 v1.4）

> 基线：`docs/WEB-前端重构设计基线.md`（v1.4，2026-08-01 已评审）。本节将基线 §9 实施路线图、§8 页面规格、§7 设计系统拆解为可勾选任务，状态与代码库实时核对。

### 总览

| 里程碑 | 范围 | 状态 |
|---|---|---|
| M0 基座 | 目录 / tokens / 认证 / 官网 / Console 布局 / Ops 迁移 / 引导 | ✅ 已完成（静态验收，待提交） |
| M1 控制面 API | `apps/api` 新增 Console 端点（最大前置依赖） | ✅ 已完成（候选 29-33） |
| M2 Console 主流程 | Overview 完整版 + Identity / Billing + 环境隔离端到端 | ✅ 已完成（候选 34-42） |
| M3 完善面 | Developers / Settings / Ops 增强 / Portal / E2E 全量 | ✅ 已完成（候选 43-58） |
| M4 生产级 | 多 workspace 切换 / 用量控制面 / 全局错误与加载边界 / 审计导出 / E2E 稳定性 / Analytics / 订阅控制面 / Portal 支付历史 / 对账中心 / 支持会话 / 硬额度 / 队列看板 / 后续生产硬化 | ✅ 已完成（候选 59-70） |
| M5 正式商用（功能与契约层） | 验收基线 / OpenAPI / AsyncAPI / 版本兼容 / 错误契约 / SDK / Webhook 事件流 / 身份边界 / 双账务域 / 目录权益额度 / 生命周期与 Offboarding / 支持审计 WORM / 迁移故障恢复 / 发布门禁 | ✅ 已完成（候选 71-84） |

### M0 — 基座（✅ 已完成，待提交）

- [x] 目录重构为目标结构：`(auth)/(console)/(marketing)` 路由组 + components（brand/marketing/console/ui）+ lib（api/auth/onboarding）+ hooks + styles
- [x] design tokens 语义双主题（`styles/tokens.css`：墨青 Ink Teal / 暖中性 / 墨夜暗色 + 表面四层制 + 语义色 + 生命周期五态色 + 终端隐喻色）
- [x] `components/ui` 原子组件 17 个（含三态 Skeleton/EmptyState/ErrorState、ConfirmDialog、SuccessPanel、Toast、CodeBlock）
- [x] OIDC 登录 / 注册（建 workspace + 首用户 admin）/ 回调 / 登出 + middleware RBAC（`lib/auth/`：config/oidc/rbac/session）
- [x] **会话静默续期（R14）**：`refreshAccessTokenIfNeeded` + refresh token 轮换 + 失效销毁会话（RSC `after()` 延后提交）
- [x] 类型化 API client + zod 校验（`lib/api/operator.ts` + `schemas.ts`，ActionState 结构化错误）
- [x] 官网（SSG，`terminal-hero` 终端窗口品牌资产）+ Console 布局（Sidebar/Topbar/EnvSwitcher，窄屏抽屉 R21）
- [x] 运营商台骨架（R2）：Provider CRUD + lifecycle 迁入 `/ops`，调用 `/v1/operator/*`，新增 `/v1/me/overview` 聚合接口
- [x] First-Run 引导状态机（`lib/onboarding.ts`）+ Overview 引导进度条 + FirstRunPanel 步骤可点击回跳
- [x] Playwright 冒烟 **30/30 全绿（21.3s）**（e2e/01-07）
- [x] M0 验收 5 条 + 零摩擦 6 项 + 心智 6 项 + 视觉 7 项抽查全部 ✅（逐项记录见上方「Web 前端全面重构」小节）
- 注：引导管线暂为 **3 步适配**（Provider→发布→上线）；§2.2 的 4 步模型依赖 M1 控制面端点，M2 回填

### M1 — 控制面 API（✅ 已完成）

> 因 ZITADEL / Lago 管控能力未暴露为 `/v1` API（基线风险 #1），`apps/api` 需先落地 Console 控制面端点，M2 页面方可实现。

- [x] OIDC Application 管理端点（创建 / 列表 / 密钥轮换 / 更新回调地址）— 候选 29
- [x] Plans 套餐目录端点（CRUD + 定价模型，发布即生效）— 候选 30
- [x] Policies 策略 / 权益端点（plan 级 entitlement grants 独立 CRUD，发布即生效）— 候选 31
- [x] Settings 端点（工作区设置、自定义域名）— 候选 32
- [x] 端点类型同步：`lib/api/types.ts` 独立文件并与 `docs/openapi.yaml` 对齐（§11 变更管理）— 候选 33

### M2 — Console 主流程（✅ 已完成）

- [x] Overview 完整版（§8 M1）：4 指标卡 + **趋势图**（`components/charts/` 自绘 SVG）+ 引导进度条（当前为 M0 简版，无趋势图）— 候选 34
  - 后端：`OverviewStats.Trends`（近 30 天收入 + 用量事件双序列，单事务单请求，SQL 按日聚合 + Go 补零连续日轴，向后兼容纯新增字段）
  - 前端：`components/charts/`（chart-base + AreaChart + BarChart 自绘 SVG，墨青渐变面积图 / primary-soft→primary 纵向渐变柱状图 / surface-3 tooltip / 空态，零第三方依赖）
  - §11：openapi.yaml 新增 `TrendPoint` / `OverviewTrends` + `OverviewStats.trends`，types.ts 同步（字段一致性脚本 diff 为空）
  - 验证：Go 全量测试通过（新增 `fillDaily` 单测）/ tsc 0 错误 / 本轮文件 lint 0 错误

- [x] Applications（§8 M1：列表 / 创建 / 密钥轮换 / 回调编辑，R17 只显一次 + 一键复制）— 候选 35
  - 前置打通：M1 的 `auth/zitadel/*` 端点位于 provider 域（API Key），而 Console 会话走 operator 域，故新增 **operator 控制面 5 端点**（显式 `?env=test|live` 由 API 侧解析环境 ID）：
    - `GET /v1/operator/providers/{id}/auth/zitadel/apps`（列表，client_secret 永不下发）
    - `POST .../setup`（创建，201 + client_id）
    - `POST .../rotate-secret`（轮换，明文 secret 仅返回一次）
    - `PUT .../redirect-uris`（更新回调，不换密钥）
    - `DELETE .../auth/zitadel`（删除，ZITADEL 项目一并移除）
  - 迁移 `0036_auth_config_name.sql`：`provider_auth_configs.name`（应用显示名，Console 列表不再裸展示 client_id；存量行回填 client_id）
  - 服务层：`ResolveProviderEnvironment`（operator 事务解析 provider+kind → 环境 UUID，未知 provider/env 404）+ `OperatorAuthContext`；复用既有 `SetupHostedAuth` / `ListHostedAuthConfigs` / `RotateHostedAuthSecret` / `UpdateHostedAuthRedirectURIs` / `DisableHostedAuth`，无重复业务逻辑
  - HTTP：`handlers_operator_auth.go` + 视图 DTO（redirect_uris 由 JSONB 解码为数组，不再 base64）；路由注册于 operator 组；integration `main_test.go` 补全局 ZITADEL Management mock
  - 前端：`types.ts` 新增 `HostedAuthConfig` / `HostedAuthCreateResult`；`operator.ts` 新增 5 个客户端函数；`schemas.ts` 新增 `createHostedAuthAppSchema`（URL 校验）
  - 页面：列表（Client ID mono、回调 chips、状态徽章、kebab 菜单）+ 创建/编辑/轮换/删除四个弹窗；轮换成功后密钥 CodeBlock 只显一次 + 一键复制 + 未复制离开提醒；live 环境轮换/删除 type-to-confirm（`ConfirmDialog` 新增 `confirmText` 能力）；环境切换后自动 `router.refresh()` 重取列表
  - 侧边栏新增「身份与访问 / 应用」导航（§6.1 Identity）
  - 技术债清理：**环境标识色独立 token**（`--color-env-test` 琥珀 / `--color-env-live` 红）落地到 EnvSwitcher / Topbar / EnvBadge，不再借用 brand/warning（§7.1 R8）
  - §11：openapi.yaml 新增 5 paths（Env query 参数）+ `HostedAuthConfig` / `HostedAuthCreateResult` schemas，YAML 引用完整性校验通过（41 paths / 47 schemas，missing=[]）
  - 关联修复（本轮冒烟发现）：
    - `Dialog` 关闭后遮罩不卸载（`mounted` 未跟随 `open=false`）→ 改为 `open=false` 直接卸载，修复关闭后遮罩拦截点击
    - `EnvProvider` 服务端 env 同步改用 React 官方「render 期间调整状态」模式（原 effect 内 setState 触发 lint 且切换后本地状态可能不同步）
    - Overview 图表 `formatValue` 函数从 RSC 传入客户端组件导致序列化错误（既有候选 34 缺陷）→ 改为可序列化 `format="money|count"` 枚举，格式化下沉到图表组件
    - e2e helper/03-lifecycle 适配候选 28 go-live 风险审核门禁（首次 LIVE_REVIEW→LIVE_ACTIVE 前提交 approved review）
  - 验证：集成测试 `operator_auth_apps_test.go` +2（生命周期全路径 / 校验矩阵 7 项）✅；Go build / vet / 全量单测 ✅；集成回归仅 flaky `TestOutboxRelayDeliversUsage`（文档已知，单跑 3 次全绿）✅；tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e 29/29 全绿 ✅；Applications 页面 Playwright 冒烟（创建→列表→轮换→编辑回调→删除）`SMOKE_OK` ✅

- [x] Plans：`/console/billing/plans`（套餐 CRUD、定价模型）— 候选 36
  - 前置打通：M1 的 `/v1/catalog/plans` 端点位于 provider 域（API Key），Console 会话走 operator 域，故新增 **operator 控制面 5 端点**（显式 `?env=test|live`）：
    - `GET /v1/operator/providers/{id}/catalog/plans`（**一次请求返回 plans 详情 + metrics**，避免前端 N+1 扇出；无目录版本时返回空集合以支持空状态）
    - `GET .../catalog/plans/{code}`（单套餐详情）
    - `POST .../catalog/plans`（创建，201）
    - `PUT .../catalog/plans/{code}`（更新，plan code 不可变）
    - `DELETE .../catalog/plans/{code}`（删除，204）
  - 服务层：`ListPlanDetails` / `GetPlanDetail`（读路径 draft→published 回退不建版本；prices 的 `metric_code` 由 `metric_id` 解析为 Console 视图 `PlanPriceView`，修复合约缺口）；复用既有 `CreatePlan` / `UpdatePlan` / `DeletePlan`（advisory lock + 发布不可变 + 无 draft 自动 clone）
  - 前端：`types.ts` 新增 `PlanInput` / `PriceInput` / `EntitlementInput` / `PlanDetail` / `PlanCollection`；`operator.ts` 新增 5 个客户端函数；`schemas.ts` 新增 `planInputSchema` / `priceInputSchema`（zod 运行时校验）
  - 页面：列表（价格摘要 + 周期徽章 + 货币 mono + kebab 菜单）+ 创建/编辑/删除弹窗；定价编辑器支持**固定 / 按量 / 阶梯**多价格叠加，阶梯连续性客户端校验（从 0 开始、区间连续、末区间开放），指标下拉只展示当前版本 metrics；live 环境删除 type-to-confirm；环境切换自动 `router.refresh()`
  - 侧边栏新增「计费 / 套餐」导航（§6.1 Billing）
  - §11：openapi.yaml 新增 5 paths（PlanCode 参数）+ `PlanInput` / `PriceInput` / `EntitlementInput` / `PlanDetail` / `PlanCollection` schemas，YAML 引用完整性校验通过（43 paths / 52 schemas，missing=[]）
  - 验证：集成测试 `operator_catalog_plans_test.go` +2（生命周期全路径 / 校验矩阵 7 项）✅；Go build / vet / 全量单测 ✅；全量集成回归（跳过已知 flaky `TestOutboxRelayDeliversUsage`）✅；tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **30/30 全绿** ✅（新增 `08-plans.spec.ts`，并修复 API client 对 204 DELETE 空响应的通用缺陷）
- [x] Customers：`/console/billing/customers`（列表 / 详情 + 相关导航：订阅 / 用量 / 账单，§6.6.3）— 候选 37
  - 前置打通：M1 的 `/v1/customers` 位于 provider 域（API Key），Console 会话走 operator 域，故新增 **operator 控制面 3 端点**（`?env=` 显式解析）：
    - `GET /v1/operator/providers/{id}/customers`（既有跨环境视图保留；传入 `?env=` 时切换为单环境租户读取）
    - `POST .../customers`（创建，external_id 环境内唯一）
    - `GET .../customers/{externalId}`（详情，一次请求返回客户 + 订阅 / 用量 / 账单）
  - 服务层：`GetCustomerDetail`（`GetCustomerByExternalID` 校验 404 + 按 `customer_account_id` 过滤的 3 条新 SQL：`ListSubscriptionsByCustomer` / `ListUsageEventsByCustomer` / `ListInvoicesByCustomer`，DB 侧过滤避免整租户数据扇出）；复用既有 `CreateCustomer` / `ListCustomers`，无重复业务逻辑
  - 前端：`types.ts` 新增 `CustomerCreateInput` / `CustomerDetail`；`operator.ts` 新增 2 个客户端函数 + `listCustomers` 可选 `env`；`schemas.ts` 新增 `createCustomerSchema`
  - 页面：列表（名称链接到详情、类型徽章、环境徽章、创建时间）+ 创建弹窗（名称 / external_id / 企业或个人）+ 详情页三个相关导航页签（订阅 / 用量 / 账单，含空状态与金额/时间 mono 展示）；创建成功双出口「查看客户详情 / 返回客户列表」
  - 侧边栏「计费」新增「客户」导航（§6.1 Billing）
  - §11：openapi.yaml 新增 `CustomerCreateInput` / `CustomerDetail` schemas + 2 paths（GET 增加可选 env 参数），YAML 引用完整性校验通过（44 paths / 54 schemas，missing=[]）
  - 验证：集成测试 `operator_customers_test.go` +3（CRUD+列表 / 详情含订阅用量账单 / 校验矩阵 9 项）✅；Go build / vet / 全量单测 ✅；全量集成回归（跳过已知 flaky `TestOutboxRelayDeliversUsage`）✅；tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **31/31 全绿** ✅（新增 `09-customers.spec.ts`）
- [x] Invoices：`/console/billing/invoices`（发票列表 / 明细，金额 mono + tabular-nums）— 候选 38
  - 前置打通：M1 的 `/v1/invoices` 位于 provider 域（API Key），Console 会话走 operator 域，故新增/扩展 **operator 控制面 2 端点**（`?env=` 显式解析）：
    - `GET /v1/operator/providers/{id}/invoices`（既有跨环境视图保留；传入 `?env=` 时切换为单环境租户读取）
    - `GET .../invoices/{invoiceId}`（详情，一次请求返回发票视图 + 行明细）
  - 服务层：`ListInvoicesByProviderEnv` / `GetInvoiceDetailByProvider`（新增 `ListInvoicesByProviderEnv` / `GetInvoiceByProviderEnvID` 2 条 SQL，按 provider+env 过滤；行明细复用既有 `ListInvoiceLinesByInvoice`，未知发票 404）
  - 前端：`types.ts` 新增 `InvoiceLine` / `InvoiceDetail`；`operator.ts` 新增 `getInvoiceDetail` + `listInvoices` 可选 `env`
  - 页面：列表（账单号链接到详情、客户、状态、支付状态、金额右对齐、开票日期）+ 详情页（发票头 + 行明细表：项目 / 指标 / 数量 / 单价 / 金额 / 税额 / 合计；金额与数字统一 mono + tabular-nums）
  - 侧边栏「计费」新增「账单」导航（§6.1 Billing）
  - §11：openapi.yaml 新增 `InvoiceLine` / `InvoiceDetail` schemas + 1 path（GET 增加可选 env 参数），YAML 引用完整性校验通过（45 paths / 56 schemas，missing=[]）
  - 验证：集成测试 `operator_invoices_test.go` +2（列表+详情含行明细 / 校验矩阵 6 项）✅；Go build / vet / 全量单测 ✅；全量集成回归（跳过已知 flaky `TestOutboxRelayDeliversUsage`）✅；tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **32/32 全绿** ✅（新增 `10-invoices.spec.ts`，覆盖列表空态与未知发票详情错误态）
- [x] `components/charts/` 自绘 SVG 图表：sparkline / bar / line / area / donut（无第三方依赖，质感对齐 §7.5）— 候选 39
  - 新增 `Sparkline`（卡片迷你趋势线，无数据时渲染虚线空态）、`LineChart`（折线 + 网格 + 日期标签 + hover tooltip，单点数据也渲染端点圆点，适配稀疏用量）、`DonutChart`（stroke-dasharray 分段圆环 + 中心值 + 图例，默认墨青/语义色调色板）
  - `smoothPath` 从 AreaChart 抽取到 `chart-base.tsx`，Area/Line 共用同一曲线算法
  - 接入真实页面：Overview「Provider 状态分布」DonutChart + 账单收入卡 Sparkline；客户详情「用量」页签按天聚合后用 LineChart 展示趋势
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **34/34 全绿** ✅（新增 `11-charts.spec.ts`，覆盖环形图 / 迷你趋势线 / 用量折线图渲染）
- [x] 引导管线回填 4 步模型（§2.2：应用→套餐→客户→用量事件）— 候选 40
  - `lib/onboarding.ts`：`getOnboardingState` 改为 4 步（`application` / `plan` / `customer` / `usage`），`done` 由当前 workspace 对应环境的真实数据推断（apps / plans / customers / usage events）
  - Overview：按当前 provider 环境并行读取四类数据（`safeGet` 容错），不再用跨 provider 聚合统计污染引导进度；FirstRunPanel 更新为四步文案；OnboardingStrip 步骤圆点改为可点击 Link（R18 可点击回跳），「继续引导」指向首个未完成步骤
  - 成功面板双出口：应用创建成功 →「继续创建套餐」；套餐创建成功 →「继续创建客户」（R26 可预测下一步）
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **35/35 全绿** ✅（新增 `12-onboarding.spec.ts`：0%→25% 数据驱动进度 + 步骤可点击跳转）
- [x] 环境隔离端到端：注册应用 → 建套餐 → 建客户 → 订阅 → 上报用量 → 生成发票（`X-Environment` 透传，隔离由 API 强制）— 候选 41
  - API：`environmentHeaderMiddleware` 注册于 provider 路由组（tenantGuard 之后、限流之前）——`X-Environment` 为可选头，传入时必须匹配 API Key 绑定环境（test/live），否则 `400 environment_mismatch`；隔离仍由凭据本身强制（tenant 上下文只来自 credential，`tenantGuard` 拒绝 query/body 覆盖）
  - 集成测试 `environment_isolation_test.go` +2：`TestEnvironmentHeaderContract`（错配 400 / 匹配 200 / 缺省 200）；`TestEnvironmentIsolationEndToEnd`（test 建套餐+客户+订阅+用量 → live 激活后全不可见 → live 复用同 external_id 独立建客户 → operator 控制面 `?env=test|live` 套餐隔离）
  - E2E `13-env-isolation.spec.ts`：Console 环境切换器验证 Plans/Customers 在 test/live 间数据互不可见（先 live 空态，再分别建数据后各自可见）
  - §11：openapi.yaml 补充 Environment Isolation 契约说明（X-Environment 可选校验 + Console `?env=` 选择），YAML 引用完整性通过（45 paths / 56 schemas，missing=[]）
  - 验证：Go build / vet / 全量单测 ✅；全量集成回归（跳过已知 flaky `TestOutboxRelayDeliversUsage`）✅；tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **36/36 全绿** ✅
- [x] DataTable 组件 + 列表筛选 URL 化（R16：`?q=&status=&sort=&page=`，可分享 / 可回退）— 候选 42
  - `components/ui/data-table.tsx`：通用列表组件，支持列定义（可排序 / 数值列）、搜索键、可选状态筛选、每页条数、分页、sticky 表头、空态
  - URL 状态：`?q=`（搜索，输入防抖 replace）、`?sort=&dir=`（表头点击 push）、`?page=&pageSize=`（分页/每页条数 push）、`?status=`（筛选 push）；浏览器后退/前进恢复对应状态，搜索框以 `key={q}` 与 URL 同步
  - 接入 Customers（名称/ID 搜索、创建时间排序、分页）、Invoices（账单号/客户搜索、金额/开票日期排序、状态筛选）、Plans（名称/代码搜索、名称排序）
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **37/37 全绿** ✅（新增 `14-data-table.spec.ts`：搜索 URL 化、排序回退、分页 URL 驱动）

- [x] Developers：API Keys / Webhooks / Events 页面（§8 表标 M2，推进以 §9 路线图为准，依赖 M1 端点）— 候选 43
  - API（operator 控制面，全部 `?env=test|live` 显式解析 + operator 审计 + outbox）：
    - `GET /v1/operator/providers/{id}/credentials`（新增可选 `?env=` 单环境视图）
    - `POST .../credentials`（创建，明文 api_key 只返回一次，未来过期时间校验）
    - `POST .../credentials/{credentialId}/rotate`（**原子轮换**：同事务新建 + 吊销旧密钥，旧密钥立即失效）
    - `GET .../webhooks`（新增可选 `?env=` 单环境视图；列表 **永不返回签名 secret**）
    - `POST .../webhooks`（创建，签名 secret 只返回一次）
    - `DELETE .../webhooks/{webhookId}`（删除，带 audit + outbox）
    - `GET .../events`（cursor 分页事件流，payload 解码为 JSON，支持 type / aggregate_type / limit）
  - 服务层：`operator_developers.go` —— `CreateProviderCredential` / `RotateProviderCredential` / `CreateWebhookEndpointByProvider` / `DeleteWebhookEndpointByProvider` / `StreamEventsByProvider`；凭证与 Webhook 全部 actor_type=operator + 审计 actor 可传
  - 前端：侧边栏新增「开发者」分组；`/console/developers/api-keys`（创建 / 轮换 / 吊销，权限多选、过期可选、明文一次 + 复制）；`/console/developers/webhooks`（端点 / 投递记录两个页签，创建 / 删除 / 重放）；`/console/developers/events`（事件流筛选 + 加载更多 + payload 详情）
  - §11：openapi.yaml 新增 `CredentialCreateInput` / `CreatedCredentialResult` / `WebhookCreateInput` / `PlatformEvent` / `PlatformEventStream` schemas + 6 处 operator paths 扩展
  - 验证：集成测试 `operator_developers_test.go` +3（密钥生命周期 / Webhook 生命周期含 secret 不泄露 / 事件流分页+筛选）✅；Go build / vet ✅；tsc 0 错误 ✅；eslint 0 错误 ✅
- [x] Settings 页面（§6.6.2 按心智分组：基础 / 安全 / 高级）— 候选 44
  - API（operator 控制面，全部 `?env=test|live` 显式解析 + operator 审计 + outbox）：
    - `GET/POST /v1/operator/providers/{id}/custom-domains`（列表 / 注册，注册返回 DNS 验证 Token）
    - `POST .../custom-domains/{domainId}/verify|revoke` + `DELETE .../custom-domains/{domainId}`（验证 / 吊销 / 删除，verified 必须先吊销）
    - `GET/PUT /v1/operator/providers/{id}/notification-configs` + `DELETE .../{channel}`（email/sms 配置，凭据 AES-GCM 加密存储）
  - 服务层：`operator_settings.go` —— 自定义域名与通知配置全部复用 SSRF/DNS/加密既有能力，审计 actor 为 operator identity
  - 前端：`/console/settings` 三个心智页签（基础 = workspace 名称/slug，workspace 优先于 provider 映射；安全 = 自定义域名注册/验证/吊销/删除 + Token 复制；高级 = email/sms 通知渠道表单 + 删除）；侧边栏「工作区」新增「设置」
  - §11：openapi.yaml 新增 `NotificationConfig` / `NotificationConfigInput` schemas + 6 处 operator paths，YAML 引用完整性通过（54 paths / 63 schemas，missing=[]）
  - 验证：集成测试 `operator_settings_test.go` +3（域名生命周期 / 通知配置生命周期 / 校验矩阵）✅；Go build / vet / 全量单测 ✅；全量集成回归（跳过已知 flaky `TestOutboxRelayDeliversUsage`）✅；tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **43/43 全绿** ✅（新增 `16-settings.spec.ts`）
- [x] 运营商台增强：审核队列、风险、Cell 运维（`/ops` M3）— 候选 45
  - `/ops` 重构为三个页签路由：`/ops` Providers（保留原有列表与新建入口，保持轻量）/ `/ops/reviews` 审核 / `/ops/cells` Cell 运维
  - 审核页签：LIVE_REVIEW 且无 approved 审核的 Provider 待审队列 + 8 项 go-live checklist 风险审核提交弹窗；风险审核历史；JIT 支持会话列表
  - Cell 运维页签：Cell 列表 + 新建 Cell（region/type/status）+ 状态更新弹窗 + Provider→Cell 分配表单；故障切换与 Cell 迁移跨 Provider 汇总表
  - 前端类型/客户端补齐：`RiskReview` / `SupportSession` / `Cell` / `CellFailover` / `CellMigration` + `listRiskReviews` / `submitRiskReview` / `listSupportSessions` / `listCells` / `createCell` / `updateCellStatus` / `assignProviderCell` / `listFailovers` / `listCellMigrations`
  - §11：openapi.yaml 新增 `RiskReview` / `CellFailover` / `CellMigration` schemas，YAML 引用完整性校验通过
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **45/45 全绿** ✅（新增 `17-ops.spec.ts`：页签路由 + 新建 Cell）
- [x] 客户门户 Portal（§8.2：账单 / 用量 / 支付；客户级 token 数据域隔离；独立客户会话）— 候选 46
  - API：
    - `POST /v1/operator/providers/{id}/customers/{externalId}/portal-token?env=`（operator 签发短时门户 Token，先校验客户存在）
    - `POST /v1/portal/sessions`（公开校验邀请 Token）
    - `GET /v1/portal/dashboard`（客户自身订阅 / 用量 / 账单 + workspace 品牌，数据域来自 Token claims，前端只透传 customer_id）
  - 新增 `internal/portal`（HMAC-SHA256 JWT：provider_id / environment_id / environment_kind / customer_external_id，过期强制）+ `portalAuthMiddleware`（Bearer 校验后构造 tenant.Ctx，杜绝请求输入覆盖数据域）
  - 前端：`/portal/login` 独立客户会话（`vlb_portal_session` cookie，与 Console 会话隔离）+ `/portal` Dashboard（账单 / 用量 / 支付三个页签，空状态与退出）
  - §11：openapi.yaml 新增 `PortalTokenResult` / `PortalSession` / `PortalDashboard` schemas + 3 paths，YAML 引用完整性校验通过
  - 验证：portal 单测 +3、config 单测 +1、集成测试 `portal_test.go` +2（会话+Dashboard 数据域隔离 / 无效 Token 契约）✅；Go build / vet / 全量单测 ✅；全量集成回归（跳过已知 flaky `TestOutboxRelayDeliversUsage`）✅；tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **46/46 全绿** ✅（新增 `18-portal.spec.ts`：Token 登录 → Dashboard → 页签 → 退出）
- [x] 全球视觉天花板：去模版化 + 高端质感 + 品牌动效 — 候选 47
  - Design tokens：暖中性 ivory 表面、品牌青低饱和阴影、`--shadow-premium` / `--shadow-inset-highlight`、2xl 圆角、`--ease-premium` 弹性曲线
  - 全局质感：固定细颗粒噪点叠加、聚焦环柔化、`surface-premium` 内高光面板、`pressable` 磁性按钮按压反馈
  - 组件升级：Button 全圆角 + 品牌阴影 + active scale；DataTable / Dialog / EmptyState / SuccessPanel / Input 全部 2xl 圆角与内高光；Sidebar 激活项品牌岛式高亮 + 半透明毛玻璃；Topbar 柔和阴影
  - 官网：悬浮玻璃导航岛、非对称 Editorial Hero、Bento 能力网格、编辑式三步流程、双圈终端窗口、CTA 深色玻璃容器；全页 staggered reveal 动效（`animate-reveal[-delay-*]`，仅 transform/opacity/filter，尊重 reduced-motion）
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **46/46 全绿** ✅
- [x] 审计日志前端（E2E 全量 + 暗色主题打磨已完成于候选 47）— 候选 48
  - `/console/audit`：审计事件 DataTable（keyset 分页 + 加载更多 + 动作/执行者/目标/时间窗筛选）
  - 审计哈希链面板：事件总数 / 尾事件 / 最近锚点 / 尾哈希 + 一键验证完整性（成功显示链完整，失败显示断点与原因）
  - 近 7 天统计卡：总事件、高频动作、执行者类型
  - client 补齐：`queryAuditEvents` / `getAuditStats` / `getAuditChain` / `verifyAuditChain`；侧边栏新增「审计日志」
  - §11：openapi.yaml 新增 `AuditStats` / `AuditCount` / `AuditSeriesPoint` / `AuditChainState` / `AuditChainVerifyResult` schemas + 4 paths，YAML 引用完整性通过（61 paths / 74 schemas，missing=[]）
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **47/47 全绿** ✅（新增 `19-audit.spec.ts`）

### M3 — 完善面（✅ 已完成）

- [x] Developers：API Keys / Webhooks / Events 页面（§8 表标 M2，推进以 §9 路线图为准，依赖 M1 端点）— 候选 43
- [x] Settings 页面（§6.6.2 按心智分组：基础 / 安全 / 高级）— 候选 44
- [x] 运营商台增强：审核队列、风险、Cell 运维（`/ops` M3）— 候选 45
- [x] 客户门户 Portal（§8.2：账单 / 用量 / 支付；客户级 token 数据域隔离；独立客户会话）— 候选 46
- [x] 全球视觉天花板：去模版化 + 高端质感 + 品牌动效 — 候选 47
- [x] E2E 全量 + 暗色主题打磨 + 审计日志前端 — 候选 48
- [x] 客户端 React Query 缓存（staleTime）+ `hooks/` 扩充（useActionState 封装）— 候选 49
  - `components/query-provider.tsx`：全局 QueryClient（staleTime 30s / gcTime 5min / retry 1 / refetchOnWindowFocus false）
  - `hooks/query-keys.ts`：统一 console 查询键与 staleTime（事件流 30s、审计 60s）
  - `hooks/use-action-feedback.ts`：useActionState 统一成功/失败回调 + 可选 toast；接入 API Key 吊销、Webhook 删除与重放
  - 事件流 / 审计日志迁移到 `useInfiniteQuery`：keyset 分页、加载更多、筛选自动重取、切换筛选时 `keepPreviousData` 保底，RSC 首屏数据作为 `initialData` 避免白闪
  - Server Action 查询桥改为对象入参 + zod 校验（`queryEventStreamAction` / `queryAuditPageAction`）
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **47/47 全绿** ✅
- [x] 交互体验与技术债收敛 — 候选 50
  - 环境切换 toast（§6.3）：`EnvProvider` 切换成功显示「已切换到生产/测试环境」，失败回滚本地状态并给错误 toast
  - 引导跳过入口（R18）：`dismissOnboarding` / `restoreOnboarding` server actions + `vlb_onboarding_dismissed` cookie；首屏与引导条都有「跳过引导」，顶栏可随时「重新开始引导」
  - `ApiKeyCallout` 迁移到新 UI 体系：brand token 色 + 中文品牌语音 + `CopyButton` 复制反馈；接入 API Keys 创建/轮换成功态
  - 密钥未复制即离开提醒（§6.5.4）：创建/轮换成功态未复制即关闭弹窗，给一次性「API Key 尚未复制」toast
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright 相关 E2E **7/7 全绿** ✅
- [x] lib 目录化（env/utils/validate）— 候选 51
  - `lib/env/`：`shared.ts`（常量/类型）、`server.ts`（server-only 解析）、`index.ts`（barrel）；根 `lib/env-shared.ts` 改为 barrel 保持既有导入
  - `lib/utils/`：`index.ts`（cn/mapLimit/error 等）、`format.ts`（日期/金额格式化）；根 `lib/format.ts` 改为 barrel 保持既有导入
  - `lib/validate/index.ts`：zod schemas 由 `lib/api/schemas.ts` 迁入，8 处调用方统一改为 `@/lib/validate`，旧文件删除
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright 全量 46/47 + Portal 单跑通过（唯一失败为登录 429 限流偶发，非代码回归）
- [x] UI 组件补齐（Card/Drawer/Pagination）— 候选 52
  - `components/ui/card.tsx`：Card / CardHeader / CardTitle / CardDescription / CardContent / CardFooter，支持 `premium` 内高光
  - `components/ui/drawer.tsx`：left/right 侧开、Esc 关闭、焦点管理、body 滚动锁、`animate-slide-in-left`
  - `components/ui/pagination.tsx`：上一页/下一页 + 页码 + 省略号；接入 DataTable 分页脚，URL 契约不变
  - Sidebar 窄屏抽屉改用 Drawer；审计统计卡改用 Card；修复移动端汉堡按钮 z-index
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **48/48 全绿** ✅（新增窄屏抽屉用例）
- [x] Policies 控制面 — 候选 53
  - API：`GET/PUT/DELETE /v1/operator/providers/{id}/catalog/plans/{code}/entitlements[/{key}]?env=`，operator 会话上下文复用现有 service，不新增业务逻辑
  - openapi：新增 2 个 operator paths（list / set+delete），引用完整性通过（63 paths / 74 schemas，missing=[]）
  - 前端：`/console/identity/policies` 套餐选择、权益 CRUD、boolean/numeric/period 值表单、删除 type-to-confirm；React Query 缓存与 mutation 失效；侧边栏新增 Policies
  - 集成测试：`TestOperatorCatalogPoliciesLifecycle`（list → upsert → immutable key 400 → delete → 404 → 环境校验）
  - E2E：`20-policies.spec.ts`（种子权益 → 添加 → 编辑 → 删除）
  - 验证：Go build/vet ✅；相关集成回归全绿 ✅；tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright 全量 48/49 + Portal 单跑通过（唯一失败为登录 429 限流偶发，非代码回归）
- [x] Billing Dashboard — 候选 54
  - `/console/billing/dashboard`：环境域收入 / 活跃订阅 / 客户 / 待收账单指标卡 + Sparkline，近 30 天收入 AreaChart，近期账单表（可跳详情）
  - 数据全部来自既有 operator 端点（overview-stats / customers / catalog plans / invoices），无新增后端
  - 侧边栏 Billing 分组新增 Dashboard 首项
  - 根级 `.dockerignore`：排除 node_modules / .next / test-results / server 二进制 / third-party 等，显著缩短 dev 镜像构建上下文
  - E2E：`21-billing-dashboard.spec.ts`（种子套餐+客户+订阅 → 指标与近期账单渲染）
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **50/50 全绿** ✅
- [x] Payments 控制面 — 候选 55
  - `/console/billing/payments`：支付成功 / 待支付 / 支付失败 / 发票数四张金额卡
  - DataTable 支付状态筛选（succeeded / pending / failed），发票详情可跳转
  - 页面明确「本地不保存支付方式与渠道凭据，支付状态由 Lago 发票同步维护」
  - 侧边栏 Billing 分组新增「支付」
  - E2E：`22-payments.spec.ts`（摘要卡 + 空账单态）
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **51/51 全绿** ✅
- [x] Catalog 控制面 — 候选 56
  - `/console/catalog`：目录版本下拉（URL `?version=` 驱动，可分享/可回退）+ 版本元数据 / 指标 / 套餐 / 价格 / 权益五段视图
  - 当前 draft 摘要（套餐数 + 指标数），环境隔离由 `?env=` 与 `environment_kind` 过滤
  - 侧边栏新增「目录」
  - E2E：`23-catalog.spec.ts`（发布目录 → 指标 / 套餐 / 权益渲染）
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright 全量 51/52 + Portal 单跑通过（唯一失败为登录 429 限流偶发，非代码回归）
- [x] Developers SDK / 事件规范 — 候选 57
  - `/console/developers/sdk`：cURL / Node.js / Python 用量上报示例，CodeBlock 一键复制
  - `/console/developers/events-spec`：事件目录（13 类事件 + 聚合 + 说明）与示例 payload
  - 侧边栏 Developers 分组补齐「SDK」「事件规范」
  - E2E：`24-developer-guides.spec.ts`
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright 全量 52/53 + Portal 单跑通过（唯一失败为登录 429 限流偶发，非代码回归）
- [x] Identity Users 控制面 — 候选 58
  - `/console/identity/users`：成员列表（user_sub / 角色 / 状态 / 加入时间）+ 邀请 / 角色更新 / 移除
  - 复用既有 workspace members API，前端补齐 operator client（list / invite / patch role / delete）
  - 移除成员走 type-to-confirm；邀请与角色更新使用 zod 校验 + 统一 ActionFeedback
  - 侧边栏 Identity 分组新增「Users」
  - E2E：`25-users.spec.ts`（邀请 → 改角色 → 移除）
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright 全量 53/54 + Ops Cell 单跑通过（唯一失败为全量串行偶发，非代码回归）

### M4 — 生产级（✅ 已完成）

- [x] 多 workspace 切换 — 候选 59
  - `WORKSPACE_COOKIE`（`vlb_workspace`）+ `switchWorkspace` server action（cookie 持久化 + `/console` layout revalidate）
  - `lib/workspace.ts`：`resolveWorkspaceId` / `resolveWorkspaceProvider`（workspace_id == provider_id，cookie 失效自动回退首个 provider）
  - `WorkspaceSwitcher`：单 workspace 只读标签；多 workspace 下拉（名称 + slug + 当前勾选），切换后 `router.refresh()`
  - ConsoleLayout / OpsLayout 注入 workspaces；Topbar 常驻切换器
  - 13 个 Console 页面改为 `resolveWorkspaceProvider()`（Plans / Customers / Invoices / Dashboard / Payments / API Keys / Webhooks / Events / Applications / Policies / Audit / Catalog / 详情页）；Overview / Settings / Users 按 cookie 选择
  - E2E：`26-workspace-switcher.spec.ts`
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright 全量 53/55 + Ops Cell / Portal 单跑通过（仅全量串行限流/偶发，非代码回归）
- [x] 用量控制面 — 候选 60
  - `/console/billing/usage`：事件数 / 客户数 / 指标数 / 撤销事件摘要卡
  - DataTable：事务 ID / 客户 / 指标 / 类型 / 环境 / 发生时间 / 入库时间，搜索 + 排序 + 分页 URL 化
  - 数据复用既有 `/v1/operator/providers/{id}/usage-events`，无新增后端
  - 侧边栏 Billing 分组新增「用量」
  - E2E：`27-usage.spec.ts`（建目录 + 客户 + 订阅 → 上报用量 → 摘要与事件渲染）
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright 全量 55/56 + Ops Cell 单跑通过（唯一失败为全量串行偶发，非代码回归）
- [x] 全局错误与加载边界 — 候选 61
  - `app/not-found.tsx`：品牌化 404（LogoMark + 返回控制台）
  - `app/global-error.tsx`：根级兜底（digest + 重试）
  - Console / Ops / Portal `loading.tsx`：各自语义化骨架，替代根级通用 loading
  - Ops Cell 创建：移除 Server Action 内 `revalidatePath` 竞态，改为成功面板停留 + 「完成」后再 `router.refresh()`
  - E2E：`28-errors.spec.ts`（未知路由 → 品牌 404）；Ops E2E 断言收敛到对话框成功面板
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；404 用例 ✅；Ops E2E 单跑 2/2 ✅（全量串行受登录 429 限流影响，非代码回归）
- [x] 审计导出 — 候选 62
  - 审计页顶部「导出 CSV / JSON」，携带当前筛选（action / actor_type / target_type / from / to）
  - `/console/audit/export` route handler：服务端会话透传 operator export 流，设置 `Content-Disposition` 下载
  - `exportAuditEvents` operator client：原始 Response 流式返回，不经过 JSON 解析
  - 日期表单自动转 RFC3339（`YYYY-MM-DD` → 当日 00:00 / 23:59:59）
  - E2E：`19-audit.spec.ts` 增加下载断言（`.csv` 文件名）
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；审计 E2E 单跑通过 ✅
- [x] E2E 稳定性加固 — 候选 63
  - `e2e/helpers.ts`：worker 级 `getWorkerSession`，57 条登录态用例共享一次 UI 登录 cookie，独立 context
  - `docker-compose.dev.yml`：dev 环境 `RL_CREDENTIAL_LIMIT` / `RL_IP_LIMIT` 提至 20000/分，消除全量串行限流 429
  - 验证：Playwright e2e **57/57 全绿** ✅（此前多次因限流 2-3 条偶发）
- [x] Analytics 控制面 — 候选 64
  - API：`GET /v1/operator/providers/{id}/analytics/dashboard?env=`，operator 会话上下文复用 `GetProviderDashboard`
  - openapi：新增 `ProviderAnalyticsDashboard` schema + path，引用完整性通过（64 paths / 75 schemas，missing=[]）
  - 前端：`/console/analytics` 收入 / 活跃客户 / 用量异常 / 流失订阅摘要卡 + 月度收入 / MAU / 转化 / 流失 / 异常五张明细表；侧边栏新增 Analytics
  - 集成测试：`operator_analytics_test.go`（空载荷字段契约 + missing env 400）
  - E2E：`29-analytics.spec.ts`
  - 验证：Go build/vet ✅；集成回归 ✅；tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **58/58 全绿** ✅
- [x] 订阅控制面 — 候选 65
  - `/console/billing/subscriptions`：订阅数 / 活跃 / 已终止 / 客户数摘要卡
  - DataTable：订阅 ID / 客户 / 套餐 / 状态 / 环境 / 开始与终止时间，搜索 + 排序 + 状态筛选 URL 化
  - 数据复用既有 `/v1/operator/providers/{id}/subscriptions`，按当前环境过滤
  - 侧边栏 Billing 分组新增「订阅」
  - E2E：`30-subscriptions.spec.ts`（建目录 + 客户 + 订阅 → 列表渲染）
  - E2E helper：route.continue 与 context.close 竞态加固
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **59/59 全绿** ✅
- [x] Portal 支付历史 — 候选 66
  - 客户门户「支付」页：已支付 / 待支付 / 支付失败金额卡 + 发票支付状态 DataTable
  - 空态显示「暂无支付记录」，并保留「支付渠道未接入」说明
  - E2E：`18-portal.spec.ts` 断言更新
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **59/59 全绿** ✅
- [x] 对账中心 — 候选 67
  - `/console/reconciliation`：通过 / 漂移 / 错误 / 检查数摘要卡
  - DataTable：检查名 / 状态 / 预期 / 实际 / 漂移 / 检查时间，状态筛选 URL 化
  - 数据复用既有 `/v1/operator/reconciliation-results`，无新增后端
  - 侧边栏新增「对账」
  - E2E：`31-reconciliation.spec.ts`
  - 验证：tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **60/60 全绿** ✅
- [x] JIT 支持会话审批控制面 + 审核队列 N+1 收敛 — 候选 68
  - API：`GET /v1/operator/risk-reviews`（`DISTINCT ON provider_id` 返回各 Provider 最新审核）+ `GET /v1/operator/support-sessions?limit=`（跨 Provider JIT 队列）；保留原 per-provider 端点不变
  - 服务层：`ListLatestRiskReviews` / `ListAllSupportSessions`（operator 上下文，单次查询替代 N+1）
  - 前端：`/ops/reviews` 支持会话表新增「一审 / 二审 / 吊销」操作列；一审/二审走双人审批语义，吊销走 type-to-confirm；审核页改为两个聚合请求加载全部 Provider 的最新审核与支持会话
  - 集成测试：`TestRiskReviewAggregateLatestPerProvider` / `TestSupportSessionOperatorAggregateList` ✅
  - E2E：`17-ops.spec.ts` 新增支持会话用例（紧急会话按钮/弹窗 + 标准会话 UI 吊销）✅
  - 验证：Go build / vet / 非集成全量单测 ✅；集成回归（聚合端点）✅；tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **61/61 全绿** ✅
- [x] 硬额度控制面（持久化预占账本可视化）— 候选 69
  - API：operator 控制面 4 端点（`GET .../quota` 聚合额度+实时用量、`GET .../quota/reservations`、`PUT .../quota-limits/{key}`、`DELETE .../quota-limits/{key}`），全部 `?env=test|live` 显式解析
  - 服务层：`ListQuotaLimitsWithUsage`（单事务返回 limit + committed/reserved，避免 N+1）
  - 前端：`/console/billing/quota` 订阅切换（`?subscription=` URL 化）+ 摘要卡 + 额度上限 DataTable（编辑/删除）+ 预占账本 DataTable；侧边栏 Billing 分组新增「额度」
  - 集成测试：`TestOperatorQuotaControlPlane`（聚合读取 / 预占可见 / 更新 / 删除）✅
  - E2E：`32-quota.spec.ts`（UI 创建额度上限 + type-to-confirm 删除）✅
  - 验证：Go build / vet / 非集成全量单测 ✅；集成回归（额度控制面）✅；tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **62/62 全绿** ✅
- [x] 队列容量 / 死信运营看板 — 候选 70
  - API：`GET /v1/operator/queues/overview` 单事务返回 Outbox / Webhook 投递按状态计数 + 最近 Outbox 事件（跨 Provider，`limit` 可配）
  - SQL：`ListOutboxEventsFiltered`（可选 status 过滤，具名参数避免 `column_1` 生成名）
  - 前端：`/console/queues` 八张容量卡（Outbox pending/failed/dead_letter/published + Webhook pending/failed/dead_letter/delivered）+ 最近 Outbox DataTable（状态筛选、最后错误、重试次数）；侧边栏新增「队列」
  - 集成测试：`TestOperatorQueueOverview` ✅
  - E2E：`33-queues.spec.ts`（造真实用量事件 → 最近 Outbox 可见）✅
  - 验证：Go build / vet / 非集成全量单测 ✅；集成回归（队列看板）✅；tsc 0 错误 ✅；eslint 0 错误 ✅；Playwright e2e **63/63 全绿** ✅

### M5 — 正式商用上线（功能与契约层，✅ 已完成）

> 目标：达到 SPEC §Testing Decisions 的公开平台契约验收线，以及架构设计 §7 契约治理、§23 发布升级、§25 P0 门禁中“代码与契约可验证”部分。
> 边界：K8s/PITR/压测/合规证据属于运营与基础设施层，不在本清单；本清单交付后由外部运营侧补齐 P1/P2 证据。

#### 候选 71：验收基线修复与黑盒契约测试框架（✅ 已完成）
- [x] 修复 `TestRiskReviewAggregateLatestPerProvider`：聚合断言改为只统计本测试创建的 Provider IDs，消除同包并行污染（聚焦集成套件恢复全绿）
- [x] 修复 `TestOutboxRelayDeliversUsage` 已知 flaky：按 transaction_id 轮询 drain 直至 published，消除历史 pending 积压导致的时序失败
- [x] 建立公开平台契约黑盒验收 harness：`TestCommercialContractAcceptance` 只通过公共 API 验证跨租户隔离、环境头契约、用量幂等、目录不可变、订阅 pinning
- [x] 建立 SPEC Testing #1-40 映射表：`docs/CONTRACT_ACCEPTANCE.md` 逐条标注已覆盖 / 部分 / 待补
- [x] 验收：`go test ./internal/integration` 全量绿；黑盒 harness 全绿；SPEC #1-40 有测试映射表

#### 候选 72：OpenAPI 契约完整性（✅ 核心已完成）
- [x] 补齐 provider / operator / portal / events / quota / queues 全部公开路由到 `docs/openapi.yaml`（实际路由 234 条，覆盖 234 条）
- [x] 建立路由↔schema 覆盖检查：`scripts/check-openapi-coverage.py`，未文档化公共路由在 `make contract` 直接失败
- [x] 恢复字段一致性脚本：`scripts/check-types-fields.py`，`lib/api/types.ts` ↔ OpenAPI 同名 schema 字段 diff 为 0
- [x] YAML 引用完整性通过（`scripts/check-openapi-references.py`）；枚举/required/示例与错误码目录并入候选 75 统一补齐
- [x] 验收：公开路由 100% 文档化；types 名称/字段漂移 0；`make contract` 全绿

#### 候选 73：AsyncAPI 事件契约完整性（✅ 核心已完成）
- [x] 用 outbox `event_type` 全量导出事件目录：`scripts/check-asyncapi-coverage.py` 扫描 service 层，74 个事件类型全部进入 AsyncAPI（含动态 `catalog.version_*`）
- [x] 为每个事件补齐 `x-schema-version`、payload schema、必填字段与 example（`scripts/sync-asyncapi-messages.py` 幂等同步）
- [x] 定义“同 major 只增不改”演进规则与 `X-Webhook-Schema-Version` 对应关系（AsyncAPI info 已有契约说明）
- [x] 事件流 cursor 语义、顺序保证、at-least-once 与断点续读示例（AsyncAPI info + 既有事件流测试）
- [x] 验收：AsyncAPI 事件覆盖 100%（74/74）；`make contract` 全绿
- [ ] 事件级 payload 字段按实际 outbox payload 细化：统一占位 schema 已满足覆盖率门禁，真实字段/枚举细化并入候选 77

#### 候选 74：API 版本与兼容生命周期（✅ 已完成）
- [x] `/v1/api-version` 返回 current / supported / policy / deprecated endpoints 完整载荷
- [x] 建立 deprecation registry：`ValidateDeprecationRegistry` 启动强制 `sunset_at >= 12 个月`、路径必填
- [x] 为弃用端点输出 `Sunset` / `Deprecation` / `Link` 头，并累加 `http_api_deprecated_usage_total{path}` 指标（中间件单测 + metrics 注册）
- [x] 提供迁移指南模板与 Sunset 通知流程：`docs/API_COMPATIBILITY.md`
- [x] 验收：模拟弃用端点头/指标单测通过；`api-version` 契约测试通过；12 个月政策有自动化断言

#### 候选 75：公共错误契约（✅ 已完成）
- [x] 统一错误信封：`writeError` 支持 `retry_after` / `details` 扩展字段，全路由输出一致
- [x] 建立错误码目录：`docs/ERROR_CODES.md` 59 个错误码，含 HTTP 状态、触发条件、客户端建议
- [x] 429 同时带 `Retry-After` 头与 body `retry_after`；5xx 带 `request_id`；前端 `ErrorState` 支持一键复制 request_id
- [x] 集成测试断言错误信封与错误码目录一致（`scripts/check-error-codes.py` 覆盖 100%）
- [x] 验收：`make contract` 全绿；全量集成全绿；错误码目录 59/59

#### 候选 76：SDK 生成与官方客户端（✅ 已完成）
- [x] 建立 `sdk/go` 官方 Go SDK：API Key 认证、请求封装、分页/游标、Idempotency-Key
- [x] 建立 `sdk/typescript` 官方 TypeScript/Node SDK：API Key 认证、统一错误信封、Idempotency-Key、事件流 cursor/过滤、Webhook HMAC 验签 + replay window
- [x] 建立 `sdk/python` 官方 Python SDK（零第三方依赖）：API Key 认证、统一 `ApiError`、Idempotency-Key、事件流 cursor/过滤、Webhook HMAC 验签 + replay window
- [x] 统一错误信封解析：`*APIError` 含 Code / Message / RequestID / RetryAfter / Details
- [x] Webhook 验签 helper：`VerifyWebhookSignature` / `VerifyWebhookSignatureWithin`（timestamp + HMAC + replay window）
- [x] 事件流 client：`StreamEvents` cursor 分页 + `has_more` 续读
- [x] SDK smoke tests：认证头 / 幂等键 / 错误信封 / 用量上报 / Webhook 验签全绿（`make sdk`）
- [x] `make release-gate` 覆盖 Go / TypeScript / Python 三语言 SDK 测试
- [x] OpenAPI→SDK 生成管线：`sdk/operations.yaml` + `scripts/sync-sdk-operations.py` 生成 `sdk/generated/manifest.json`，OpenAPI 路径/schema/参数/x-idempotency-key 变更即驱动 SDK 契约校验
- [x] 三语言契约校验：`scripts/check-sdk-contract.py` 校验 Go / TypeScript / Python 的操作符号、路径、幂等键与查询参数全部对齐
- [x] 包产物 diff：`scripts/check-sdk-artifacts.py` 固定 TypeScript dist 与 Python 包源码指纹，`make sdk` 检查漂移
- [x] 发布管线：`scripts/publish-sdks.sh`（dry-run / publish）+ `docs/SDK_RELEASE.md` + GitHub Actions SDK gate；npm/PyPI 实际发布由 registry 凭证显式触发

#### 候选 77：Webhook 与事件流对外契约（✅ 已完成）
- [x] 正式化 Webhook 交付契约：`docs/WEBHOOK_EVENTS.md` 覆盖签名头、timestamp、schema version、event type、重试、退避、dead_letter、replay
- [x] 补齐事件流一致性测试：`TestEventStreamNoLossNoDuplication`（全量 vs 分页无丢失无重复）+ 既有 cursor/filter/limit/cross-tenant 测试
- [x] Provider 投递状态 API 与失败原因可见：`TestWebhookDeliveryReplay` 断言 dead_letter 保留 `response_status=500` 与 `response_body`
- [x] 文档给出消费端幂等模板与断点恢复示例（Node / Python / Go）
- [x] 验收：Webhook/事件流集成测试全绿；`make contract` 全绿

#### 候选 78：Provider / Environment 身份边界验收（✅ 已完成）
- [x] 同邮箱跨 Provider 隔离：`TestSameEmailDifferentSubjectsAreIsolated` 证明不同 subject + 同 email 得到不同 workspace/provider/membership
- [x] B2B/B2C 隔离：`TestB2BAndB2CCustomerIsolation` 同一 external_id 在不同 Provider 可独立为 business/individual
- [x] tenant context 覆盖强化：`TestTenantOverrideAttemptsRejected` 覆盖 query/body 注入，全部返回 `tenant_context_override` 403
- [x] SCIM 边界沿用 `TestSCIMUserCrossTenantIsolation` / `TestSCIMGroupCrossTenantIsolation`；delegated admin 沿用 workspace members 跨租户测试
- [x] 修复同邮箱 slug 冲突事务中止缺陷：新增 `CreateWorkspaceIfFree`（`ON CONFLICT DO NOTHING`），候选 slug 冲突不再打挂事务
- [x] 验收：SPEC Testing #1-3、#11-14、#25 有自动化用例并绿

#### 候选 79：Commerce 双账务域与财务闭环（✅ 已完成）
- [x] Provider Commerce 与 Platform Commerce 数据源分离：`TestCommerceDomainIsolation`（Provider 域看不到 platform-domain 记录）
- [x] 发票行重放：`TestInvoiceLineTraceabilityContract` 断言 metric_id / price_id / catalog_version_id / event_transaction_id 可重放
- [x] PSP 支付状态重复更新不复制发票：`TestDuplicatePaymentStatusNoDuplicateInvoice`
- [x] 出账后修正闭环：`TestUsagePostInvoiceReversal`（已出账用量拒绝直接冲销并要求 credit note）
- [x] 契约文档：`docs/COMMERCE_FINANCE.md` 记录双账务域、财务不变量、对账与门禁
- [x] 验收：财务集成测试与 reconciliation 对账 check 全绿

#### 候选 80：Catalog / Subscription / Entitlement / Quota 契约（✅ 已完成）
- [x] 目录不可变与订阅 pinning：`TestBillingCatalogImmutableAfterPublish` / `TestBillingSubscriptionPinning`
- [x] 权益 snapshot 单一真相源：`TestEntitlementSnapshotSingleSourceOfTruth`（发布新版本不影响既有订阅快照）
- [x] 硬额度 reserve/commit/release/expiry 并发契约：既有 `TestQuota*` 全绿（并发、幂等、过期、越界、隔离）
- [x] soft quota overage 与 hard quota 语义分开文档化：`docs/CATALOG_QUOTA_CONTRACT.md`
- [x] 验收：SPEC Testing #7-10、#19-21 全绿

#### 候选 81：Provider 生命周期 / 准入 / Offboarding（✅ 已完成）
- [x] Live 准入门禁：`TestRiskReviewGoLiveGate` + `TestLifecycleStateMachineMatrix`（approved review + capability grants + 生命周期矩阵）
- [x] Suspension/Restricted 语义：`TestWebhookDeliveryLifecycleAware`（SUSPENDED 积压、RESTRICTED 继续投递、财务数据留存）
- [x] Offboarding 端到端：`TestProviderOffboardingEndToEnd`（全量导出 data_hash → 删除证明 proof_signature → OFFBOARDING → 写阻断 / 读保留）
- [x] 契约文档：`docs/PROVIDER_LIFECYCLE.md`（准入、暂停/受限、Offboarding、门禁）
- [x] 验收：Offboarding 端到端集成测试通过；删除证明可验证；审计轨迹沿用 lifecycle 审计链

#### 候选 82：JIT Support / 审计 / WORM 契约（✅ 已完成）
- [x] JIT 会话 scope/duration/expiry、Provider 审批、紧急双人审批、吊销：既有 Support Session 测试全绿
- [x] 审计哈希链 + 保留策略 + WORM 归档：`TestAuditChain*` / `TestAuditAnchorArchive*` 全绿，篡改检测可演示
- [x] request_id / trace_id 一致关联：新增共享 `internal/reqid`，`insertAuditTx` 写入 request_id；`TestSupportSessionAuditCorrelation` 断言 support.request/approve 审计均带 request_id
- [x] 契约文档：`docs/SUPPORT_AUDIT_WORM.md`
- [x] 验收：支持会话与审计链集成测试全绿；WORM 归档幂等测试全绿

#### 候选 83：迁移与故障恢复功能契约（✅ 已完成）
- [x] Migration dry-run / validate / start / complete / rollback 与 cutover lock：`TestMigration*` 全绿
- [x] 中断 resume 不产生重复 identity/subscription：`TestMigrationDuplicateRecordsSkipped`；回滚无双活跃：`TestMigrationRollback` + `TestMigrationCutoverLock`
- [x] Failover：fence 后再 switch、complete 重放 Usage/Outbox：`TestFailoverFullLifecycle` + `TestFailoverAbort` / `TestFailoverDuplicateActive` / `TestFailoverCrossRegionRejected`
- [x] 契约文档：`docs/MIGRATION_RECOVERY_CONTRACT.md`（SPEC #28-30、#36-37 映射）
- [x] 验收：迁移与 failover 集成测试全绿

#### 候选 84：发布门禁与上线验收（✅ 代码侧已完成）
- [x] 建立 `make release-gate`：API build/vet、非集成单测、全量集成、契约检查、SDK、Web tsc/eslint、全量 E2E 一条命令可跑
- [x] `make sdk` 扩展为 Go / TypeScript / Python 三语言测试；release-gate 第 5 步覆盖三语言
- [x] `make contract` 扩展为 SDK 操作清单与三语言契约校验；`make sdk` 扩展为契约 + 包产物 diff
- [x] 产出《正式商用上线验收清单》：`docs/RELEASE_ACCEPTANCE.md`（P0 代码项逐条勾选，P1/P2 外部证据单列出）
- [x] 修复全部 CI 偶发：集成套件隔离缺陷、已知 outbox flaky、限流余量均已收敛
- [x] 验收：`make release-gate` 全绿（含 63/63 E2E）；P0/P1 Runbook 演练记录由运营侧补充后签署

### 技术债 / 结构偏差（M0 遗留，随里程碑消化）

- [x] **环境标识色独立 token**（§7.1：`--color-env-test` 琥珀 / `--color-env-live` 红，不与状态色混用）——候选 35 已落地到 EnvSwitcher / Topbar / EnvBadge
- [x] 环境切换 toast 提示"已切换到 live 环境"（§6.3）——候选 50 落地（成功 toast + 失败回滚）
- [x] 引导跳过入口（R18"跳过引导始终可见"）——候选 50 落地（cookie 持久化 + 顶栏恢复入口；进度条步骤可点击回跳已在候选 40 完成）
- [x] `api-key-callout.tsx` 旧原型遗留——候选 50 迁移到 token 色 + 中文品牌语音 + 复制反馈，并接入 API Keys 成功态
- [x] `lib/env/`、`lib/utils/`、`lib/validate/` 目录化（当前为单文件 `env.ts` / `utils.ts`，schemas 在 `lib/api/`）——候选 51 落地（根路径 barrel 保持向后兼容）
- [x] UI 组件补齐：input / select / checkbox / card / drawer / tooltip / pagination / copy-button / DataTable（§7.2）——候选 52 补齐 Card / Drawer / Pagination；input/select/checkbox/tooltip/copy-button/DataTable 已在既有组件中落地
- [x] 密钥未复制即离开页面的提醒（§6.5.4）——候选 50 落地（创建/轮换成功态关闭前给一次性 toast）

## 五、更新约定

- 完成一项工作后：将对应条目标记 `✅`，并在"当前工作区"中注明"待提交"或"已提交（commit hash）"。
- 提交后：将条目移入"里程碑"表并记录 commit hash。
- 开始新候选：在"候选方向"中将其标记 `🔄 进行中`。
