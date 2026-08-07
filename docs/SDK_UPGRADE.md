# ZITADEL SDK 升级治理

## 现状基线

- ZITADEL 服务基线：`v4.16.x`（生产门禁 `>=4.6.0`），当前验证镜像 `v4.16.2`。
- npm SDK 基线：`@zitadel/client` / `@zitadel/proto` 固定 `1.3.1`（2025-07-28 发布）。
- 结论：npm SDK 的发布节奏明显滞后于 ZITADEL 主仓库；`v2` API 本身保持向后兼容，因此当前实现可生产运行，但存在两个已知 compat shim。

## 已知 SDK 差距

| 差距 | SDK 1.3.1 | 当前实现 | 升级后动作 |
| --- | --- | --- | --- |
| `RetrieveIdentityProviderIntentResponse.user_action.create_user` | 未生成类型 | `idp-intent.ts` 解码 unknown field 6 | 删除 unknown 解码路径，保留 typed path |
| `CreateUserRequest.metadata` 顶层字段 | 未生成类型 | `withCreateUserMetadata` 编码 unknown field 6 | 删除编码 shim，改用 typed `metadata` |
| `@zitadel/client` 依赖策略 | 精确锁定 `1.3.1` | 无自动升级 | 升级后跑探针与真实 E2E |
| IdP intent 契约探针 | 基础探针不含 intent | `zitadel-contract-probe.mjs` | 增加真实 intent 校验 |

## 探针

```bash
cd apps/web
pnpm run probe:sdk
```

输出包含：

- 已安装 `@zitadel/client` / `@zitadel/proto` 版本。
- npm latest 版本（网络不可用时为 `unknown`）。
- 当前 SDK 是否已生成 `user_action.create_user`。
- 仍处于激活状态的 compat shim 清单。

该探针已接入 `.github/workflows/ci.yml` 的 `verify` job。

## 升级清单

1. 升级 `@zitadel/client` / `@zitadel/proto` 到新版本。
2. 运行 `pnpm run probe:sdk`，确认 `hasTypedCreateUser=true`。
3. 删除 `idp-intent.ts` 中的 unknown field 6 解码与顶层 metadata 编码 shim，改用 typed 字段。
4. 运行 `tsc`、ESLint、单元测试。
5. 运行 `scripts/ci-zitadel-e2e.sh` 真实 ZITADEL + Dex 双链路回归。
6. 更新本文档基线版本。

## 可行性

高。理由：

- ZITADEL `v2` Session / OIDC / User API 是稳定资源面，新增字段均为向后兼容扩展。
- 官方 Login App 的 npm 包与主仓库同源，MIT 许可；SDK 发布后可平滑迁移。
- 当前 compat shim 有单测覆盖（`idp-intent.test.ts`），typed path 优先，SDK 升级不改变业务代码语义。
- 备选方案：若 npm 长期停更，可从 `zitadel/zitadel` 官方 proto 用 `buf` 生成本地包，替换 npm 依赖；成本高于等待官方发布，但技术可行。
