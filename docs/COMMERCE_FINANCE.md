# Commerce 双账务域与财务闭环

## 双账务域

- Provider Commerce：Provider 对终端 Customer 收费；Provider 是 Merchant of Record。
- Platform Commerce：平台对 Provider 收费；记录只存在于 operator 域。
- 两个账务域使用不同账户、目录、发票、支付连接、审计与对账流程；Provider 域 RLS 永远看不到 Platform 记录。

## 财务不变量

- 已发布目录版本不可变，订阅固定绑定 `catalog_version_id`。
- 每张发票行可重放：`metric_id` / `price_id` / `event_transaction_id` 指向不可变输入。
- finalized / voided 发票财务字段与行不可变；只允许更新 `payment_status` 等非财务字段。
- PSP 支付状态重复/乱序更新不得创建重复发票或重复支付行。
- 已出账用量不能直接冲销；必须走 credit note（`usage_already_invoiced` 语义）。

## 对账

- 每小时 reconciliation 覆盖：invoice amount consistency、invoice line total match、unpaid finalized overdue。
- 对账漂移通过 `reconciliation_drift{check}` 暴露；差异处置见 `docs/RUNBOOK.md`。

## 契约门禁

- `TestCommerceDomainIsolation`：Provider 域看不到 Platform Commerce 记录。
- `TestInvoiceFinalizedImmutability`：finalized/voided 发票与行不可变。
- `TestInvoiceLineTraceabilityContract`：发票行保留 metric/price/catalog version/usage transaction。
- `TestDuplicatePaymentStatusNoDuplicateInvoice`：重复支付状态不复制发票。
- `TestUsagePostInvoiceReversal`：已出账用量拒绝直接冲销并要求 credit note。
