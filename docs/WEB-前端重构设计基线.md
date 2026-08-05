# vLogBin 前端（apps/web）重构设计基线

文档版本：v1.4  
文档状态：**设计基线（Design Baseline）— 已评审，作为实施依据**  
评审日期：2026-08-01  
评审人：SaaS 高级产品经理（产品 / 交互 / 视觉 / 心智模型四维评审 + 零摩擦交互打磨 + 低学习成本心智模型打磨 + 平台级视觉质感打磨）  
适用范围：`apps/web`（Next.js App Router 前端）彻底重构  
关联文档：[ZITADEL-Lago平台级生产架构设计方案.md](./ZITADEL-Lago平台级生产架构设计方案.md)、[TECHNICAL.md](./TECHNICAL.md)、[SPEC-ZITADEL-Lago多Provider平台.md](./SPEC-ZITADEL-Lago多Provider平台.md)

---

## 0.1 评审变更摘要（v1.0 → v1.1）

| # | 变更 | 动因 |
|---|---|---|
| R1 | 新增 **First-Run 引导管线**（§2.2） | 产品：无引导则"10 分钟上线"无落地路径 |
| R2 | **M0 保留现有运营商台**，Provider CRUD + lifecycle 迁入 `/ops`（§5、§9） | 产品：现有功能不可丢弃；`/v1/operator/*` 已可用 |
| R3 | **删除侧边栏 Environments 项**，环境只驻顶栏切换器（§6.1） | 心智模型：环境是全局状态，不是功能模块 |
| R4 | **可见层命名统一 workspace**，provider 仅用于 API 层（§2.3） | 心智模型：避免内部术语泄露 |
| R5 | 环境切换器交互规范细化（§6.3） | 交互：全站最高频交互需确定性规范 |
| R6 | 破坏性操作统一 `ConfirmDialog` 模式（§7.2） | 交互：收敛为组件级模式 |
| R7 | 三态规范组件化：Loading / Empty / Error（§7.2） | 交互：空状态是 onboarding 主入口 |
| R8 | 语义色 token 体系 + 环境标识色独立（§7.1） | 视觉：单一强调色撑不起平台 |
| R9 | 暗色主题 = token 重构，非变量覆盖（§7.1） | 视觉：语义 token 双主题定义 |
| R10 | 新增品牌资产与数据表格规范（§7.3、§7.4） | 视觉：官网品牌完整性与数据可读性 |
| R11 | 注册即建 workspace，首用户自领 admin（§3.1） | 产品：角色模型地基必须定死 |
| R12 | Portal 信息架构补充（§8.2） | 产品：终端客户侧认证与数据来源需明确 |
| R13 | 新增 **零摩擦交互原则**（§6.5，五法则 + 场景细节） | 交互：0 摩擦为最高产品哲学，全站交互细则收敛为一节 |
| R14 | 会话静默续期 + 过期友好处理（§3.3） | 交互（L3）：避免工作中被突然踢回登录页 |
| R15 | 表单摩擦消除：键盘/即时校验/防双击/保留输入（§6.5.2） | 交互（L2/L3/L4） |
| R16 | 列表筛选 URL 化：搜索/排序/分页可分享可回退（§6.5.3） | 交互（L3） |
| R17 | 密钥只显示一次 + 离开提醒 + 一键复制（§6.5.4） | 交互（L3/L4） |
| R18 | 引导进度条可点击回跳 + 跳过入口（§6.5.5） | 交互（L1） |
| R19 | 写操作乐观更新 + 失败回滚（§6.5.6） | 交互（L2） |
| R20 | 错误文案可行动 + 页面级 ErrorBoundary（§6.5.6） | 交互（L5） |
| R21 | 窄屏适配：侧边栏折叠抽屉、环境切换器收进菜单（§6.5.7） | 交互（响应式摩擦） |
| R22 | 可达性即零摩擦：键盘/焦点环/aria + reduced-motion（§6.5.8） | 交互（无障碍摩擦） |
| R23 | 新增 **心智映射表**（§2.4）：概念人话化 + 视觉隐喻 | 心智：术语黑话是最大学习成本 |
| R24 | **一致动作语法**：全站按钮 `[动词]+[对象]`（§6.6.1） | 心智：可预测的按钮语义 |
| R25 | **渐进披露**：每页单一主 CTA、高级设置折叠（§6.6.2） | 心智：首屏不过载 |
| R26 | **可预测下一步**：完成态双出口（继续/返回）（§6.6.3） | 心智：无死胡同 |
| R27 | **页面地名志**：每页定位语 + 首屏导览（§6.6.4） | 心智：用户始终知道"我在哪、能干嘛" |
| R28 | **模式记忆**：偏好持久化（§6.6.5） | 心智：不重复决策 |
| R29 | **品牌色彩系统**：墨青主色 + 暖中性灰 + 墨夜暗色，脱离模板质感（§7.1） | 视觉：脱离 Tailwind 默认模板 |
| R30 | **品牌资产升级**：V 形 mark + 终端/日志隐喻 + 品牌语音（§7.3） | 视觉：凸显 vLogBin 个性化 |
| R31 | **表面层级四层制**：canvas/surface-1/2/3 + 1px 边框 + 微阴影（§7.1） | 视觉：告别平板卡片 |
| R32 | **反模板化清单**：禁止默认质感/emoji/纯白卡片（§7.6） | 视觉：可执行的质量红线 |
| R33 | **质感微规范**：焦点环/骨架脉冲/图表渐变/数字 mono（§7.7） | 视觉：精致度细节 |

---

## 0. 产品愿景与定位

> **vLogBin —— 让 SaaS 开发者 10 分钟上线「身份 + 计费」，无需自建。**

平台以"多 SaaS Provider 的身份与计费基础设施"为定位，前端需要支撑五种产品面：

| 产品面 | 目标用户 | 说明 |
|---|---|---|
| 官网面（Marketing） | 潜在 SaaS 开发者 | 产品介绍、定价、文档入口、注册引导 |
| 认证面（Auth） | 平台用户 | OIDC 登录 / 注册 / 回调 / 登出 |
| **Console（重构主轴）** | SaaS Provider 开发者与管理员 | 管理身份、计费、客户、套餐、开发者资源 |
| 运营商台（Ops） | vLogBin 平台运营人员 | Provider 准入审核、风险、Cell 运维 |
| 客户门户（Portal） | Provider 的终端客户 | 查看账单、用量、支付 |

现状诊断：现有 `apps/web` 仅为"运营商内部管理台"（Provider CRUD + lifecycle 操作页），无登录、无多租户、无设计系统。**差距：缺官网、登录、Console、客户门户 4 个产品面。**

---

## 1. 技术栈基线

沿用现有栈，不引入重型组件库，全部原子组件自写：

| 领域 | 选型 | 版本基线 | 说明 |
|---|---|---|---|
| 框架 | Next.js App Router（RSC + Server Actions） | 16.x（当前 16.2.12） | 服务端组件优先 |
| 语言 | TypeScript strict | ~5.9 | 全量 strict，禁止 `any` 逃逸 |
| 样式 | Tailwind CSS v4 + CSS 变量 tokens | 4.x | 对齐 `prototype/proto.css` 的视觉语言 |
| 字体 | Geist | — | 沿用现有 |
| 表单校验 | zod | — | Server Action 入参 + 客户端共用 schema |
| OIDC 校验 | jose | — | 服务端校验 id_token，不下发浏览器 |
| 状态管理 | 客户端 React Query（如需） | — | RSC 直取优先，React Query 做缓存 |
| 测试 | Playwright E2E | ^1.62（已配置） | `apps/web/e2e/` |

依赖策略：**仅允许** `next/react/react-dom`、`zod`、`jose`、`@tanstack/react-query` 等必要依赖；图表用自绘 SVG，不引图表库；UI 不引 shadcn/ui 运行时，仅参考其模式自写。

---

## 2. 多租户与环境模型（前端视角）

### 2.1 环境模型

- 每开发者 = 1 **workspace**（API 层称 provider，见 §2.3 命名规范）；登录后 Session 携带 `workspace_id`（即 `provider_id`）。
- 每 workspace 内 **test / live** 环境隔离，URL 显式携带 `?env=`（如 `?env=test`）保证刷新不丢状态；环境选择持久化到 cookie，URL 参数优先。
- 所有 `/v1` 数据请求附加 `X-Environment` 头（或对应 query 参数），由 `apps/api` 侧强制隔离，前端仅透传。
- 环境相关 UI 约束：test 环境可自由创建/删除测试数据；live 环境操作需二次确认，破坏性操作（删除客户、出账）走 `ConfirmDialog` type-to-confirm（§7.2）。
- **环境标识色独立**：test = amber、live = red，恒显于顶栏徽标，不与 success/warning/danger 状态色混淆（§7.1）。

### 2.2 First-Run 引导管线（R1）

新用户首次登录后必须走完一条引导序列，实现"10 分钟上线"承诺。状态机持久化于 `lib/onboarding.ts`，按序推进，允许跳过但空状态始终引导补齐：

```
1. 创建第一个应用（OIDC Application）
   └─ 配置回调 URL → 完成态: 应用已创建
2. 创建第一个套餐（Plan）
   └─ 设定价格模型 → 完成态: 套餐已上线
3. 创建第一个客户（Customer）
   └─ 触发订阅 → 完成态: 客户已订阅
4. 上报第一条用量事件
   └─ 提供 SDK/curl 示例 → 完成态: 计量打通
```

落地要素：
- 每个数据页的 `EmptyState` 显示当前管线步骤 + 主 CTA（§7.2）。
- 创建类表单成功后弹 `SuccessPanel`（下一步建议链接 + 可复制密钥），支撑管线推进（§7.2）。
- Overview 页顶部展示引导进度条（`x/4`），全部完成自动隐藏。

### 2.3 命名规范（R4）

**用户可见层一律使用 workspace / 工作区**；`provider` 仅出现在 API 文档与内部代码。

| 层 | 术语 |
|---|---|
| URL | `/console`（无 provider 字样） |
| 页面文案 / 顶栏 | workspace 名 |
| API 参数 | `provider_id`（内部） |
| 侧边栏分组标题 | 工作区 / 产品 / 开发者 |

### 2.4 心智映射表（R23）

**原则**：用户带着已有心智进入产品。每个平台概念必须映射到用户熟悉的概念，用"人话 + 视觉隐喻"降低认知成本。以下为全站统一映射，任何新增概念须先入此表：

| 平台概念 | 用户心智（熟悉概念） | 人话解释（页面用词） | 视觉隐喻 |
|---|---|---|---|
| workspace | 项目 / 工作区（GitHub Org） | "你的 SaaS 产品的家" | 网格方块 |
| environment（test/live） | 草稿模式 / 发布模式（Stripe test/live） | "测试环境随便试，正式环境对真实客户生效" | 盾牌（TEST 琥珀 / LIVE 红） |
| application（OIDC） | 你的应用的"接入点" | "告诉 vLogBin 你的产品是谁" | 应用方块 |
| plan | 价格方案（订阅套餐） | "你卖给客户的定价方案" | 价格签 |
| customer | 客户（CRM 心智） | "使用你产品的终端客户" | 人头 |
| usage event | 用量记录（埋点/日志心智） | "客户用了多少，你上报一条记录" | 仪表 |
| invoice | 账单（水电账单心智） | "自动生成的应收账单" | 收据 |
| api key / webhook | 密钥 / 回调（开发者熟悉） | "程序访问的凭证 / 事件通知" | 钥匙 / 闪电 |
| policy / entitlement | 权限 / 权益（角色心智） | "谁能用什么功能" | 锁 / 勾选 |

落地要求：任何页面首次出现上表概念时，旁置"名词解释"（tooltip 或 `info` 图标），文案用"人话"列。

---

## 3. 认证与会话架构

### 3.1 流程

1. 用户访问 `/console` 未登录 → 中间件重定向到 `/login?next=<return_to>`。
2. `/login` 发起 ZITADEL OIDC **Authorization Code + PKCE**：生成 `code_verifier`/`code_challenge` 存入临时 cookie，跳转 ZITADEL。
3. 回调 `/auth/callback`：换取 token，服务端用 `jose` 校验 id_token（issuer/audience/exp/nonce），提取 `workspace_id`、角色、组织等声明。
4. 建立会话：httpOnly + Secure + SameSite=Lax cookie（自签 JWT session，含 `workspace_id`、`env`、角色），`maxAge` 与 ZITADEL 会话对齐。
5. `/auth/logout`：清 cookie，跳转 ZITADEL 登出端点。

**注册决策（R11）**：`/signup` 一次性完成 —— 创建 ZITADEL 平台用户 → 同步创建默认 workspace → 首用户自动授予 `provider_admin`。三者任一失败则整体回滚，保证角色模型地基一致。

### 3.2 安全要点

- 令牌（access/refresh/id token）**只存在服务端**，永不下发浏览器；浏览器只持自签 session cookie。
- PKCE `code_verifier` 一次性使用；回调后立即清理。
- 中间件（Edge/Middleware）做粗粒度路由守护（`/console`、`/ops`），**Server Action 与服务端组件内做细粒度 RBAC 二次校验**，中间件不信任。
- `OPERATOR_TOKEN` 等平台密钥只存服务端环境变量，现有原型中客户端直调敏感接口的问题必须根除。

### 3.3 会话静默续期与过期友好（R14，交互 L3）

- 会话过期前约 5 分钟，服务端静默刷新（refresh token 轮换），用户无感知。
- 若刷新失败：**不立即踢出**，先弹非阻塞提示"会话即将过期，请重新登录"，保留 `next` 回跳并携带原上下文（当前路由 + 环境 + 筛选），登录后原样返回。
- 强制过期仅发生在访问受保护数据时；阅读态页面允许看到缓存内容，写操作才要求会话有效。

### 3.3 RBAC 模型

从 id_token / session 提取声明：

- `provider_admin`：workspace 全量管理（默认角色，首个用户）
- `provider_developer`：开发者资源（API Keys、Webhooks、事件）
- `provider_billing`：仅计费域
- `platform_operator`：`/ops` 运营商台

实现：`lib/auth/session.ts` 提供 `requireRole(role)` / `requireProvider(providerId)` 工具，Server Action 与 RSC 统一调用。

---

## 4. 数据获取策略

| 场景 | 策略 |
|---|---|
| 页面首屏 | 服务端组件直取（RSC），直接 `fetch` `/v1` API |
| 高频缓存 | 客户端 React Query（`staleTime` 合理配置） |
| 写操作 | Server Actions 统一网关（`src/app/actions.ts`），乐观更新 + `useActionState` |
| 环境切换 | 服务端读 cookie + URL 参数，刷新时由 RSC 重取 |

约定：

- `lib/api/` 提供类型化 client：`apiClient(env, token?)`，统一 base URL、错误解析、超时。
- 所有 API 契约类型集中在 `lib/api/types.ts`，与 `docs/openapi.yaml` 同步维护。
- Server Actions 入参一律 zod 校验，错误以结构化 `ActionState` 返回（沿用 `action-state.ts` 模式扩展）。

---

## 5. 目标目录结构

```
apps/web/src/
├── app/
│   ├── (marketing)/page.tsx            # 官网（SSG）
│   ├── (auth)/
│   │   ├── login/page.tsx              # 登录
│   │   ├── signup/page.tsx             # 注册（引导建 Provider）
│   │   └── callback/page.tsx           # OIDC 回调
│   ├── (console)/
│   │   └── console/
│   │       ├── layout.tsx              # 侧边栏 + 顶栏 + 环境切换
│   │       ├── overview/page.tsx       # 概览
│   │       ├── identity/
│   │       │   ├── applications/page.tsx
│   │       │   ├── users/page.tsx
│   │       │   └── policies/page.tsx
│   │       ├── billing/
│   │       │   ├── dashboard/page.tsx
│   │       │   ├── customers/page.tsx
│   │       │   ├── plans/page.tsx
│   │       │   ├── invoices/page.tsx
│   │       │   └── payments/page.tsx
│   │       ├── catalog/page.tsx
│   │       ├── developers/
│   │       │   ├── api-keys/page.tsx
│   │       │   ├── webhooks/page.tsx
│   │       │   └── events/page.tsx
│   │       └── settings/page.tsx
│   ├── (ops)/ops/...                   # 运营商台（审核、风险、Cell）
│   ├── (portal)/portal/...             # 客户门户（账单、用量、支付）
│   ├── actions.ts                      # 统一 Server Actions 网关
│   ├── action-state.ts                 # ActionState 类型（扩展）
│   └── layout.tsx / globals.css
├── components/
│   ├── ui/                             # 原子组件（button/input/card/badge/dialog/table/tabs/toast/confirm-dialog/empty-state/error-state/skeleton/success-panel...）
│   ├── charts/                         # 自绘 SVG 图表（sparkline/bar/line）
│   └── console/                        # 页面级组合（app-table/customer-card/invoice-list/onboarding...）
├── lib/
│   ├── api/                            # 类型化 API client + types
│   ├── auth/                           # OIDC 会话、jose 校验、RBAC
│   ├── env/                            # 环境切换上下文/工具
│   ├── onboarding.ts                   # First-Run 引导状态机（§2.2）
│   ├── validate/                       # zod schemas
│   └── utils/                          # cn、格式化（金额/时间）
├── hooks/                              # useActionState 封装、React Query hooks
└── styles/                             # design tokens（CSS 变量，语义 token 双主题）
```

---

## 6. Console 信息架构

### 6.1 导航结构（侧边栏三大分组）

- **工作区**
  - Overview：MRR / 活跃客户 / 用量事件 / 待付发票 指标卡 + 趋势图 + 引导进度条（§2.2）
  - Identity（副标题"身份与访问 · 认证应用"）：Applications / Users / Policies
  - Billing（副标题"计费 · 订阅 · 收入"）：Dashboard / Customers / Plans / Invoices / Payments
  - Catalog
- **产品**（预留扩展位）
- **开发者**
  - Developers：API Keys / SDK / 事件规范
  - Webhooks
  - Settings

> **注意（R3）**：侧边栏**不设 Environments 导航项**。环境是全局状态而非功能模块，只存在于顶栏切换器 + live 徽标，符合 Stripe test/live mode 的成熟心智。

### 6.2 顶栏

- **环境切换器**（test/live 分段控件，常驻）
- **live 环境恒显红色徽标 "LIVE"**（test 显示琥珀色 "TEST"），位于 workspace 名右侧
- 当前用户菜单（头像、workspace 名、登出）
- 全局搜索（预留）

### 6.3 环境切换交互规范（R5）

1. **切换不改变当前子路由**，只刷新数据（URL 写 `?env=`，路径不动）。
2. 切换瞬间顶栏进度条 + 内容区 skeleton，禁止整页白闪。
3. 切到 live 后破坏性按钮自动升级为二次确认（ConfirmDialog）。
4. 环境选择持久化 cookie，URL 参数优先级最高；手动清除 `?env=` 回落 cookie，再回落 test。
5. 切换后 toast 提示"已切换到 live 环境"。

### 6.4 全局定位锚点（R12/M4）

顶栏恒显：workspace 名 + 环境徽标 + 用户头像菜单；侧边栏当前项高亮；二级页显示面包屑（如 工作区 / 计费 / 套餐）。

### 6.5 零摩擦交互原则（R13-R22）

**北极星：任何一次交互满足"三不"——不打断、不等待、不丢失。**

五法则（详细定义见 §0.1 变更表）：**L1 不打断** / **L2 不等待** / **L3 不丢失** / **L4 不重复** / **L5 不困惑**。以下为落地细节。

#### 6.5.1 全局反馈（L1/L2/L5）

- Toast 一律**非阻塞**、3 秒自动消失、可手动关闭；出现多个时堆叠不覆盖操作区。
- 确认弹窗只用于**真正破坏性**操作（删除、出账、吊销、恢复）；普通操作用乐观更新 + 可撤销 toast。
- 任何写操作按钮有 pending 态（spinner + 禁用），防双击重复提交。

#### 6.5.2 表单摩擦消除（R15，L2/L3/L4）

- 键盘友好：首个字段 autofocus；Enter 提交；Tab 顺序按视觉布局。
- 校验即时反馈：字段 onBlur 触发单字段校验，提交时全量校验；错误**内联展示**（红边框 + 下方消息 + `aria-invalid`），不用弹窗/alert。
- 必填项标记 `*`；placeholder 即真实示例（如 `acme`、`https://api.example.com/callback`）。
- 智能预填（L4）：slug / client_id / webhook URL 由名称自动生成，可编辑；输入时实时预览最终 URL / curl。
- **失败不丢输入**（L3）：校验失败后已填内容全部保留，聚焦首个错误字段；提交按钮从 pending 恢复可再点。
- 成功态可见：创建类表单成功后显示 `SuccessPanel`（§7.2），表单重置与成功反馈**同时发生**，杜绝"以为没提交"。

#### 6.5.3 列表与筛选（R16，L3）

- 搜索词、筛选条件、排序、分页**全部 URL 化**（`?q=&status=&sort=&page=`），可分享、可回退、刷新不丢。
- 分页切换保留筛选；操作失败 toast + 保留当前筛选状态。
- 环境切换后列表重新请求，但**滚动位置保留**；`?env=` 与 cookie 冲突时 URL 优先并回写 cookie（确定性策略）。

#### 6.5.4 密钥与复制（R17，L3/L4）

- 密钥生成后**只显示一次**，默认密码模式（点击显示），旁置一键复制（复制成功 toast "已复制"）。
- 若密钥尚未复制即离开页面/关闭弹窗，给一次性提醒（不强制）。
- 密钥列表只显示前缀 + 创建时间，完整值一律"重新生成"换取。

#### 6.5.5 引导管线交互（R18，L1）

- 引导进度条（§2.2）**可点击回跳**任意已完成步骤；"跳过引导"入口始终可见，不强制。
- 步骤状态持久化（cookie/服务端），跨会话可续；跳过/完成后续可从空状态随时补齐。

#### 6.5.6 写操作与错误（R19/R20，L2/L5）

- 写操作优先乐观更新（React Query），服务端确认后校正；失败回滚 + toast 说明原因。
- 错误文案结构 = **发生了什么 + 为什么 + 下一步动作**（如"客户删除失败：仍有未出账单。请先结清账单后重试"），不展示裸状态码。
- 页面级 ErrorBoundary 兜底：友好文案 + "重试" + "返回上一页"，避免白屏。

#### 6.5.7 窄屏与响应式（R21）

- 侧边栏在 `<lg` 折叠为抽屉（汉堡入口），选择项后自动收起。
- 环境切换器在窄屏**收进用户菜单**（不溢出顶栏）；live 徽标仍常驻可见。

#### 6.5.8 可达性 = 零摩擦（R22）

- 全站键盘可达（Tab 顺序、Enter/Space 激活、Esc 关闭弹窗）；焦点可见环（focus-visible）。
- 交互控件有 aria-label / aria-describedby；错误信息与输入框关联。
- `prefers-reduced-motion` 时全局降级动画；页面切换用 view transition 平滑过渡，避免不必要动效。

### 6.6 低学习成本心智模型（R23-R28）

**北极星：用户不学就会用。** 三个断言——① 概念必须映射熟悉心智（§2.4）；② 按钮语义全局可预测；③ "下一步"永远可推断、无死胡同。

#### 6.6.1 一致动作语法（R24）

- 全站主按钮 = **`[动词]+[对象]`**，动词词表统一：`创建 / 保存 / 更新 / 删除 / 生成 / 吊销 / 重试 / 取消`；如"创建应用""生成密钥""出账"。
- 禁止风格漂移（`New`/`Add`/`Create` 混用）；英文环境映射同表。
- 破坏性按钮红色 + 动词明确（`删除客户`而非 `移除`）；次级操作用图标 + tooltip。
- 主 CTA 位置固定：页面右上角（列表页）/ 弹窗右下角（表单页）。

#### 6.6.2 渐进披露（R25）

- **每页首屏只有一个主 CTA**；次要能力收进"更多选项"折叠或二级入口。
- 表单默认只显示必填核心字段，高级字段（webhook 签名、自定义头、重试策略）收进"高级设置"折叠，展开后可记忆展开状态。
- 设置页按心智分组：基础 / 安全 / 高级，避免扁平长列表吓退用户。

#### 6.6.3 可预测下一步（R26）

- **任何完成态给双出口**：主出口"继续下一步"（引导管线中的下一步），次出口"返回列表"。
- 空状态 CTA 文案 = 该页面的首步动作（"创建第一个应用"），点击后直达表单。
- 详情页提供"相关导航"：客户详情 → 订阅 / 用量 / 账单入口并排。

#### 6.6.4 页面地名志（R27）

- 每个一级页面顶部一行定位语：**"我在哪 · 能做什么 · 第一步"**（如 Customers："客户 · 管理使用你产品的终端客户 · 导入或创建第一个客户"）。
- 新功能 / 新入口加"新增"徽标（非打断，仅标识，首见后消失）。
- 空状态页必须说明该页价值，避免"空白即困惑"。

#### 6.6.5 模式记忆（R28）

- 记住用户偏好并持久化：上次环境（§6.3 已有 cookie）、表格列宽、筛选条件、折叠状态、暗色主题。
- 高频动作置顶（最近使用的应用 / 客户排在列表头部）。
- 语言与地区偏好遵循系统，可手动覆盖并记忆。

---

## 7. 设计系统

### 7.1 Design Tokens（`styles/tokens.css`）

**语义 token 体系（R8/R9/R29/R31）**：组件只消费语义 token，不碰原始色值。主题为 `[data-theme="light"|"dark"]` 下的**完整 token 定义**，而非变量覆盖。默认跟随系统，用户可手动切换并持久化。

**品牌色彩系统（R29）** —— 脱离 Tailwind 默认质感的三个关键决策：

1. **墨青（Ink Teal）主色**：`#0d9488` 精修为品牌墨青族 `#0f766e → #0d9488 → #14b8a6`，深青收边、亮青高光，比默认 teal 更沉稳高级；`--color-primary-ink` 用于主按钮文字，保证 AA 对比。
2. **暖中性（Warm Neutral）替代纯灰**：`zinc` 灰改为带 3-5% 暖调的 `#fafaf9 / #f5f5f4 / #e7e5e4 ...`，页面观感更柔和，与墨青形成"暖底冷墨"的品牌对比。
3. **墨夜（Ink Night）暗色主题**：暗色非纯黑，用 `#0c0e12`（带青底调）做画布，表面层逐级提亮，边框 `#1f242c`；文字主 `#e7eaef`。

色彩（语义层，每色含 hover/active 档位）：

- **表面四层制（R31）**：
  - `--color-canvas`：页面底（light 暖白 / dark 墨夜）
  - `--color-surface-1`：卡片（1px 边框 + 微阴影，非纯白平板）
  - `--color-surface-2`：内嵌区（输入框、代码块、表格斑马行）
  - `--color-surface-3`：浮层（tooltip / popover / dialog，阴影最深）
- 边框：`--color-border`（surface 分隔）/ `--color-border-strong`（focus 容器、表格外框）
- 文本层：`--color-text / --color-text-muted / --color-text-faint`（对比度 ≥ AA）
- 品牌强调：`--color-primary / --color-primary-hover / --color-primary-soft / --color-primary-ink`（墨青族）
- 语义态：`--color-success / --color-warning / --color-danger / --color-info`（各含 soft/ink 档位）
- **环境标识色（独立）**：`--color-env-test`（琥珀）/ `--color-env-live`（红），仅用于环境徽标与切换器，不与状态色混用
- 禁用态：`--color-disabled / --color-disabled-fg`

几何与排版：

- 圆角语汇：`--radius-sm`(6) 控件 / `--radius-md`(10) 卡片 / `--radius-lg`(14) 弹窗 / `--radius-full` 徽标胶囊
- 间距：`--space-1..8`（8px 网格节奏）
- 阴影三重：`--shadow-sm`（1px 边框 + 2px 微影，常态）/ `--shadow-md`（hover 抬升 2px）/ `--shadow-lg`（浮层，柔和双影）
- 字体：`--font-sans`（Geist）/ `--font-mono`（Geist Mono，**用于 ID/密钥/金额/时间戳/代码块**，强化"日志"品牌气质）；字号层级：display / h1-h4 / body / caption

### 7.2 原子组件（`components/ui/`）

- 基础：`button / input / select / checkbox / switch / label / card / badge / alert / dialog / drawer / dropdown / tabs / toast / tooltip / spinner / pagination / copy-button`
- **三态组件（R7）**：
  - `Skeleton`：加载骨架，区块级脉冲
  - `EmptyState`：图标 + 说明 + 主 CTA（承接 §2.2 引导管线，显示当前步骤）
  - `ErrorState`：错误信息 + 重试按钮 + 反馈入口
- **`ConfirmDialog`（R6）**：破坏性操作统一模式 —— 红色主按钮 + **type-to-confirm 输入框**（输入资源名确认）+ busy 态 + 取消。删除客户 / 出账 / 吊销密钥等一律走它。
- **`SuccessPanel`（R4）**：创建类表单成功后的"下一步"面板（建议链接 + 可复制密钥/示例），支撑引导管线。

组件交互契约（对齐 §6.5 零摩擦原则）：

- 所有写操作组件：pending 态内置（禁用 + spinner），禁止自行再包 `disabled` 逻辑
- `ConfirmDialog`：type-to-confirm + busy 态 + Esc 可关 + focus 初始落输入框
- `Toast`：非阻塞堆叠、3s 自动消失、可手动关闭
- 表单组件：错误内联展示 + `aria-invalid`，Enter 提交
- `DataTable`：筛选/排序/分页 URL 化（§6.5.3）

全部服务端组件兼容（`"use client"` 仅交互组件），统一 ref 转发与 className 合并（`cn` util）。

### 7.3 品牌资产（R10/R30）

**品牌核心隐喻（R30）**：vLogBin = **"v"（矢量/V 形）+ "Log"（日志）+ "Bin"（收纳容器）**。产品天然自带"开发者日志 + 计量"气质，视觉语言与之强绑定：

- **V 形 mark**：墨青渐变的 V 形几何（`linear-gradient(135deg, #14b8a6, #0f766e)`），负空间可理解为日志行收束进容器，应用在官网、登录页、Console 顶栏、favicon。
- **终端/日志隐喻**：官网 hero 用**终端窗口**（macOS 三键 + 代码块）展示"10 分钟上线"的 curl / SDK 片段——这是最直观的品牌差异化表达，也是产品价值主张的可视化。
- **代码块组件 `CodeBlock`**：surface-2 内嵌、行号、一键复制、mono 字体，贯穿官网与 Console（引导管线第 4 步的 SDK 示例、Webhooks 签名示例、API Keys 使用说明）。
- **favicon / app icon**：V 形渐变图形 + 墨夜底。
- **品牌语音**：开发者向、命令式、极简——"Deploy in minutes"（不要 "Empower your business"）；中文语音"上线身份与计费，十分钟"。
- **官网视觉语言**：几何语言（V 形切角、圆角 8/12/20）、hero 墨青渐变 + 暖中性底、终端窗口组件、定价卡三态（当前/推荐/企业）。

### 7.4 数据表格规范（R10，`DataTable` 组件）

- sticky 表头；金额右对齐 + mono 字体；状态徽标左置圆点
- 空列显示 `—`；行 hover 高亮 + 行内操作菜单（kebab）
- 分页：底部页码 + 每页条数选择；大数据量默认服务端分页
- 列宽：标识列（ID/名称）自适应，数值列固定宽

### 7.5 图表（`components/charts/`）

自绘 SVG：sparkline、bar、line、area、donut；统一坐标轴、tooltip、动画规范，无第三方依赖。

质感细节（对齐 R33）：

- 面积图用墨青渐变（`#14b8a6 → transparent`，透明度 0.25 渐变到 0）；折线主色 `--color-primary`，端点圆点 3px 描边 surface-1
- 网格线 1px `--color-border`，坐标轴标签 `--color-text-faint`
- bar 用 `--color-primary-soft` 底 + `--color-primary` 顶（纵向渐变），hover 抬升透明度
- tooltip：surface-3 浮层 + 2px 主色左描边 + mono 数值
- 金额/计数一律 `tabular-nums` + `--font-mono`，数字竖列对齐

### 7.6 反模板化清单（R32，质量红线）

**禁止项**（出现即视为不通过 Review）：

1. 纯白卡片 + `#e4e4e7` 灰边框 + 默认 teal 按钮 = Tailwind 默认质感（必须用品牌 token 体系）
2. emoji 作图标或空状态插图（一律用 SVG 几何图标 / V 形资产）
3. 硬投影大黑阴影（`shadow-lg` 无边框感的悬浮）
4. 渐变滥用（渐变只用于品牌资产位：mark、hero、面积图、选中态）
5. 千篇一律居中 hero"大标题 + 三卡片"（官网 hero 必须含终端窗口差异化组件）
6. 文案模板腔（"Empower / Supercharge / Unlock" 等空话，见 §7.3 品牌语音）

### 7.7 质感微规范（R33）

- **焦点环**：2px `--color-primary` 40% 透明度外环 + 1px 内描边，全站键盘焦点统一
- **hover 过渡**：120ms ease-out，仅 transform/opacity/box-shadow（CSS 硬件加速）
- **骨架脉冲**：surface-2 底 + 墨青 10% 高光扫动，区块级
- **数字与代码**：所有 ID / 密钥 / 金额 / 时间戳 / 状态码用 `--font-mono` + `tabular-nums`
- **状态徽标**：左置 6px 圆点（语义色）+ 文字，胶囊底 soft 色
- **空状态插图**：几何图形（V 形 / 终端提示符 `>`）而非 emoji，墨青 outline 描边
- **暗色主题校验**：对比度 ≥ AA、无纯黑纯白、边框层级清晰（墨夜族）

---

## 8. 页面级规格（MVP 优先级）

| 页面 | 路径 | 优先级 | 关键元素 |
|---|---|---|---|
| 官网 | `/` | M0 | Hero、三卡（身份/计费/开发者）、定价、CTA |
| 登录 | `/login` | M0 | OIDC 跳转、`next` 回跳 |
| 注册 | `/signup` | M0 | 创建平台用户 + 默认 workspace + 首用户 admin |
| 回调 | `/auth/callback` | M0 | 建会话、回跳 |
| Console Layout | `/console` | M0 | 侧边栏、顶栏、环境切换、live 徽标 |
| Overview | `/console/overview` | M1 | 4 指标卡 + 趋势图 + 引导进度条 |
| Applications | `/console/identity/applications` | M1 | 应用列表/创建/密钥 |
| Plans | `/console/billing/plans` | M1 | 套餐 CRUD、定价模型 |
| Customers | `/console/billing/customers` | M1 | 客户列表/详情 |
| Invoices | `/console/billing/invoices` | M1 | 发票列表/明细 |
| API Keys | `/console/developers/api-keys` | M2 | 密钥创建/轮换/吊销 |
| Webhooks | `/console/developers/webhooks` | M2 | 端点/签名/重试 |
| Events | `/console/developers/events` | M2 | 事件流查看 |
| Settings | `/console/settings` | M2 | 环境、域名、通知 |
| Ops | `/ops` | M0（骨架）/ M3（增强） | Provider CRUD + lifecycle（自现状迁入）、审核队列、风险、Cell |
| Portal | `/portal` | M3 | 账单、用量、支付（见 §8.2） |

### 8.2 Portal 信息架构（R12）

- 访问方式：客户通过邀请链接 `portal.vlogbin.io?customer=<id>` 进入，**客户侧会话**（与平台用户会话隔离，独立 cookie 域）。
- 页面：账单（Invoice 列表 + 明细 PDF）、用量（Usage 按套餐聚合 + 周期）、支付（Payment 方式 + 历史 + 自动扣款开关）。
- 数据来源：`/v1` 的 customers/invoices/metered 端点，但客户侧仅能读取**自己**的数据 —— 由 `apps/api` 以 customer 级 token 强制数据域隔离，前端只透传 `customer_id`。
- 与 Portal 同源的视觉：复用 design tokens，但顶栏品牌为**客户所属 provider 的 workspace 名**，无 Console 管理入口。

---

## 9. 实施路线图

### M0 — 基座（本阶段实施起点）

- 目录重构为目标结构
- design tokens（语义双主题）+ `components/ui` 原子组件（含三态、ConfirmDialog、SuccessPanel）
- OIDC 登录 / 注册（建 workspace + 首用户 admin）/ 回调 / 登出 + 中间件 RBAC
- 类型化 API client + zod 校验
- 官网（SSG）+ Console 布局（侧边栏/顶栏/环境切换）
- **运营商台骨架（R2）**：现有 Provider CRUD + lifecycle 状态机（`lifecycle-actions.tsx` 迁移）迁入 `/ops`，调用现有 `/v1/operator/*`
- First-Run 引导状态机（`lib/onboarding.ts`）与 Overview 引导进度条
- Playwright 冒烟（登录流、环境切换、运营商台 lifecycle）

**M0 验收**：未登录访问 `/console` 重定向登录；登录后进入 Console；test/live 切换后 URL 显式携带 `?env=` 且数据源切换；现有 provider lifecycle 操作在 `/ops` 完整可用；首登用户可见引导管线。

**M0 零摩擦抽查**（对齐 §6.5）：① 表单校验失败不丢已填内容、聚焦首个错误字段；② 提交按钮有 pending 态防双击；③ 切换环境不白闪、保留滚动位置；④ 密钥生成后可一键复制、未复制离开有提醒；⑤ 全部交互键盘可达、焦点可见；⑥ `prefers-reduced-motion` 生效。

**M0 心智抽查**（对齐 §6.6）：① 全站按钮文案为 `[动词]+[对象]`，无混用；② 每个一级页有定位语（我在哪·能做什么·第一步）；③ 表单高级字段默认折叠、可记忆展开；④ 完成态有双出口（继续/返回）；⑤ 首次出现的概念旁置人话解释（§2.4 映射表）；⑥ 环境选择、暗色主题偏好持久化。

**M0 视觉抽查**（对齐 §7）：① 无 Tailwind 默认质感（纯白卡/默认灰边框/默认 teal）；② 品牌 V 形 mark 用于官网与 Console 顶栏；③ 官网 hero 含终端窗口组件；④ ID/密钥/金额用 mono + tabular-nums；⑤ 无 emoji 图标；⑥ 暗色主题非纯黑、对比度 ≥ AA；⑦ 全站焦点环统一。

### M1 — 控制面 API（最大前置依赖）

`apps/api` 新增 Console 控制面端点（OIDC 应用、Plans、Policies、Settings、自定义域名）——因 ZITADEL/Lago 管控能力未暴露为 `/v1` API。

### M2 — Console 主流程

Overview、Identity、Billing、环境隔离端到端：注册应用 → 建套餐 → 建客户 → 订阅 → 上报用量 → 生成发票。

### M3 — 完善面

Developers、Settings、运营商台增强、客户门户、E2E 全量、暗色主题打磨、审计日志。

---

## 10. 风险与对策

| # | 风险 | 等级 | 对策 |
|---|---|---|---|
| 1 | 引擎管控 API 缺失：Lago `api/`（Rails）源码不在仓库、ZITADEL 管控面未包装为 `/v1` | 高 | 依赖 M1 控制面端点；先行以现有 `/v1` 可用端点实现 M0/M2 可验证部分 |
| 2 | AGPL 合规：ZITADEL 核心 AGPL-3.0 | 中 | 仅网络交互（OIDC/API），不内嵌修改源码；由法务确认边界 |
| 3 | 环境隔离正确性 | 高 | `apps/api` 侧强制 `X-Environment`；前端不承载隔离逻辑，只透传 |
| 4 | First-Run 引导依赖 M1 数据端点（创建应用/套餐/客户） | 中 | M0 落地引导状态机与 UI，M1 端点就绪后管线自动可用；M0 验收以状态机正确性为准 |
| 5 | live 二次确认摩擦可能导致误操作绕过 | 低 | 确认后需输入资源名，busy 态防重复提交；操作日志记录审计 |
| 6 | 范围控制 | 中 | 本期不做 Lago front 逆向移植、不做 `[locale]` 路由、不做多 region 控制面前端 |

---

## 11. 变更管理

- 本文件为**设计基线**，后续 PR 改动 `apps/web` 结构时须同步更新本文档。
- 重大决策（认证方案、目录结构、组件契约）变更须评审后在此记录 ADR 摘要。
- 与 `docs/openapi.yaml` 同步：前端类型以 openapi 为准，出现漂移时以 openapi 为准并更新 `lib/api/types.ts`。

---

## 附：与现有文档的关系

| 文档 | 关系 |
|---|---|
| ZITADEL-Lago平台级生产架构设计方案.md | 平台总体架构，本文件是其前端实现层 |
| TECHNICAL.md | 后端实现细节；前端调用的 `/v1` 端点以该文档为准 |
| SPEC-ZITADEL-Lago多Provider平台.md | 需求规格；本文件页面规格来自其 User Stories |
