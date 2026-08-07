# vLogBin 自建登录页 + ZITADEL Session API 技术方案（评审修订版）

> 状态：设计定稿基线 + 架构/产品评审修订，基于 `third-party/zitadel` 源码逐项核对。
> 评审修订：2026-08-06。评审裁决：有条件通过；补齐本节与 §2.1 的 P0 决策后进入 P0 实现。
> 实现状态：P0/P1/P2/P3/P4 主体完成；真实 ZITADEL v4.16.0 契约探针与 Playwright 自建登录 E2E 通过；用户/组织灰度白名单与结构化登录事件已实现；按 env 放量由部署环境变量控制。
> 已知边界：真实 OIDC callback 可换取 token 并完成 `/v1/signup`；`vlb_session` cookie 内嵌 access/refresh token 在 JWT access token 场景下超过 4KB，浏览器可能拒绝写入，进入控制台前需迁移为服务端会话存储。
> 已落地缓解：access/refresh token 拆到独立加密分片 cookie（`vlb_session_token_N`），主会话 cookie 仅保留身份；callback 使用 200 + meta refresh 先落 cookie 再进入控制台。
> 终态推进：平台 API 已新增 `auth_session_vault` 服务端 token vault（`POST/GET/DELETE /v1/auth/vault/{id}`，加密存储），web 会话下一步切换为“身份 cookie + vault token id”，彻底移除客户端 token。
> 终态已落地：web 会话 cookie 仅保留身份 JWT + `vid`，access/refresh token 全部通过平台 API token vault 按需读取；token 分片 cookie 已移除。
> 验证状态：真实 standalone 后端闭环已确认（vault 201 + signup 200）；浏览器 console 最终验收因 Playwright 中间跳转竞态待稳定后补测。
> 最终验证：standalone（`HOSTNAME=localhost`）下 Playwright 真实登录到达 `/console`，Console 成功加载 vault/workspaces/overview 数据。
> 生产安全基线：vault 使用独立 `AUTH_VAULT_MASTER_KEY` 加密并支持 previous-key 轮换；create/get/delete 均写入审计事件并上报 `auth_vault_operations_total` 指标；过期 vault 由 `auth-vault-sweeper` 定时清理。
> 工作负载身份：web 用 `AUTH_VAULT_SERVICE_PRIVATE_KEY` 签发 5 分钟 RS256 JWT（iss `vlogbin-web`、aud `vlogbin-auth-vault`），API 用 `AUTH_VAULT_PUBLIC_KEY` 验签；静态 token 仅作回退。
> CI：`.github/workflows/ci.yml` 运行 Go/Web 门禁，并在 `scripts/ci-zitadel-e2e.sh` 中启动真实 ZITADEL + 平台 API + standalone Web，执行浏览器 Console 闭环 E2E。
> 定位：vLogBin 提供与品牌一致的统一登录页，ZITADEL 继续作为独立身份引擎，vLogBin 只通过公开 Session API / OIDC API / User API 与其交互。
> 合规边界：不修改、不内嵌 ZITADEL AGPL 核心源码。`proto/` 与 `apps/docs/` 为 Apache-2.0，`apps/login/`、`packages/zitadel-client/`、`packages/zitadel-proto/` 为 MIT，可作为接口与实现参考；法律结论以专业顾问意见为准。

## 0. 核心目标与边界

### 0.1 核心目标

vLogBin 自建登录页的真实价值只有两个，且必须在安全合规前提下实现：

1. **品牌一致性**：登录、注册、MFA、错误与加载状态呈现 vLogBin 统一的品牌语言，不再跳转到 ZITADEL 托管登录页。
2. **登录体验控制**：vLogBin 自主设计登录流程、错误反馈、账号选择器与无障碍交互，不改变 ZITADEL 作为身份引擎的职责。

### 0.2 安全合规硬约束

- 密码校验、锁定、tarpit、MFA 因子校验继续由 ZITADEL 执行；vLogBin 不存储密码，不自行实现身份协议。
- 每次 `CreateCallback` 前，vLogBin 服务端必须通过 MFA 门槛；`CreateCallback` 本身不校验 MFA，vLogBin 是入口守卫。
- 最终身份令牌仍通过标准 OIDC `code → /oauth/v2/token` 获取；Session token 只用于登录流程状态，不进入浏览器可读范围。
- 生产基线锁定 ZITADEL v4.16.x（≥4.6.0），禁用 `EnableRelationalTables`，持续跟进安全公告。
- vLogBin 只依赖公开 API 与 MIT/Apache-2.0 参考实现，不修改、不内嵌 ZITADEL AGPL 核心源码。

### 0.3 非目标

- 不实现 OIDC/SAML/SCIM 协议本身。
- 不替换 ZITADEL 的用户、组织、策略、凭据管理模型。
- 不承诺在本方案内覆盖 ZITADEL 全量登录能力；未支持的能力（如 IDP 直连）必须显式回退托管登录页。

## 1. 代码核对结论

### 1.1 版本基线

仓库快照同时包含：

- `proto/zitadel/session/v2/*`：正式版 Session API，本方案唯一使用目标。
- `proto/zitadel/session/v2beta/*`：已在 proto 注释中标记 deprecated，将在下个 major 版本移除，禁止新代码依赖。
- `backend/v3/`：新关系型架构，Session API 的 `ListSessions` / `DeleteSession` 在该特性开启时走新实现；对外契约不变。

仓库没有可用的版本号常量，但 `apps/docs` 内含 v2.70.0 benchmark，且代码已包含 `backend/v3`。上线前必须对实际部署的 ZITADEL 版本执行契约探针测试，锁定当前版本行为。

**生产基线（评审锁定）**：

- 目标版本：ZITADEL v4.16.x；任何生产版本不得低于 4.6.0（覆盖 Session API 两个已公开 CVE 的修复线）。
- 默认路径：事件溯源实现。`EnableRelationalTables` 保持关闭；`backend/v3` 的 session 权限仍是占位，不可作为生产依赖。
- 当前 `third-party/zitadel` 是开发态快照，不能作为长期版本基线；实际部署必须锁定官方发布镜像/包版本。

### 1.2 Session API 契约表

源码依据：`proto/zitadel/session/v2/session_service.proto`、`internal/api/grpc/session/v2/server.go`、`internal/api/grpc/session/v2/session.go`、`internal/api/grpc/session/v2/query.go`。

| 操作 | REST 路径 | HTTP | 权限要求 | 关键说明 |
| --- | --- | --- | --- | --- |
| CreateSession | `/v2/sessions` | POST 201 | `session.write` | 返回 `sessionId` + 最新 `sessionToken` + `challenges` |
| SetSession | `/v2/sessions/{session_id}` | PATCH | `session.write` | `session_token` 字段已 deprecated 且被忽略；响应轮换 token |
| GetSession | `/v2/sessions/{session_id}` | GET | 见源码注释 | 可传 `sessionToken` 替代权限 |
| ListSessions | `/v2/sessions/search` | POST | `session.read` 或自有 session | 常用 `idsQuery` 实现账号选择器 |
| DeleteSession | `/v2/sessions/{session_id}` | DELETE | `session.delete` 或自有 session | 可传 `sessionToken` 替代权限 |
| GetAuthRequest | `/v2/oidc/auth_requests/{auth_request_id}` | GET | `authenticated` | OIDC 授权请求信息 |
| CreateCallback | `/v2/oidc/auth_requests/{auth_request_id}` | POST | `session.link` | 用 session 换回调 URL（含 code） |
| AddHumanUser | `/v2/users/human` | POST | `user.write` | deprecated；注册推荐改用 `POST /v2/users/new` |
| CreateUser | `/v2/users/new` | POST | `user.write` | 当前非 deprecated 的用户创建入口 |
| ListUsers | `/v2/users` | POST | `authenticated` | 登录名/邮箱/手机号搜索，返回 userId 后再建 session |

### 1.3 认证主体与权限

源码依据：`cmd/defaults.yaml`、`cmd/setup/03.go`、`internal/command/instance.go`、`apps/login/src/lib/service.ts`。

ZITADEL 官方 Login App 使用一个带 `IAM_LOGIN_CLIENT` 角色的机器用户：

- 首次初始化时创建机器用户并授予 `IAM_LOGIN_CLIENT`。
- `IAM_LOGIN_CLIENT` 权限含 `session.read`、`session.write`、`session.link`、`session.delete`、`user.read`、`user.write`、`user.credential.write`、`user.passkey.write` 等。
- 初始化时可输出机器用户 PAT（`ZITADEL_FIRSTINSTANCE_LOGINCLIENTPATPATH` / `ZITADEL_LOGINCLIENT_PAT`），仅用于本地/紧急回退。
- 官方实现优先使用系统用户 JWT、Login Client Key、`ZITADEL_SERVICE_USER_TOKEN` 三种短期/可轮换凭据；vLogBin 生产环境推荐 `ZITADEL_LOGINCLIENT_KEYFILE` 或系统用户 JWT，禁止长期 PAT 作为生产主凭据。

### 1.4 Session token 语义

源码依据：`internal/api/authz/session_token.go`、`internal/command/session.go`、`internal/authz/repository/eventsourcing/eventstore/token_verifier.go`。

- token 内部格式为 `sess_<sessionID>:<tokenID>`，用 ZITADEL OIDC 加密密钥加密后返回给调用方。
- 每次 `CreateSession` / `SetSession` 都会生成新 token，旧 token 立即失效。
- token 可作为 `Authorization: Bearer <token>` 调用 ZITADEL API，但只对“已完成主认证”的 session 有效；MFA 缺失会被拒绝。
- token 不能直接用于 OAuth2 Token Introspection；要拿标准 access/id token 必须完成 OIDC `CreateCallback` 后由客户端走 `/oauth/v2/token`。
- `GetSession` / `DeleteSession` 可用 body 中的 `sessionToken` 替代权限校验。

### 1.5 Checks、Challenges 与 Factors

源码依据：`proto/zitadel/session/v2/session.proto`、`proto/zitadel/session/v2/challenge.proto`、`proto/zitadel/session/v2/session_service.proto`、`internal/command/session.go`。

| 类型 | 说明 |
| --- | --- |
| `checks.user` | 仅支持 `user_id` 或 `login_name`；**不直接支持 email/phone**，必须先用 `ListUsers` 解析为 userId |
| `checks.password` | 依赖 user 已确认；失败返回 `COMMAND-3M0fs`，锁定返回 `COMMAND-JLK35` / `COMMAND-SFA3t` |
| `checks.totp` | 6 位 code，依赖 user |
| `checks.otp_sms` / `otp_email` | 一次性 code，依赖 user；需要先请求对应 challenge |
| `checks.web_auth_n` | 需要先前请求 WebAuthN challenge 并完成浏览器断言 |
| `checks.idp_intent` | 用 IDP intent id/token 完成外部身份确认 |
| `checks.recovery_code` | 验证后作废 |
| `challenges.web_auth_n` | 返回 `PublicKeyCredentialRequestOptions`，需传 `domain` |
| `challenges.otp_sms` | 可 `returnCode` 或由 ZITADEL 发送 |
| `challenges.otp_email` | 支持 `send_code.url_template` 或 `return_code` |
| `user_agent` | `fingerprintId`、`ip`、`description`、`header`，用于设备绑定与审计 |
| `lifetime` | 必须大于 0；未设置则不过期；每次 Set 会重算 `expirationDate` |

### 1.6 错误契约

源码依据：`internal/api/grpc/gerrors/zitadel_errors.go`、`proto/zitadel/message.proto`、`proto/zitadel/error/v2/error.proto`、`apps/login/src/lib/grpc/interceptors/error-classification.ts`。

推荐只依赖稳定 `id` / `slug`，不展示英文 message：

| 场景 | 稳定标识 | 语义 |
| --- | --- | --- |
| 密码错误 | `COMMAND-3M0fs` | `Errors.User.Password.Invalid` |
| 用户锁定 | `COMMAND-JLK35` / `COMMAND-SFA3t` | `Errors.User.Locked` |
| session 已过期 | `COMMAND-Hkl3d` | `Errors.Session.Expired` |
| session 已终止 | `COMMAND-Hewfq` | `Errors.Session.Terminated` |
| session 不存在 | `QUERY-SFeaa` / `COMMAND-Flk38` | `Errors.Session.NotExisting` |
| token 无效 | `COMMAND-sGr42` | `Errors.Session.Token.Invalid` |
| WebAuthN 无 challenge | `COMMAND-Ioqu5` | `Errors.Session.WebAuthN.NoChallenge` |
| 用户未激活 | `Errors.User.NotActive` | 用户状态检查 |

Connect/gRPC 错误详情可能携带 `CredentialsCheckError.failed_attempts`，REST JSON 中对应 `details[].failedAttempts`，用于登录页展示剩余次数提示。客户端统一按 gRPC code → HTTP 映射处理：400/401/403/404/409/429/5xx。

### 1.7 官方 Login App 架构（已核对）

源码依据：`apps/login/src/proxy.ts`、`apps/login/src/lib/server/cookie.ts`、`apps/login/src/lib/server/session.ts`、`apps/login/src/lib/service.ts`、`apps/docs/content/guides/integrate/login-ui/login-app.mdx`。

- Next.js 应用，服务端持有短期凭据（Login Client Key / 系统用户 JWT / Service Token）调用 ZITADEL Connect/Session API。
- 浏览器只持有 `sessions` httpOnly cookie，内容是 `[{id, token, loginName, organization, timestamps}]`。
- `/.well-known/*`、`/oauth/*`、`/oidc/*`、`/idps/callback/*`、`/saml/*` 由中间件重写到 ZITADEL。
- 登录页把 `authRequest` 或 `requestId` 一直带在 URL，最终通过 `OIDCService.CreateCallback` 换成回调 URL。
- 服务端为每个 step 调用 `createSession` / `setSession`，并把响应中的新 token 写回 cookie。
- 服务端在 `CreateCallback` 前执行 `isSessionValid`：区分 passwordless（WebAuthN `userVerified=true`）与 U2F 二因素（`userVerified=false`），禁止把 U2F 当作主认证因素。

### 1.8 官方 Login App / Go SDK / OIDC 库路线核对（2026-08-07）

官方资料核对结论：

- ZITADEL 文档明确推荐用 `Session v2 API` + `OIDC v2 API` + `@zitadel/proto` / `@zitadel/client` 构建自建登录 UI；`zitadel-go` 文档也把 `Session API` 列为后续扩展目标。
- `zitadel-go` v3 的高层 `pkg/authentication` 是 `zitadel/oidc` RP 的封装，适合“跳转托管登录页 + 标准 OIDC”，不包含自建登录 UI 的状态机。其 `pkg/client/session/v2` 与 `pkg/client/oidc/v2` 是生成型 gRPC 客户端，与 `@zitadel/client` 等价，但没有官方 Login App 级别的业务实现。
- `zitadel/oidc` v3 是经过 OpenID 认证的底层 RP/OP 库，负责协议与令牌语义，不承载 ZITADEL Session/User API 的登录流程。
- 官方 Login App（`zitadel/zitadel` 仓库 `apps/login`）是当前唯一被 ZITADEL Cloud 使用的自建登录参考实现，业务契约应以其源码为准。

IdP 自动建号组织解析（源码依据：`apps/login/src/lib/server/idp-intent.ts`）：

1. 优先使用授权请求上下文中的 `organization`（来自 `urn:zitadel:iam:org:id:{orgid}` 或 primary-domain scope 解析出的唯一组织）。
2. 其次使用 IdP 返回的 username 后缀做域名发现；仅当命中唯一组织且该组织 `allowDomainDiscovery=true` 时采用。
3. 最后回退到实例默认组织，官方通过 Organization v2 `ListOrganizations(defaultQuery)` 获取。

新 `user_action.create_user` 是官方推荐字段，替代 deprecated `add_human_user`；npm `@zitadel/proto` 1.3.1 尚未暴露该字段，unknown field 6 的 `data` 在 @bufbuild v2 中包含长度前缀，vLogBin 已实现确定性兼容解码并保留 typed field 优先路径，SDK 升级后无需改业务代码。

## 2. 推荐架构

```mermaid
flowchart LR
    U[浏览器] -->|自有 UI + httpOnly cookie| W[vLogBin Next.js]
    W -->|OIDC 代理 middleware| Z[ZITADEL]
    W -->|Session v2 Connect/gRPC| Z
    W -->|User v2 Connect/gRPC| Z
    W -->|现有 platform API| A[vLogBin API]
    W -->|exchangeCode + verifyIdToken| Z
```

关键决策：

1. 新增 `lib/auth/zitadel-session.ts` 作为唯一 Session API 适配层，基于 `@zitadel/client` 生成的 ConnectRPC 客户端（MIT），禁止页面直接拼请求。
2. REST JSON 仅保留给契约探针与排障；主链路不再手写 REST 字段映射，避免 grpc-gateway 与 Connect 的契约漂移。
3. 生产凭据使用 Login Client Key / 系统用户 JWT / Service Token；PAT 仅本地与紧急回退，密钥不进前端。
4. 浏览器不接触 ZITADEL session token。登录流程中的 `sessionId + sessionToken` 存入独立 httpOnly `login_flow` cookie，并使用现有 `SESSION_SECRET` 派生密钥做 AES-256-GCM 加密。
5. 保持 `AUTH_MODE=oidc` 的现有授权码链路作为兜底，用功能开关渐进切换，不一次性删除。
6. 最终 token 仍通过标准 OIDC `code → /oauth/v2/token` 获取，Session token 只用于“当前登录流程状态”和 `CreateCallback`。
7. 新增 OIDC 代理 middleware，覆盖 `/.well-known`、`/oauth`、`/oidc`、`/idps/callback`、`/saml`、`/assets`；ZITADEL 后端地址必须来自受信配置，不能由客户端 Host 任意推导。

### 2.1 评审决策记录

| 决策 | 结论 | 理由 |
| --- | --- | --- |
| D1 传输契约 | 主链路用 `@zitadel/client` 生成客户端，REST 仅探针 | ZITADEL 新服务只提供 ConnectRPC，REST gateway 仅保留给既有服务 |
| D2 生产凭据 | Login Client Key / 系统用户 JWT 优先，禁用长期 PAT | PAT 是静态长期秘密，泄露等于完整会话管理权限 |
| D3 MFA 门槛 | 服务端 `isSessionValid` 独立模块，CreateCallback 前强制 | `CreateCallback` 不校验 MFA，官方 Login App 同样自行实现门槛 |
| D4 OIDC 代理 | middleware 重写协议路径，后端地址受信白名单 | 登录可用性依赖 vLogBin 代理层，必须可观测、可回退 |
| D5 能力回退 | 未覆盖的 IDP/SAML 流程显式回退托管登录页 | 自建登录页不替代全部 ZITADEL 登录能力 |
| D6 可观测性 | 登录成功/失败、MFA 拒绝、fallback 率、ZITADEL 延迟进指标 | 登录是安全核心路径，不能静默失败 |
| D7 版本治理 | 锁 v4.16.x（≥4.6.0），禁用 `EnableRelationalTables` | Session API 历史 CVE 与 backend/v3 权限 TODO 决定生产边界 |

## 3. 登录流程

```mermaid
sequenceDiagram
    participant B as 浏览器
    participant W as vLogBin Web
    participant Z as ZITADEL
    B->>W: /oauth/v2/authorize?client_id=...
    W->>Z: 代理 authorize
    Z-->>W: redirect /login?authRequest=V2_xxx
    W-->>B: 渲染登录名页
    B->>W: 提交 loginName/email/phone
    W->>Z: POST /v2/users（登录名/邮箱/手机号搜索）
    W->>Z: POST /v2/sessions { checks.user }
    W-->>B: 密码页
    B->>W: 提交密码
    W->>Z: PATCH /v2/sessions/{id} { checks.password }
    W->>Z: GET /v2/users/{id}/authentication_methods
    alt 需要 MFA
        W-->>B: MFA/OTP/Passkey 页
        B->>W: 提交因子
        W->>Z: PATCH /v2/sessions/{id} { checks.totp/otp/web_auth_n }
    end
    W->>W: isSessionValid（主因素 + MFA 门槛）
    W->>Z: POST /v2/oidc/auth_requests/{auth_request_id} { session }
    Z-->>W: callbackUrl
    W-->>B: 302 callbackUrl
    B->>W: /callback?code=...
    W->>Z: POST /oauth/v2/token（exchangeCode）
    W->>A: provisionWorkspace
    W-->>B: 写入 vLogBin session cookie
```

各步骤源码对照：

| 步骤 | vLogBin 新实现 | ZITADEL 官方参考 |
| --- | --- | --- |
| 代理 authorize | `lib/auth/oidc.ts` 保留 | `apps/login/src/proxy.ts` |
| 登录名搜索 | `zitadel-session.searchUsers` | `apps/login/src/lib/zitadel.ts` 的 `searchUsers` |
| 创建 session | `zitadel-session.createSession` | `apps/login/src/lib/server/session.ts` |
| 密码检查 | `zitadel-session.setSession` | `apps/login/src/lib/server/password.ts` |
| OTP challenge/check | `zitadel-session.setSession` | `apps/login/src/components/login-otp.tsx` |
| Passkey challenge/check | `zitadel-session.setSession` | `apps/login/src/lib/server/passkeys.ts` |
| 完成 OIDC | `zitadel-session.createCallback` | `apps/login/src/lib/oidc.ts` |

## 4. 注册流程

1. `/signup` 改为 vLogBin 自有页面，收集邮箱、姓名、可选密码。
2. 服务端先调 `GET /v2/settings/login` 确认 `allowRegister` / `allowLocalAuthentication`。
3. 调 `POST /v2/users/new`（`CreateUser`）创建 human；官方 Login App 当前仍使用 deprecated `POST /v2/users/human`，vLogBin 应直接使用非 deprecated 入口。
4. 创建后因投影尚未就绪，官方实现有“NotFound 重试”模式；vLogBin 同样实现指数退避重试建 session。
5. 按用户配置创建 session：有密码则一次性 `user + password` check，无密码则先 user check 后进入验证码/Passkey 初始化页。
6. 注册结果需纳入邮箱验证、`humanMFAInitSkipped` 与 MFA 初始化跳过窗口，与官方 Login App 的 enrollment guard 对齐。
7. 完成 OIDC `CreateCallback` 前必须通过 `isSessionValid`；通过后沿用现有 `provisionWorkspace` 事务初始化 workspace。

## 5. 契约层

### 5.1 环境变量

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `AUTH_MODE` | 是 | `oidc`（默认）/ `operator-token` / `oidc-custom-login` |
| `ZITADEL_URL` | 是 | ZITADEL issuer/API 根地址 |
| `ZITADEL_API_URL` | 自建登录模式必填 | Session API 基址，默认同 `ZITADEL_URL` |
| `ZITADEL_CLIENT_ID` | 是 | OIDC 应用 client id |
| `ZITADEL_CLIENT_SECRET` | 按应用 | confidential client 必填 |
| `ZITADEL_LOGINCLIENT_KEYFILE` | 生产推荐 | 机器用户 key file，服务端签发短时 JWT |
| `AUDIENCE` + `SYSTEM_USER_ID` + `SYSTEM_USER_PRIVATE_KEY[|_FILE]` | 生产备选 | 系统用户 JWT，官方 Login App 同款模式 |
| `ZITADEL_SERVICE_USER_TOKEN` | 备选 | 服务用户 token，需纳入轮换审计 |
| `ZITADEL_LOGIN_CLIENT_PAT` | 本地/紧急回退 | 开发用 PAT；生产禁止作为长期主凭据 |
| `ZITADEL_LOGIN_UI_MODE` | 否 | `redirect`（当前行为）/ `session-api`（渐进切换） |
| `ZITADEL_TRUSTED_DOMAIN` | 是 | 与 `APP_BASE_URL` 同源，注册进 Trusted Domains |
| `SESSION_SECRET` | 是 | 加密 login_flow cookie 与现有 session |

### 5.2 适配层 API

```ts
export interface ZitadelSessionClient {
  searchUsers(input: SearchUsersInput): Promise<UserHit[]>;
  createSession(input: CreateSessionInput): Promise<CreateSessionResult>;
  setSession(input: SetSessionInput): Promise<SetSessionResult>;
  getSession(input: GetSessionInput): Promise<ZitadelSession>;
  listSessions(ids: string[]): Promise<ZitadelSession[]>;
  deleteSession(input: DeleteSessionInput): Promise<void>;
  getAuthRequest(id: string): Promise<AuthRequest>;
  getLoginSettings(orgId?: string): Promise<LoginSettings>;
  listAuthenticationMethodTypes(userId: string): Promise<AuthMethodType[]>;
  isSessionValid(session: ZitadelSession): Promise<boolean>;
  createCallback(input: { authRequestId: string; sessionId: string; sessionToken: string }): Promise<{ callbackUrl: string }>;
}
```

所有返回值用 zod 校验后再进入业务层；服务端错误统一转成 `ZitadelApiError { code, id, message, failedAttempts? }`。
`isSessionValid` 必须独立于页面组件：WebAuthN `userVerified=true` 才可算 passwordless 主因素，`false` 只算 U2F 二因素。

### 5.3 关键请求示例

以下 REST 示例用于契约探针与排障；主链路由 `@zitadel/client` 生成客户端调用同一语义，不手写字段映射。

创建 session（登录名已解析为 userId）：

```json
POST /v2/sessions
Authorization: Bearer <LOGIN_CLIENT_JWT>
{
  "checks": {
    "user": { "userId": "283399833706168162" }
  },
  "userAgent": {
    "fingerprintId": "f6d3...",
    "ip": "203.0.113.9",
    "description": "Chrome 139, macOS 15.2",
    "header": { "user-agent": { "values": ["Mozilla/5.0 ..."] } }
  },
  "lifetime": "86400.000000000s"
}
```

密码检查：

```json
PATCH /v2/sessions/283399833706168162
Authorization: Bearer <LOGIN_CLIENT_JWT>
{
  "checks": { "password": { "password": "..." } }
}
```

完成 OIDC：

```json
POST /v2/oidc/auth_requests/V2_224908753244265546
Authorization: Bearer <LOGIN_CLIENT_JWT>
{
  "session": {
    "sessionId": "283399833706168162",
    "sessionToken": "<最后一次响应中的 token>"
  }
}
```

### 5.4 安全约束

- `sessionToken` 只在服务端出现；login_flow cookie 加密存储，`SameSite=Lax`、`HttpOnly`、`Secure`。
- 每次 `setSession` 后必须把新 token 回写 cookie，绝不复用旧 token。
- state/PKCE/verifier 继续用现有 httpOnly cookie，OIDC state 恒定校验。
- 设备指纹用 `fingerprintId` cookie，随 session `user_agent` 上报，支持账号选择器与异常登录审计。
- MFA 判断放在 vLogBin 服务端：`isSessionValid` 读取 session factors + `ListUserAuthMethodTypes` + LoginSettings，不允许前端决定“是否已充分认证”。
- `CreateCallback` 本身只验证 session token，不校验 MFA；因此 `isSessionValid` 是 vLogBin 侧强制门槛，未通过不得创建回调。
- 主因素规则：password / passwordless（WebAuthN `userVerified=true`）/ IDP intent 可作主因素；U2F（WebAuthN `userVerified=false`）只能作二因素。
- 失败密码/OTP 的锁定与 tarpit 交给 ZITADEL 执行，vLogBin 只展示 `failedAttempts`，不自己实现爆破保护。
- 登录页错误文案按稳定 id 映射，禁止输出 ZITADEL 英文原始 message。
- 登录凭据不进入日志、错误页与浏览器源码；Login Client Key / 私钥只允许 Secret Manager 或文件挂载提供。

## 6. 测试与交付拆分

### 6.1 测试矩阵

- 单元：zod schema、错误映射、token cookie 加解密、`isSessionValid` 全分支。
- MFA 回归：password-only 拒绝、TOTP-only 拒绝、U2F-only 拒绝、passwordless 通过、密码+MFA 通过、policy forceMFA 分支。
- 集成：契约探针 + 真实 ZITADEL 的 create/set/get/delete/callback 全链路，覆盖凭据轮换与过期。
- E2E：真实 ZITADEL + Playwright，覆盖登录名、密码、TOTP、OTP email/SMS、Passkey、登出、过期、账号选择器。
- 契约探针：对目标 ZITADEL 版本实际调用五个 Session 端点，锁定响应字段与错误 id，作为 CI 第一步。
- 版本门禁：CI 断言 ZITADEL ≥4.6.0、`EnableRelationalTables` 未启用、`v2beta` 未被引用。

### 6.2 分批上线

1. P0：`zitadel-session.ts`（基于 `@zitadel/client`）+ zod + `isSessionValid` MFA 模块 + 契约/版本探针 + `AUTH_MODE=oidc-custom-login` 开关 + OIDC 代理骨架，不改现有页面。
2. P1：登录名/邮箱/手机号搜索 + 密码页替换，`login_flow` 加密 cookie 落地，失败自动回退原 OIDC 跳转。
3. P2：MFA（TOTP / OTP email/SMS / Passkey / U2F）与账号选择器。
4. P3：注册 / 邮箱验证 / Passkey 初始化 / MFA 初始化跳过，未覆盖的 IDP/SAML 流程显式回退托管登录页，复用 `provisionWorkspace`。
5. P4：全量灰度（按 env/user/org 放量），观察成功登录率、MFA 完成率与 fallback 率后删除或冻结旧跳转路径，补充回归基线。

## 7. 风险清单

- `v2beta` 会在下个 major 移除：任何新代码不得引用 `session/v2beta`。
- 传输契约：主链路使用 `@zitadel/client` 生成客户端；REST 仅探针，不假定 grpc-gateway 与 Connect 完全一致。
- 版本风险：当前 `third-party/zitadel` 为开发态快照，生产必须锁 v4.16.x（≥4.6.0）并持续跟进 CVE；禁止启用 `EnableRelationalTables`。
- 凭据风险：长期 PAT 泄露等于完整 Session API 权限；生产必须使用短期 JWT / Login Client Key 并纳入轮换审计。
- MFA 漂移：`isSessionValid` 是对 ZITADEL 策略逻辑的镜像实现，必须有 MFA 回归测试与版本探针，防止策略语义升级后放行或误拒。
- `CreateCallback` 不校验 MFA：任何遗漏 `isSessionValid` 的调用路径都会成为二因素绕过点。
- IDP/SAML 覆盖：未实现前必须显式回退托管登录页，禁止静默失败或降级跳过。
- 代理安全：OIDC 代理的目标后端必须来自受信配置白名单，防止 Host 头驱动 SSRF。
- `ListUsers` 搜索依赖组织上下文与登录策略，搜索失败必须走“防用户枚举”文案路径。
- Session API 与 OIDC 是配合关系：不要用 Session token 替代标准 OAuth token。
- AGPL 边界：vLogBin 只依赖 API 契约，参考实现限 MIT 的 `apps/login` 与 Apache-2.0 的 `proto`；生产前请法务复核。
