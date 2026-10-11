# 已收口模块：上游前端（2026-10-11）

New API 管理前端本轮范围已收口，日志保持冻结。按模块顺序推进上游前端，先完整汇总发现，再集中修复与受影响验证；不把路径枚举当作人工审查。

入口 120 文件中 63 项匹配、57 项需补审；本轮已完成全部缺口阅读并新增刷新基准，模块现为 **121/121** 当前哈希记录。使用统计改为只格式化当前页并支持键盘查看完整金额和账号名；79 文件 337 项单元回归、4 项浏览器场景、类型、涉及文件 lint 和格式检查均通过。详见 [报告](../upstreams-frontend-2026-10-11.md) 与 [台账](upstreams-frontend-2026-10-11.tsv)。下一模块为 [账号管理前端](accounts-frontend-worklist-2026-10-11.md)。

下表保留入口时的缺口状态，不代表当前仍待审。

| 状态    | 文件                                                                                            |
| ------- | ----------------------------------------------------------------------------------------------- |
| changed | `frontend/src/features/upstreams/__tests__/automatic-upstream-info.test.tsx`                    |
| pending | `frontend/src/features/upstreams/__tests__/concurrency-refresh.test.tsx`                        |
| pending | `frontend/src/features/upstreams/__tests__/history-return.test.tsx`                             |
| changed | `frontend/src/features/upstreams/__tests__/onboarding-accessibility.test.tsx`                   |
| changed | `frontend/src/features/upstreams/__tests__/onboarding-composite-fallback.test.tsx`              |
| pending | `frontend/src/features/upstreams/__tests__/onboarding-concurrency.test.tsx`                     |
| changed | `frontend/src/features/upstreams/__tests__/onboarding-fixture.tsx`                              |
| changed | `frontend/src/features/upstreams/__tests__/onboarding-group-entry.test.tsx`                     |
| pending | `frontend/src/features/upstreams/__tests__/onboarding-model-mapping.test.tsx`                   |
| pending | `frontend/src/features/upstreams/__tests__/onboarding-preparation-loading.test.tsx`             |
| pending | `frontend/src/features/upstreams/__tests__/onboarding-refresh-selection.test.tsx`               |
| pending | `frontend/src/features/upstreams/__tests__/onboarding-retry.test.tsx`                           |
| pending | `frontend/src/features/upstreams/__tests__/onboarding-search.test.tsx`                          |
| changed | `frontend/src/features/upstreams/__tests__/task-dialog-lifecycle.test.tsx`                      |
| pending | `frontend/src/features/upstreams/components/__tests__/balance-display.test.tsx`                 |
| pending | `frontend/src/features/upstreams/components/__tests__/group-history-actions.test.tsx`           |
| pending | `frontend/src/features/upstreams/components/__tests__/group-history-binding.test.tsx`           |
| changed | `frontend/src/features/upstreams/components/__tests__/group-history-overview.test.tsx`          |
| pending | `frontend/src/features/upstreams/components/__tests__/group-history-rates.test.tsx`             |
| pending | `frontend/src/features/upstreams/components/__tests__/group-order.test.tsx`                     |
| changed | `frontend/src/features/upstreams/components/__tests__/layout.test.tsx`                          |
| pending | `frontend/src/features/upstreams/components/__tests__/model-mapping-selection.test.tsx`         |
| pending | `frontend/src/features/upstreams/components/__tests__/model-options-loading.test.tsx`           |
| pending | `frontend/src/features/upstreams/components/__tests__/onboarding-batch-bar.test.tsx`            |
| changed | `frontend/src/features/upstreams/components/__tests__/onboarding-batch-workspace.test.tsx`      |
| changed | `frontend/src/features/upstreams/components/__tests__/onboarding-confirm-dialog.test.tsx`       |
| changed | `frontend/src/features/upstreams/components/__tests__/onboarding-group-binding-select.test.tsx` |
| pending | `frontend/src/features/upstreams/components/__tests__/onboarding-model-mapping.test.tsx`        |
| pending | `frontend/src/features/upstreams/components/__tests__/onboarding-probe-models.test.tsx`         |
| changed | `frontend/src/features/upstreams/components/__tests__/upstream-account-actions.test.tsx`        |
| pending | `frontend/src/features/upstreams/components/__tests__/upstream-concurrency.test.tsx`            |
| changed | `frontend/src/features/upstreams/components/__tests__/upstream-group-binding-editor.test.tsx`   |
| pending | `frontend/src/features/upstreams/components/__tests__/upstream-usage-summary.test.tsx`          |
| pending | `frontend/src/features/upstreams/components/onboarding-account-model-fields.tsx`                |
| changed | `frontend/src/features/upstreams/components/onboarding-batch-workspace.tsx`                     |
| changed | `frontend/src/features/upstreams/components/onboarding-confirm-dialog.tsx`                      |
| changed | `frontend/src/features/upstreams/components/onboarding-group-binding-select.tsx`                |
| changed | `frontend/src/features/upstreams/components/onboarding-heading-actions.tsx`                     |
| pending | `frontend/src/features/upstreams/components/onboarding-model-mapping-fields.tsx`                |
| pending | `frontend/src/features/upstreams/components/onboarding-probe-model-field.tsx`                   |
| pending | `frontend/src/features/upstreams/components/upstream-concurrency.tsx`                           |
| pending | `frontend/src/features/upstreams/components/upstream-group-history-overview.tsx`                |
| changed | `frontend/src/features/upstreams/components/upstream-usage-summary.tsx`                         |
| pending | `frontend/src/features/upstreams/constants.ts`                                                  |
| pending | `frontend/src/features/upstreams/hooks/__tests__/model-options.test.tsx`                        |
| pending | `frontend/src/features/upstreams/hooks/__tests__/preparation.test.tsx`                          |
| pending | `frontend/src/features/upstreams/hooks/use-onboarding-preparation.ts`                           |
| pending | `frontend/src/features/upstreams/lib/__tests__/candidate-search.test.ts`                        |
| pending | `frontend/src/features/upstreams/lib/__tests__/onboarding-model-mapping.test.ts`                |
| pending | `frontend/src/features/upstreams/lib/__tests__/onboarding-probe-models.test.ts`                 |
| changed | `frontend/src/features/upstreams/lib/__tests__/upstream-edit-schema.test.ts`                    |
| pending | `frontend/src/features/upstreams/lib/__tests__/usage-totals.test.ts`                            |
| pending | `frontend/src/features/upstreams/lib/history-navigation.ts`                                     |
| pending | `frontend/src/features/upstreams/lib/onboarding-candidate-search.ts`                            |
| pending | `frontend/src/features/upstreams/lib/onboarding-model-mapping.ts`                               |
| pending | `frontend/src/features/upstreams/lib/onboarding-probe-models.ts`                                |
| pending | `frontend/src/features/upstreams/lib/usage-totals.ts`                                           |
