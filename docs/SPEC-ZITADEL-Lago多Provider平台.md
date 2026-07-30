---
title: 构建基于 ZITADEL + Lago 的多 Provider SaaS 身份与计费平台
labels:
  - ready-for-agent
status: ready
source: ZITADEL-Lago平台级生产架构设计方案 v2.0
---

## Problem Statement

当前架构能够支撑一个经营主体运营自己的 B2B SaaS，但无法作为面向市场开放的身份与计费基础设施服务多个彼此独立的 SaaS Provider。

现有方案缺少 Provider、Environment、Regional Cell、B2B/B2C Customer Account、双账务域和公开 API 契约。若直接上线，会产生以下问题：

- Provider 与其终端客户混用同一个 Tenant 概念，无法稳定实施授权、计费和数据隔离；
- Test 和 Live 数据可能混用，测试用量可能污染真实发票；
- ZITADEL 与 Lago 原生模型成为外部契约，底层升级会直接破坏客户集成；
- 平台向 Provider 收费与 Provider 向终端客户收费混在同一个账务域；
- 共享 Cell 缺乏数据库级隔离、资源公平调度和热租户迁移机制；
- B2C 用户被错误建模为 Organization，无法经济地支持大量个人客户；
- 缺少目录版本、用量冲销、出账后调整和账单可重放能力；
- 缺少实时硬额度，异步计量无法阻止并发超用；
- 缺少 Provider 准入、存量迁移、退出、数据导出和受控删除；
- 缺少公开 API、Webhook、SDK 和事件流的长期兼容政策；
- 单区域、单集群设计无法满足数据驻留、企业 SLA 和区域灾备要求。

平台需要一套以 Provider 和 Environment 为一级边界、以 Global Control Plane 和 Regional Cell 为运行拓扑、以平台稳定 API 隔离 ZITADEL 与 Lago 的生产架构。

## Solution

建设一个多 Provider SaaS 身份与计费平台：

- Global Control Plane 管理 Provider、Environment、Home Region、Cell、Domain、SLA Tier 和生命周期；
- Regional Cell 承载身份、客户、目录、订阅、计量、权益、额度、发票、支付连接和区域审计；
- 共享 Cell 服务标准 Provider，专属 Cell 服务企业、强监管或超大 Provider；
- Provider 映射为 ZITADEL Project，B2B Customer Account 映射为 Organization，B2C Account 映射为隔离 User；
- Provider Environment 映射到隔离的 Lago 执行空间，Provider 只能调用平台 Billing/Metering API；
- Test 与 Live 在凭证、身份、客户、目录、用量、支付和事件目的地上完全隔离；
- Platform Commerce 和 Provider Commerce 使用不同账户、目录、发票、支付连接和对账流程；
- Catalog 使用不可变发布版本，Subscription 固定绑定版本；
- Usage 使用固定 transaction ID、Transactional Outbox、不可变 Adjustment/Reversal 和账单可重放；
- Platform Entitlement Control Plane 是权益公开真相源，可选 Quota Service 提供实时预占额度；
- 对外提供版本化 Auth、Billing、Metering、Entitlement API、SDK、Webhook 和企业事件流；
- Provider 数据按 Home Region 驻留，区域故障使用同司法地域热备和单写切换；
- 所有共享数据使用 Provider/Environment 复合边界和 PostgreSQL RLS；
- Provider 可从 Sandbox 自助开始，Live 经过验证与风险审核；
- 平台提供标准迁入、完整导出、受控删除和删除证明；
- 目标架构一次设计，按 Phase 0 至 Phase 4 分阶段交付。

## User Stories

1. As a SaaS Provider developer, I want to create a Provider account, so that I can integrate identity and billing without operating those systems myself.
2. As a SaaS Provider developer, I want an immediately available Test environment, so that I can integrate without creating real customers or invoices.
3. As a SaaS Provider owner, I want to request Live activation, so that I can launch after completing platform verification.
4. As a platform risk operator, I want to approve Live capabilities separately, so that email, SMS, payments, domains, and throughput are not enabled as one unsafe global switch.
5. As a SaaS Provider administrator, I want separate Test and Live credentials, so that test traffic can never affect production data.
6. As a SaaS Provider administrator, I want to rotate credentials without downtime, so that compromised or expiring credentials can be replaced safely.
7. As a SaaS Provider developer, I want OAuth Client Credentials, so that production integrations use short-lived scoped tokens.
8. As a SaaS Provider developer, I want restricted API keys for initial integration, so that I can start quickly and migrate to OAuth later.
9. As a SaaS Provider security administrator, I want credential scopes and IP conditions, so that each integration has minimum required access.
10. As a SaaS Provider developer, I want a stable versioned Auth API, so that ZITADEL upgrades do not break my integration.
11. As a SaaS Provider developer, I want a stable versioned Billing API, so that Lago upgrades do not break my integration.
12. As a SaaS Provider developer, I want generated SDKs, so that I can integrate using supported language clients.
13. As a SaaS Provider developer, I want at least 12 months to migrate breaking API versions, so that production clients are not forced into emergency upgrades.
14. As a SaaS Provider operator, I want API usage and deprecation telemetry, so that I know which clients still use an old version.
15. As a SaaS Provider, I want a platform-provided authentication subdomain, so that I can launch without DNS work.
16. As an enterprise SaaS Provider, I want a verified custom authentication domain, so that my customers see my trusted brand.
17. As a SaaS Provider developer, I want a stable issuer when my Cell changes, so that deployed token validators continue to work.
18. As an end user, I want OIDC Authorization Code with PKCE, so that I can sign in securely from web and mobile applications.
19. As an end user, I want MFA and Passkeys, so that my account is protected against password theft.
20. As a machine integration, I want private key or client credential authentication, so that no human password is used for automation.
21. As a SaaS Provider, I want users isolated from every other Provider, so that matching email addresses never create cross-product identity linkage.
22. As an end user, I want a Provider-specific subject identifier, so that unrelated SaaS products cannot correlate my identity.
23. As a B2C SaaS Provider, I want individual Customer Accounts, so that I do not need an Organization for every consumer.
24. As a B2B SaaS Provider, I want business Customer Accounts, so that teams, memberships, roles, and enterprise SSO are first-class.
25. As a business administrator, I want to invite and remove members, so that I can manage my company's access.
26. As a business administrator, I want to configure SAML or OIDC federation, so that employees use the corporate identity provider.
27. As a business administrator, I want SCIM provisioning, so that employee lifecycle changes are automated.
28. As a SaaS Provider administrator, I want policies controlling delegated administration, so that customer administrators cannot exceed Provider rules.
29. As a SaaS Provider, I want to define billable metrics, so that product usage can be priced using my business model.
30. As a SaaS Provider, I want draft and validation states for catalog changes, so that incomplete pricing cannot become billable.
31. As a SaaS Provider, I want published catalog versions to be immutable, so that historical invoices remain reproducible.
32. As a SaaS Provider, I want future-dated price changes, so that customers receive predictable notice and migration.
33. As a SaaS Provider, I want subscriptions pinned to exact catalog versions, so that existing contracts do not change unexpectedly.
34. As a SaaS Provider, I want fixed, usage-based, tiered, prepaid, and hybrid pricing, so that the platform supports different SaaS business models.
35. As a SaaS Provider, I want customer-specific overrides with audit history, so that negotiated enterprise contracts can be represented safely.
36. As a SaaS Provider backend, I want to submit usage using a stable transaction ID, so that retries cannot charge twice.
37. As a SaaS Provider backend, I want accepted usage to be durable before downstream billing succeeds, so that a Lago outage does not lose revenue data.
38. As a SaaS Provider operator, I want duplicate payload conflicts reported, so that reused transaction IDs do not silently corrupt billing.
39. As a SaaS Provider, I want late usage accepted within a configured window, so that delayed systems can still produce correct bills.
40. As a SaaS Provider, I want incorrect usage corrected using adjustments, so that the original evidence remains auditable.
41. As a SaaS Provider, I want invoiced usage corrected with credit or supplemental invoices, so that finalized financial records are not rewritten.
42. As a finance operator, I want every invoice line linked to exact metric, price, tax, currency, and catalog versions, so that calculations can be reproduced.
43. As a SaaS Provider, I want plan entitlements returned through a stable platform API, so that my product is not coupled to Lago.
44. As a SaaS Provider, I want boolean, numeric, and period-based entitlements, so that plans can control features and limits.
45. As a SaaS Provider, I want temporary entitlement overrides, so that trials, support grants, and negotiated exceptions are possible.
46. As a SaaS Provider, I want eventual-consistency soft quotas for ordinary usage, so that low-value requests remain inexpensive.
47. As a SaaS Provider, I want optional reserve/commit/release hard quotas, so that scarce or expensive resources cannot be overspent concurrently.
48. As an end customer, I want quota failures to return stable domain error codes, so that I understand why an operation was denied.
49. As a SaaS Provider, I want to connect my own payment service account, so that I remain the Merchant of Record.
50. As a SaaS Provider, I want to connect my own tax and electronic invoicing systems, so that my legal entity controls compliance.
51. As a SaaS Provider, I want invoices, payments, failures, and dunning orchestrated by the platform, so that I do not rebuild billing workflows.
52. As a finance operator, I want PSP payment state reconciled with Lago invoices, so that payment failures and mismatches are visible.
53. As the platform business owner, I want to bill Providers for subscription tier, MAU, event volume, invoices, and dedicated capacity, so that platform revenue follows cost and value.
54. As a platform finance operator, I want Platform Commerce separated from Provider Commerce, so that platform revenue never mixes with Provider customer revenue.
55. As a SaaS Provider developer, I want signed Webhooks, so that common integrations can receive lifecycle events.
56. As an enterprise SaaS Provider, I want Kafka, SQS, or EventBridge delivery, so that high-volume events fit my infrastructure.
57. As a SaaS Provider operator, I want event replay by cursor and scope, so that destination outages can be recovered.
58. As a SaaS Provider security administrator, I want destination validation and SSRF protection, so that Webhooks cannot access platform private networks.
59. As a SaaS Provider operator, I want destination health and automatic suspension, so that a failing endpoint does not consume unlimited platform capacity.
60. As a SaaS Provider, I want my data stored in a selected Home Region, so that contractual residency requirements are met.
61. As an enterprise SaaS Provider, I want a dedicated Cell and database, so that I can obtain stronger isolation and reserved capacity.
62. As a standard SaaS Provider, I want shared infrastructure protected by fair scheduling, so that another Provider cannot exhaust common resources.
63. As a platform operator, I want hot Providers identified before Cell saturation, so that they can be throttled or migrated safely.
64. As a SaaS Provider, I want Cell migration without issuer or public API changes, so that infrastructure movement is invisible to my applications.
65. As a SaaS Provider, I want regional recovery within the same legal region, so that availability improves without violating residency.
66. As a platform recovery operator, I want write fencing before failover, so that identity and billing cannot diverge through dual writes.
67. As a SaaS Provider, I want platform-managed notification channels for initial launch, so that I can send login and billing messages quickly.
68. As a production SaaS Provider, I want to bring my own email and SMS providers, so that branding, cost, compliance, and reputation are isolated.
69. As a platform abuse operator, I want Provider-specific messaging quotas and reputation controls, so that one bad actor cannot block all senders.
70. As an existing SaaS Provider, I want dry-run import validation, so that migration problems are found before cutover.
71. As an existing SaaS Provider, I want JIT identity migration or temporary external IdP federation, so that users do not all reset passwords on one day.
72. As an existing SaaS Provider, I want resumable customer and subscription imports, so that large migrations survive transient failures.
73. As an existing SaaS Provider, I want a cutover lock and rollback plan, so that a failed migration does not leave two active billing systems.
74. As a departing SaaS Provider, I want a complete export of identity relationships, configuration, usage, invoices, and audit data, so that I can migrate away.
75. As a privacy administrator, I want controlled deletion with financial retention exceptions, so that privacy and statutory obligations are both met.
76. As a departing SaaS Provider, I want a deletion certificate, so that contract termination is independently auditable.
77. As a platform support engineer, I want time-limited approved support sessions, so that I can diagnose customer issues without permanent tenant access.
78. As a SaaS Provider administrator, I want visibility into support sessions, so that platform access to my environment is transparent.
79. As a platform security operator, I want emergency support access to require two-person approval, so that break-glass access cannot be abused.
80. As a SaaS Provider operator, I want near-real-time identity, usage, subscription, and revenue analytics, so that I can operate my SaaS business.
81. As a platform operator, I want analytics isolated from transactional databases, so that dashboards cannot slow login or billing.
82. As a finance operator, I want hourly incremental and daily full reconciliation, so that identity, subscription, usage, invoice, and payment drift is detected.
83. As an incident responder, I want every request and event correlated by Provider, Environment, Cell, actor, transaction, and trace, so that failures can be reconstructed.
84. As a platform operator, I want Cell-by-Cell canary deployment, so that a defective release cannot affect every Provider simultaneously.
85. As a compliance auditor, I want immutable administrative and financial audit records, so that security and billing changes are provable.
86. As a Provider procurement team, I want SOC 2, ISO 27001, GDPR, and PCI boundary evidence, so that I can approve the platform for production.
87. As a platform operator, I want backup restoration and regional failover rehearsed, so that recovery objectives are demonstrated rather than assumed.
88. As a platform product owner, I want shared, reserved-capacity, and dedicated tiers, so that pricing reflects isolation and SLA value.

## Implementation Decisions

1. Introduce Provider as the top-level commercial and security tenant.
2. Introduce Environment as a mandatory child of Provider, initially supporting test and live.
3. Scope all external identifiers, credentials, catalog resources, customer accounts, events, and idempotency keys by Provider and Environment.
4. Introduce Global Control Plane for Provider Registry, Environment Registry, Home Region, Cell Registry, Domain Registry, SLA tier, lifecycle, and routing.
5. Keep terminal identity, PII, usage, invoice, payment, and audit data inside the Provider Home Region.
6. Introduce Regional Cells as repeatable deployment units containing identity, platform services, Lago, storage, eventing, and observability.
7. Support shared Cells for standard tiers and dedicated Cells for enterprise or regulated tiers.
8. Model Provider as a ZITADEL Project in shared Cells.
9. Model B2B Customer Accounts as ZITADEL Organizations with memberships and Project Grants.
10. Model B2C Individual Accounts as Provider-isolated users rather than one Organization per consumer.
11. Use pairwise Provider/client subjects and prohibit cross-Provider identity linking based on email.
12. Support dedicated ZITADEL Instances when a Provider requires isolated issuer, custom compliance boundary, or dedicated Cell.
13. Expose Platform Auth API and standard OIDC endpoints; do not expose ZITADEL management APIs as the product contract.
14. Provide platform subdomains by default and verified custom authentication domains for eligible tiers.
15. Keep issuer stable across Cell migration.
16. Introduce Domain Control Plane for ownership validation, certificate issuance, collision prevention, revocation, and takeover protection.
17. Expose Platform Catalog, Billing, Metering, Entitlement, Quota, Migration, and Event APIs.
18. Treat Lago as an internal execution engine accessed only through Provider- and Environment-aware adapters.
19. Use separate Lago execution boundaries for each Provider Environment.
20. Implement catalog lifecycle as draft, validated, published, and retired.
21. Make published catalog, metric, and price versions immutable.
22. Pin every Subscription and Invoice Line to exact versioned inputs.
23. Separate Provider Commerce from Platform Commerce in accounts, catalogs, invoices, credentials, ledgers, payment connections, and reconciliation.
24. Keep Provider as Merchant of Record for Provider Commerce.
25. Treat Provider PSP as payment truth, tax connector as tax truth, and Provider accounting system as general-ledger truth.
26. Use Transactional Outbox for business facts that generate usage or external mutations.
27. Use stable transaction IDs and payload hashes for usage idempotency.
28. Keep usage events immutable and correct them with reversal or adjustment events.
29. Use credit or supplemental invoice flows after invoice finalization.
30. Make Platform Entitlement Control Plane the only public entitlement truth.
31. Derive entitlement snapshots from versioned catalog plus Lago subscription and billing status.
32. Provide optional persistent Quota Ledger with reserve, commit, release, and expiry recovery.
33. Treat Redis as cache only, never as the sole entitlement or quota truth.
34. Use OAuth Client Credentials or private_key_jwt as the preferred machine authentication method.
35. Support restricted, rotatable API keys as a compatibility and onboarding path.
36. Publish OpenAPI and AsyncAPI contracts and generate SDKs from platform contracts.
37. Support breaking public API versions in parallel for at least 12 months.
38. Support signed Webhook delivery for all eligible tiers and cloud event-stream destinations for enterprise tiers.
39. Add destination SSRF prevention, DNS rebinding checks, concurrency isolation, circuit breaking, replay, and health state.
40. Use PostgreSQL row-level security and composite Provider/Environment keys in shared platform databases.
41. Propagate tenant context through synchronous requests, workers, reconciliation, support sessions, cache keys, object paths, and event keys.
42. Use separate databases for dedicated Cells.
43. Use single-writer regional operation with same-jurisdiction warm standby and explicit write fencing.
44. Do not implement automatic cross-region active-active identity or billing writes.
45. Introduce Provider lifecycle states: registered, test active, live review, live active, restricted, suspended, and offboarding.
46. Grant Live capabilities independently for messaging, domains, payment connections, throughput, and event delivery.
47. Support platform-managed limited notification channels and Provider-owned production channels.
48. Introduce Migration Plane with dry-run, validation, resumability, JIT identity migration, dual-write observation, cutover lock, rollback, and audit.
49. Introduce Provider Offboarding with final billing, export, credential revocation, deletion, retention, backup expiry, and deletion certificate.
50. Introduce independent Analytics Plane using event projections; it must never decide authentication, hard quota, invoice amount, or payment state.
51. Introduce JIT Support Access Plane with Provider approval, scoped duration, full audit, and emergency two-person authorization.
52. Apply Provider-level fair scheduling, quotas, bulkheads, and circuit breakers in shared Cells.
53. Define Cell capacity limits and migration thresholds before production launch.
54. Design for 1,000 Providers, 10 million identities, and peak 5,000 usage events per second over 24 months.
55. Use Cell-by-Cell GitOps rollout and stop propagation after a failed canary.
56. Maintain SOC 2 Type II, ISO 27001, GDPR, and PCI SAQ A evidence from the first production release.
57. Complete legal review of ZITADEL, Lago, and other AGPL or third-party licensing before commercial launch.
58. Deliver the architecture in phases while preserving final Provider, Environment, Cell, API, and commerce boundaries from Phase 0.

## Testing Decisions

The primary testing seam is the public platform contract at the Provider/Environment boundary. Tests assert observable API responses, emitted events, persisted financial outcomes, isolation, and recovery. They must not assert internal ZITADEL or Lago implementation details.

The preferred number of top-level seams is one:

- A black-box platform acceptance harness drives Auth, Billing, Metering, Entitlement, Quota, Migration, and Event APIs using Test and Live Provider credentials.

Supporting seams are allowed only where external systems make the primary seam slow or nondeterministic:

- ZITADEL adapter contract tests;
- Lago adapter contract tests;
- PSP, tax, email, SMS, and event-destination connector contract tests;
- database RLS security tests;
- regional recovery and Cell migration system tests.

Required behavioral coverage:

1. Provider A can never read, mutate, infer, or correlate Provider B data.
2. Test credentials can never reach Live resources, invoices, events, payments, or domains.
3. Request bodies cannot override Provider or Environment derived from credentials.
4. Identical usage retries create one billable effect.
5. Identical transaction IDs with different payloads are rejected and audited.
6. Adjustment and reversal events produce reproducible pre-invoice and post-invoice outcomes.
7. Published catalog versions cannot be mutated.
8. Existing subscriptions remain pinned when a new catalog version is published.
9. Every invoice line can be replayed from immutable inputs.
10. Platform Commerce records never appear in Provider Commerce views or reconciliation.
11. B2C users are isolated without unnecessary Organizations.
12. B2B memberships, delegated administrators, enterprise SSO, and SCIM remain inside Provider boundaries.
13. Same-email users in different Providers receive unrelated subjects and sessions.
14. Issuer and public domain remain stable during Cell migration.
15. OAuth scopes, API-key restrictions, credential rotation, expiry, and revocation behave externally as documented.
16. Webhook signatures, timestamp windows, replay protection, retries, suspension, and replay operate correctly.
17. Webhook destinations cannot resolve or redirect to private platform networks.
18. Event-stream consumers can resume from cursors without missing or duplicating effects.
19. Soft quota allows documented bounded overage.
20. Hard quota prevents concurrent overspend using reserve/commit/release.
21. Redis loss does not lose authoritative entitlement or quota state.
22. Lago outage preserves accepted usage in Outbox and recovers without duplication.
23. Kafka outage causes bounded durable backlog rather than business data loss.
24. PSP duplicate or out-of-order events do not duplicate payment or invoice state.
25. RLS blocks direct and worker-originated cross-Provider access.
26. JIT support sessions expire, remain scoped, and appear in Provider audit.
27. Provider suspension blocks the intended capabilities without corrupting retained financial data.
28. Migration dry-run reports invalid identity, customer, subscription, and balance records.
29. Interrupted migrations resume without duplicate identities or subscriptions.
30. Failed cutovers can roll back without two active billing authorities.
31. Offboarding export is complete and checksum-verifiable.
32. Deletion retains only legally required financial records and produces evidence.
33. Analytics lag or outage does not affect identity, quota, billing, or payment decisions.
34. Shared-Cell load tests demonstrate Provider-level fairness under a noisy-neighbor event storm.
35. Cell capacity tests validate 2x normal peak and 3x billing-cycle peak.
36. Regional failover fences the former writer before accepting writes in standby.
37. Unconfirmed usage and Outbox records replay correctly after recovery.
38. Cell-by-Cell deployment stops after SLO regression.
39. Public API compatibility checks reject breaking changes inside an existing major version.
40. Deprecated API usage is observable and customers receive the documented migration window.

Existing prior art should be reused from ZITADEL's OIDC, token, organization, authorization, and integration tests, plus Lago's subscription lookup, event enrichment, cache, recurring metric, worker, and billing tests. These tests inform adapter expectations but do not replace platform-level acceptance coverage.

## Out of Scope

- Building a new identity engine to replace ZITADEL.
- Building a new billing calculation engine to replace Lago.
- Exposing raw ZITADEL or Lago administrative APIs as the platform product.
- Acting as Merchant of Record for Provider end customers.
- Implementing Provider accounting general ledger or statutory revenue recognition.
- Storing or processing raw payment card data.
- Supporting arbitrary password hash import without compatibility validation.
- Cross-region automatic active-active writes for identity, subscriptions, usage aggregation, or invoices.
- HIPAA, government classified workloads, and jurisdiction-specific financial licensing in the first commercial release.
- Guaranteed zero-downtime migration for every external identity, payment, tax, email, or event provider.
- Allowing direct browser submission of trusted billing usage.
- Treating analytics storage as a financial or authorization truth source.
- Shipping every enterprise feature in Phase 1; the boundaries are required immediately, while capabilities are enabled by phase.

## Further Notes

### Delivery order

Phase 0 establishes Provider, Environment, Region, Cell, tenant context, RLS, platform API contracts, dual commerce boundaries, Outbox/Inbox, audit, and secrets.

Phase 1 delivers a shared-Cell commercial loop: Hosted Auth, OIDC, B2B/B2C accounts, versioned catalog, subscriptions, usage, invoices, Provider PSP, Webhooks, entitlement snapshots, Sandbox, Live review, reconciliation, backup, and runbooks.

Phase 2 adds enterprise SSO, SCIM, delegated administration, custom domains, hard quota, Provider-owned messaging, event streams, reserved capacity, and standard migration.

Phase 3 adds multiple regions, same-region warm standby, dedicated Cells, Cell migration, residency, complete export, offboarding, and deletion evidence.

Phase 4 adds independent analytics, Platform Commerce metering, anomaly detection, capacity forecasting, and automated hot-Provider recommendations.

### Production gates

The first Live Provider must not be enabled until:

- Provider and Environment isolation is enforced in every synchronous and asynchronous path;
- Test-to-Live isolation tests pass;
- catalog and invoice replay are proven;
- usage idempotency and correction flows pass;
- Provider Commerce and Platform Commerce reconciliation is operational;
- RLS and cross-tenant penetration tests pass;
- Webhook SSRF and replay controls pass;
- backup restoration and regional recovery are rehearsed;
- JIT support access and immutable audit are operational;
- public API compatibility automation is active;
- license and compliance reviews are complete;
- P0/P1 on-call runbooks are exercised.

### Issue tracker status

This specification is ready for the ready-for-agent label. The current workspace root is not connected to a project issue tracker, and the only Git remotes are the upstream ZITADEL and Lago repositories. The specification must be published to the platform's own tracker rather than either upstream repository.
