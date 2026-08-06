# 正式商用上线验收清单

## 自动门禁

一条命令执行全部功能/契约层验收：

```bash
make release-gate
```

覆盖：

1. API build + vet
2. API 非集成全量单测
3. API 全量集成测试
4. `make contract`（OpenAPI 234 / AsyncAPI 74 / 错误码 59 / 类型同步）
5. 官方 Go SDK 测试
6. Web tsc + eslint + 全量 Playwright E2E

## P0 代码侧验收

| 门禁 | 证据 |
|---|---|
| 验收基线 / 黑盒契约 harness | `TestCommercialContractAcceptance` + `docs/CONTRACT_ACCEPTANCE.md` |
| OpenAPI 契约完整 | `make contract`，路由覆盖 234/234 |
| AsyncAPI 事件契约完整 | `make contract`，事件覆盖 74/74 |
| API 版本兼容生命周期 | `ValidateDeprecationRegistry` + `api-version` |
| 公共错误契约 | 59 错误码目录 + `request_id` / `retry_after` |
| Webhook/事件流契约 | `docs/WEBHOOK_EVENTS.md` + 无丢失无重复测试 |
| 身份边界 | 同邮箱隔离 / tenant override / B2B-B2C |
| 双账务域与财务闭环 | Commerce 隔离 + 发票溯源 + 支付状态幂等 |
| Catalog/权益/额度契约 | 目录不可变 + 快照单一真相源 + Quota 并发 |
| Provider 生命周期/Offboarding | Offboarding 端到端 + 写阻断 / 读保留 |
| JIT/审计/WORM | 审计 request_id + 哈希链 + WORM 幂等 |
| 迁移/故障恢复 | Migration/Failover 契约测试 |

## P1/P2 外部证据

| 证据 | 责任方 | 状态 |
|---|---|---|
| PITR 与同地域恢复演练记录 | 运营/基础设施 | 待补充 |
| K8s PDB / 反亲和 / NetworkPolicy / Canary 清单 | 基础设施 | 待补充 |
| 2x/3x 压测与 Chaos 测试 | SRE | 待补充 |
| SOC2 / ISO27001 / GDPR / PCI 证据 | 合规 | 待补充 |
| AGPL/第三方许可证评审 | 法务 | 待补充 |
| P0/P1 On-call 演练记录 | 运营 | 待补充 |

## 签署

| 角色 | 结论 | 日期 | 签名 |
|---|---|---|---|
| 工程负责人 | | | |
| 安全/合规负责人 | | | |
| 上线审批人 | | | |
