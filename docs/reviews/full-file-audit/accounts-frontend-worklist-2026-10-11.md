# 已收口模块：账号管理前端（2026-10-11）

上游管理前端已收口，日志保持冻结。继续逐模块汇总发现、集中修复并验证；已匹配的记录和有效检查按依赖复用。

入口 125 文件中 82 项匹配、43 项需补审；本轮全部缺口已完整阅读，新增四文件后模块为 **129/129** 当前记录。模型选择计算改用分组索引，数字设置字段关联错误与无效状态；76 文件 464 项最终有效回归、6 项浏览器场景、类型和涉及文件 lint/格式通过。见 [报告](../accounts-frontend-2026-10-11.md) 与 [台账](accounts-frontend-2026-10-11.tsv)。下一模块为 [官方价格后端](officialpricing-worklist-2026-10-11.md)。

下表保留入口时的缺口，不代表当前仍待审。

| 入口状态 | 文件 |
| --- | --- |
| pending | `frontend/src/features/accounts/__tests__/maintenance-scope.test.tsx` |
| pending | `frontend/src/features/accounts/__tests__/manual-priority-refresh.test.tsx` |
| pending | `frontend/src/features/accounts/__tests__/status-filter-scope.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/account-batch-delete.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/account-batch-probe.test.tsx` |
| pending | `frontend/src/features/accounts/components/__tests__/account-cost-wall-switch.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/account-detail-dialog.test.tsx` |
| pending | `frontend/src/features/accounts/components/__tests__/account-identity-layout.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/account-model-sync-dialog.test.tsx` |
| pending | `frontend/src/features/accounts/components/__tests__/account-name-tooltip.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/account-operation-buttons.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/account-pool-cells.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/account-probe-dialog.test.tsx` |
| pending | `frontend/src/features/accounts/components/__tests__/account-quality.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/account-settings-panel.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/account-status-tabs.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/groups-refresh.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/manual-priority-dialog.test.ts` |
| changed | `frontend/src/features/accounts/components/__tests__/manual-priority-scheduling.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/model-sync-selection.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/platform-probe-dialog.test.tsx` |
| pending | `frontend/src/features/accounts/components/__tests__/probe-actions.test.tsx` |
| pending | `frontend/src/features/accounts/components/__tests__/probe-animation.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/probe-layout.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/probe-process.test.tsx` |
| changed | `frontend/src/features/accounts/components/__tests__/settings-lifecycle.test.tsx` |
| pending | `frontend/src/features/accounts/components/__tests__/status-dictionary-fixture.ts` |
| pending | `frontend/src/features/accounts/components/__tests__/status-filter-dictionary.test.tsx` |
| pending | `frontend/src/features/accounts/components/__tests__/status-filter-options.test.tsx` |
| pending | `frontend/src/features/accounts/components/__tests__/table-loading.test.tsx` |
| pending | `frontend/src/features/accounts/components/account-cost-wall-switch.tsx` |
| changed | `frontend/src/features/accounts/components/account-model-sync-dialog.tsx` |
| changed | `frontend/src/features/accounts/components/account-operation-controls.tsx` |
| changed | `frontend/src/features/accounts/components/account-pool-cells.tsx` |
| changed | `frontend/src/features/accounts/components/account-probe-dialog.tsx` |
| pending | `frontend/src/features/accounts/components/account-quality-cell.tsx` |
| changed | `frontend/src/features/accounts/components/account-settings-panel.tsx` |
| changed | `frontend/src/features/accounts/components/account-status-tabs.tsx` |
| changed | `frontend/src/features/accounts/components/probe-task-timeline.tsx` |
| pending | `frontend/src/features/accounts/constants.ts` |
| pending | `frontend/src/features/accounts/hooks/__tests__/probe-history.test.tsx` |
| changed | `frontend/src/features/accounts/lib/__tests__/account-pool.test.ts` |
| changed | `frontend/src/features/accounts/lib/account-pool.ts` |
