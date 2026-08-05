/**
 * Platform API 实体类型（纯类型，零运行时依赖）。
 *
 * 与 docs/openapi.yaml 的 components.schemas 对齐（§11 变更管理）：
 * - 本文件是前端类型单一事实源：operator.ts（API 解析）与 schemas.ts（zod 校验）
 *   导出的类型均 re-export 自本文件，禁止各自定义同名类型。
 * - 漂移修复流程：发现字段与 openapi.yaml / 实际 API 响应不一致时，先修正
 *   docs/openapi.yaml，再同步本文件，最后跑 tsc --noEmit 验证。
 */

// ===== 枚举 / 联合字面量 =====

/** Provider 生命周期流转目标（operator 生命周期端点）。 */
export type LifecycleTarget =
  | "LIVE_REVIEW"
  | "LIVE_ACTIVE"
  | "RESTRICTED"
  | "SUSPENDED";

// ===== 核心实体 =====

/** Provider（平台控制面视角；lifecycle_state 见 Go 端 lifecycle.LifecycleState）。 */
export interface Provider {
  id: string;
  slug: string;
  name: string;
  lifecycle_state: string;
  sla_tier?: string;
  home_region_id?: string;
  /** 仅 schemas.ts providerSchema 校验结果携带（API 响应不含该字段时缺省）。 */
  home_region_code?: string;
  cell_id?: string;
  /** 仅 schemas.ts providerSchema 校验结果携带（API 响应不含该字段时缺省）。 */
  description?: string | null;
  created_at?: string;
  updated_at?: string;
}

/** Provider 风险审核记录（operator 内部评级）。 */
export interface RiskReview {
  id: string;
  provider_id: string;
  risk_score: number;
  checks: Record<string, boolean>;
  decision: "approved" | "rejected";
  reason?: string;
  reviewed_by: string;
  reviewed_at?: string;
  created_at?: string;
}

/** JIT 支持会话（operator 视图）。 */
export interface SupportSession {
  id: string;
  provider_id: string;
  environment_id: string;
  access_type: "standard" | "emergency";
  status: string;
  requested_by: string;
  reason: string;
  requested_scopes: string[];
  approved_by?: string;
  second_approver?: string;
  granted_at?: string;
  expires_at?: string;
  revoked_at?: string;
  revoked_by?: string;
  revoke_reason?: string;
  created_at?: string;
  updated_at?: string;
}

/** Cell 拓扑单元。 */
export interface Cell {
  id: string;
  region_id: string;
  code: string;
  cell_type: "shared" | "dedicated";
  status: "active" | "draining" | "inactive";
  capacity_limits?: unknown;
  created_at?: string;
}

/** Cell 故障切换（热备）。 */
export interface CellFailover {
  id: string;
  provider_id: string;
  from_cell_id: string;
  to_cell_id: string;
  status: string;
  reason?: string;
  initiated_by: string;
  fencing_token?: string;
  replayed_usage: number;
  replayed_outbox: number;
  started_at?: string;
  completed_at?: string;
}

/** Cell 迁移任务。 */
export interface CellMigration {
  id: string;
  provider_id: string;
  from_cell_id: string;
  to_cell_id: string;
  status: string;
  scheduled_at?: string;
  precheck_passed: boolean;
  data_integrity_hash?: string;
  record_count: number;
  reason?: string;
  initiated_by: string;
  started_at?: string;
  completed_at?: string;
  error_message?: string;
  created_at?: string;
  updated_at?: string;
}

/** 环境（test / live 双环境模型）。 */
export interface Environment {
  id: string;
  provider_id: string;
  kind: string; // "test" | "live"
  status: string;
  issuer?: string;
  created_at?: string;
}

/** 区域（region registry）。 */
export interface Region {
  id: string;
  code: string;
  jurisdiction: string;
}

/** Catalog 版本列表行（轻量视图）。 */
export interface CatalogVersion {
  id: string;
  version: number;
  state: string; // "draft" | "validated" | "published" | "retired"
  environment_id: string;
  environment_kind: string; // "test" | "live"
  metrics_count: number;
  plans_count: number;
  created_at?: string;
  published_at?: string;
}

/** 订阅（operator 视图；openapi 同名 Subscription，provider 域入参端点不复用）。 */
export interface Subscription {
  id: string;
  external_id: string;
  customer_external_id: string;
  plan_code: string;
  catalog_version_id: string;
  status: string; // "active" | "terminated"
  environment_kind: string; // "test" | "live"
  started_at?: string;
  terminated_at?: string;
}

/** 客户（operator 视图）。 */
export interface Customer {
  id: string;
  external_id: string;
  account_type: string; // "individual" | "business"
  display_name: string;
  environment_id: string;
  environment_kind: string; // "test" | "live"
  created_at?: string;
}

/** 客户创建入参（external_id 在 provider 环境内唯一）。 */
export interface CustomerCreateInput {
  external_id: string;
  account_type: "individual" | "business" | string;
  display_name: string;
}

/** 客户详情（Console 一次请求返回客户 + 订阅 / 用量 / 账单）。 */
export interface CustomerDetail {
  customer: Customer;
  subscriptions: Subscription[];
  usage_events: UsageEvent[];
  invoices: Invoice[];
}

/** 客户门户 Dashboard（仅当前客户自己的数据 + workspace 品牌）。 */
export interface PortalDashboard {
  provider_name: string;
  provider_slug: string;
  customer: Customer;
  subscriptions: Subscription[];
  usage_events: UsageEvent[];
  invoices: Invoice[];
}

export interface PortalSessionInfo {
  valid: boolean;
  provider_id: string;
  environment_id: string;
  environment_kind: string;
  customer_external_id: string;
  expires_at?: string;
}

/** 用量事件（operator 视图；openapi 中对应 UsageEventRecord，同名 UsageEvent 为 provider 域入参）。 */
export interface UsageEvent {
  id: string;
  transaction_id: string;
  kind: string; // "ingestion" | "reversal"
  metric_code: string;
  customer_external_id: string;
  environment_id: string;
  environment_kind: string;
  event_timestamp?: string;
  created_at?: string;
}

/** 审计事件。 */
export interface AuditEvent {
  id: number;
  provider_id?: string;
  environment_id?: string;
  actor_type: string; // "system" | "operator" | "provider" | ...
  actor_id: string;
  action: string;
  target_type?: string;
  target_id?: string;
  metadata?: unknown;
  request_id?: string;
  created_at?: string;
}

/** 审计统计聚合（operator 视图）。 */
export interface AuditStats {
  total: number;
  by_action: Array<{ key: string; count: number }>;
  by_actor_type: Array<{ key: string; count: number }>;
  series: Array<{ bucket: string; count: number }>;
}

/** 审计哈希链状态（tamper-evident）。 */
export interface AuditChainState {
  total_events: number;
  tail_hash?: string;
  tail_event_id?: number;
  last_anchor_id: number;
  last_anchor_event_id: number;
  last_anchor_hash: string;
  last_anchor_at?: string;
}

export interface AuditChainVerifyResult {
  ok: boolean;
  verified_from: number;
  verified_to: number;
  verified_count: number;
  broken_at?: number;
  reason?: string;
}

export interface AuditPageResult {
  events: AuditEvent[];
  next_cursor: number | null;
}

/** API 密钥（key_hash 永不下发，仅 key_prefix 标识）。 */
export interface Credential {
  id: string;
  name: string;
  key_prefix: string;
  scopes: string[];
  allowed_cidrs?: string[];
  environment_id: string;
  environment_kind: string; // "test" | "live"
  environment_issuer: string;
  expires_at?: string;
  revoked_at?: string;
  last_used_at?: string;
  created_at?: string;
}

/** 创建 / 轮换 API 密钥后的响应（明文 key 仅返回一次）。 */
export interface CreatedCredential {
  credential: Credential | null;
  api_key: string;
}

/** 发票（Lago 同步）。 */
export interface Invoice {
  id: string;
  number: string;
  lago_id: string;
  issuing_date: string; // ISO date YYYY-MM-DD
  invoice_type: string; // "subscription" | "add_on" | "credit" | "one_off" | "progressive_billing"
  status: string; // "draft" | "finalized" | "voided" | "pending" | "failed"
  payment_status: string; // "pending" | "succeeded" | "failed"
  currency: string; // 3-letter ISO code
  total_amount_cents: number;
  customer_external_id: string;
  customer_account_id?: string;
  subscription_id?: string;
  catalog_version_id?: string;
  environment_id: string;
  environment_kind: string; // "test" | "live"
}

/** 发票行明细（Console 发票详情视图）。 */
export interface InvoiceLine {
  id: string;
  lago_fee_id: string;
  metric_code?: string;
  item_type: string;
  item_name: string;
  units: string;
  precise_unit_amount: string;
  amount_cents: number;
  taxes_amount_cents: number;
  total_amount_cents: number;
  currency: string;
  event_transaction_id?: string;
  from_date?: string;
  to_date?: string;
  created_at?: string;
}

/** 发票详情（发票 + 行明细）。 */
export interface InvoiceDetail {
  invoice: Invoice;
  lines: InvoiceLine[];
}

/** Catalog 指标。 */
export interface Metric {
  id: string;
  code: string;
  name: string;
  aggregation_type: string;
  field_name?: string;
  billable: boolean;
}

/** Catalog 计划。 */
export interface Plan {
  id: string;
  code: string;
  name: string;
  interval: string;
  currency: string;
}

/** Catalog 价格。 */
export interface Price {
  id: string;
  charge_model: string; // "fixed" | "per_unit" | "tiered"
  metric_code?: string;
  properties?: unknown;
}

/** 权益授权（plan 级 entitlement grant）。 */
export interface EntitlementGrant {
  id: string;
  key: string;
  value_type: string;
  value?: unknown;
}

/** Catalog 版本全量元数据（比列表行 CatalogVersion 更丰富）。 */
export interface CatalogVersionMeta {
  id: string;
  provider_id: string;
  environment_id: string;
  version: number;
  state: string;
  created_at?: string;
  validated_at?: string;
  published_at?: string;
  retired_at?: string;
}

/** Catalog 版本详情（版本 + 全部组成元素）。 */
export interface CatalogVersionDetail {
  version: CatalogVersionMeta;
  metrics: Metric[];
  plans: Plan[];
  prices: Price[];
  entitlement_grants: EntitlementGrant[];
}

/** 价格入参（套餐创建/更新时提交；properties 结构随 charge_model 变化）。 */
export interface PriceInput {
  metric_code?: string;
  charge_model: "fixed" | "per_unit" | "tiered" | string;
  properties: Record<string, unknown>;
}

/** 权益入参（PlanInput.entitlements 可选；Console 本期由 Policies 页面管理）。 */
export interface EntitlementInput {
  key: string;
  value_type: "boolean" | "numeric" | "period" | string;
  value: unknown;
}

/** 套餐创建/更新入参（plan code 不可变；更新时 body code 必须与路径一致）。 */
export interface PlanInput {
  code: string;
  name: string;
  interval: "weekly" | "monthly" | "yearly" | string;
  currency: string;
  prices: PriceInput[];
  entitlements?: EntitlementInput[];
}

/** Console 控制面套餐详情（prices 的 metric_code 已由 API 解析）。 */
export interface PlanDetail {
  plan: Plan;
  prices: Price[];
  entitlement_grants: EntitlementGrant[];
}

/** Plans 页面一次请求的全量载荷（当前 draft 版本，无 draft 回退 published）。 */
export interface PlanCollection {
  plans: PlanDetail[];
  metrics: Metric[];
}

/** 工作区（平台控制面多租户单元）。 */
export interface Workspace {
  id: string;
  slug: string;
  name: string;
  created_by: string;
  created_at?: string;
  updated_at?: string;
}

/** 工作区成员。 */
export interface WorkspaceMembership {
  id: string;
  workspace_id: string;
  user_sub: string;
  role: string; // "provider_admin" | "provider_developer" | "provider_billing"
  status: string; // "active" | "suspended" | "removed"
  created_at?: string;
  updated_at?: string;
}

/** 自定义认证域名（Console Settings 安全分组）。 */
export interface CustomDomain {
  id: string;
  provider_id: string;
  environment_id: string;
  domain: string;
  verification_token: string;
  status: "pending" | "verified" | "revoked";
  verified_at?: string;
  revoked_at?: string;
  created_at?: string;
  updated_at?: string;
}

/** 通知配置（解密后的 Console 视图；config 含渠道凭据，仅本会话可见）。 */
export interface NotificationConfig {
  id: string;
  provider_id: string;
  environment_id: string;
  channel: "email" | "sms";
  provider_type: string;
  config: Record<string, unknown>;
  from_address: string;
  enabled: boolean;
  created_at?: string;
  updated_at?: string;
}

/** Provider 能力开关。 */
export interface Capability {
  id: string;
  provider_id: string;
  capability: string;
  status: string;
  granted_at?: string;
  revoked_at?: string;
  granted_by?: string;
  reason?: string;
}

/** Webhook 端点（operator 视图）。 */
export interface WebhookEndpoint {
  id: string;
  provider_id: string;
  environment_id: string;
  environment_kind?: string;
  environment_issuer?: string;
  url: string;
  /** 仅在创建响应中出现一次，列表永不返回签名密钥。 */
  secret?: string;
  enabled: boolean;
  events: string[];
  created_at?: string;
  updated_at?: string;
}

/** Webhook 投递记录。 */
export interface WebhookDelivery {
  id: string;
  endpoint_id: string;
  outbox_event_id: string;
  status: string;
  attempts: number;
  response_status?: number;
  response_body?: string;
  delivered_at?: string;
  created_at?: string;
}

/** Console 事件流视图（operator /v1/operator/providers/{id}/events）。 */
export interface PlatformEvent {
  id: string;
  provider_id: string;
  environment_id: string;
  environment_kind: string;
  aggregate_type: string;
  aggregate_id: string;
  event_type: string;
  payload: Record<string, unknown> | unknown[] | string | number | boolean | null;
  payload_hash: string;
  transaction_id: string;
  status: string;
  attempts: number;
  created_at?: string;
  published_at?: string;
  next_attempt_at?: string;
  last_error?: string;
}

export interface PlatformEventStream {
  events: PlatformEvent[];
  next_cursor: string | null;
  has_more: boolean;
}

/** OIDC 应用（Console 控制面视图；client_secret 永不随列表返回）。 */
export interface HostedAuthConfig {
  id: string;
  name: string;
  client_id: string;
  enabled: boolean;
  redirect_uris: string[];
  created_at?: string;
  updated_at?: string;
}

/** 创建 / 轮换密钥后的应用响应（明文 client_secret 仅返回一次，R17）。 */
export interface HostedAuthCreateResult extends HostedAuthConfig {
  client_secret?: string;
  issuer_url?: string;
}

/** 趋势序列单日数据点；date 为 ISO-8601 日期（YYYY-MM-DD，UTC）。 */
export interface TrendPoint {
  date: string;
  value: number;
}

/**
 * 概览趋势序列（M2）：收入 = 近 30 天 finalized 发票按出票日汇总（cents）；
 * 用量事件 = 近 30 天 ingestion 事件按入库日计数。两序列均为后端补零后的
 * 连续日轴，前端直接绘制，无需再分箱。
 */
export interface OverviewTrends {
  revenue: TrendPoint[];
  usage_events: TrendPoint[];
}

/** 概览聚合统计（R29，跨所有 provider 单请求聚合）。 */
export interface OverviewStats {
  published_versions: number;
  active_subscriptions: number;
  customers: number;
  revenue_cents: number;
  trends: OverviewTrends;
}

// ===== 结果 / 输入类型 =====

/** 创建 / 激活 Provider 的结果。 */
export interface CreateProviderResult {
  provider: Provider | null;
  testEnvironment: Environment | null;
  apiKey: string | null;
}

/** 生命周期流转结果。 */
export interface LifecycleResult {
  provider: Provider | null;
  environment: Environment | null;
  apiKey: string | null;
}

/** 注册供给结果（R11）。 */
export interface SignupResult {
  workspace: Workspace | null;
  membership: WorkspaceMembership | null;
  /** 注册供给自动创建的 REGISTERED provider（未分配区域，需在 ops 激活）。 */
  provider: Provider | null;
}

/** 创建 Provider 入参（与 createProviderInputSchema 对齐）。 */
export interface CreateProviderInput {
  slug: string;
  name: string;
  home_region_code: string;
  description?: string;
}

/** 激活 Provider 入参（与 activateProviderInputSchema 对齐）。 */
export interface ActivateProviderInput {
  provider_id: string;
  home_region_code: string;
  reason?: string;
}

/** 注册供给入参（POST /v1/signup）。 */
export interface SignupInput {
  email?: string;
  name?: string;
}
