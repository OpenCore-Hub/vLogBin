# Migration / 故障恢复功能契约

## Migration

- 生命周期：draft → validated → importing → completed / rolled_back。
- dry-run 校验：无效 customer / subscription / balance 记录在 `invalid-records` 可查。
- cutover lock：start 后禁止新订阅/新用量写入；complete / rollback 后释放。
- 中断后 resume：重复记录按唯一约束跳过，不产生重复 identity/subscription。
- rollback：标记 records rolled_back 并释放 cutover lock，不会留下双活跃计费源。

## Failover

- 顺序：initiated → fenced → switched → completed / aborted。
- write fencing：fence 后源 Cell 拒绝写入；switch 前 abort 可回滚。
- complete：自动重放未确认 Usage 与 Outbox。
- 同 Provider 只允许一个 active failover。
- 跨 Region failover 被拒绝。

## 契约门禁

- `TestMigrationDryRunValidation`、`TestMigrationImportAndComplete`、`TestMigrationCutoverLock`、`TestMigrationRollback`、`TestMigrationDuplicateRecordsSkipped`。
- `TestFailoverFullLifecycle`、`TestFailoverAbort`、`TestFailoverDuplicateActive`、`TestFailoverCrossRegionRejected`。
- `TestFailoverFence`（fence 后拒绝写入）与 `TestFailoverSwitch` 在 failover 测试集中覆盖。

## SPEC 映射

- #28 dry-run 无效记录 → `TestMigrationDryRunValidation`
- #29 中断 resume 无重复 → `TestMigrationDuplicateRecordsSkipped`
- #30 cutover rollback 无双活跃 → `TestMigrationRollback` + `TestMigrationCutoverLock`
- #36 fence 后 switch → `TestFailoverFullLifecycle`
- #37 未确认 Usage/Outbox 重放 → `TestFailoverFullLifecycle`（replayed_usage / replayed_outbox）
