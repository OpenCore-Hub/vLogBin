# SDK 发布与契约治理

## 契约源

公开 API 的唯一契约源是 `docs/openapi.yaml`。官方 SDK 支持的操作声明在
`sdk/operations.yaml`，每个操作绑定 OpenAPI 路径、方法、请求/响应 schema、
查询参数、幂等键语义，以及 Go / TypeScript / Python 三语言的公开函数名。

`scripts/sync-sdk-operations.py` 从这两份文件生成
`sdk/generated/manifest.json`。任何 OpenAPI 路径、schema、查询参数或
`x-idempotency-key` 变更都会在 `make contract` 中暴露为 SDK 契约漂移。

## 本地门禁

```bash
make contract   # OpenAPI / AsyncAPI / 错误码 / 类型 / SDK 操作清单
make sdk        # 三语言测试 + 操作符号校验 + 包产物指纹 diff
make release-gate
```

`scripts/check-sdk-contract.py` 校验每个清单操作在三个 SDK 中都有对应函数、
路径字符串、幂等键传递和查询参数。`scripts/check-sdk-artifacts.py` 固定
TypeScript `dist` 与 Python 包源码的 SHA-256 指纹，防止生成物无声漂移。

## 发布

```bash
scripts/publish-sdks.sh dry-run
scripts/publish-sdks.sh publish
```

- `dry-run`：执行完整 SDK 门禁，并输出 npm 包 dry-run 信息。
- `publish`：需要 `NPM_TOKEN` 与 `PYPI_TOKEN`，分别发布
  `@vlogbin/platform-sdk` 与 `vlogbin-platform-sdk`。
- 发布前必须通过 `make release-gate`。
- SDK 跟随平台 `v1` API 与 12 个月并行兼容政策，破坏性变更要求新的 major
  版本（见 `docs/API_COMPATIBILITY.md`）。

## CI

`.github/workflows/sdk-release.yml` 在 OpenAPI 或 SDK 文件变化时运行三语言
门禁与契约校验。发布动作由维护者显式执行，不在普通推送中自动发布。
