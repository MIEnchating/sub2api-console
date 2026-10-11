# 已收口模块：New API 管理前端（2026-10-11）

本模块本轮范围已收口，见 [渠道读取与编辑状态](../newapi-frontend-2026-10-11.md)。下一模块为上游前端；日志冻结。

进入模块时 124 文件中 67 项哈希匹配，57 项待补审（31 项无记录、26 项旧记录失效）。下表保留原始队列状态用于追溯，现均已完整阅读并登记；另新增两个回归文件，当前 126/126 匹配。集中完成渠道读取、任务组草稿和保存状态修复；68 文件 408 项单元/请求契约及八个浏览器场景通过。

历史全局报告的第 222、223、226 轮包含本模块检查命令摘要和文件哈希，须核对传递依赖、配置、fixture 和实际输出后决定复用，不能仅凭轮数宣称当前通过；不重复修复已收口的读取生命周期和分页问题。

| 状态    | 文件                                                                                                   |
| ------- | ------------------------------------------------------------------------------------------------------ |
| changed | `frontend/src/features/newapi-management/components/__tests__/automation-channel-pagination.test.tsx`  |
| changed | `frontend/src/features/newapi-management/components/__tests__/automation-channel-scope.test.tsx`       |
| changed | `frontend/src/features/newapi-management/components/__tests__/automation-dialog-cancellation.test.tsx` |
| pending | `frontend/src/features/newapi-management/components/__tests__/automation-flow.test.tsx`                |
| pending | `frontend/src/features/newapi-management/components/__tests__/automation-optional-channels.test.tsx`   |
| changed | `frontend/src/features/newapi-management/components/__tests__/automation-page-loading.test.tsx`        |
| changed | `frontend/src/features/newapi-management/components/__tests__/automation-task-groups.test.tsx`         |
| pending | `frontend/src/features/newapi-management/components/__tests__/channel-batch-selection.test.tsx`        |
| changed | `frontend/src/features/newapi-management/components/__tests__/channel-form.test.tsx`                   |
| pending | `frontend/src/features/newapi-management/components/__tests__/channel-group-members.test.tsx`          |
| pending | `frontend/src/features/newapi-management/components/__tests__/channel-group-order.test.tsx`            |
| pending | `frontend/src/features/newapi-management/components/__tests__/channel-groups.test.tsx`                 |
| pending | `frontend/src/features/newapi-management/components/__tests__/channel-layout.test.tsx`                 |
| pending | `frontend/src/features/newapi-management/components/__tests__/channel-maintenance.test.tsx`            |
| pending | `frontend/src/features/newapi-management/components/__tests__/channel-model-discovery.test.tsx`        |
| pending | `frontend/src/features/newapi-management/components/__tests__/channel-page-layout.test.tsx`            |
| changed | `frontend/src/features/newapi-management/components/__tests__/channel-write-pending.test.tsx`          |
| changed | `frontend/src/features/newapi-management/components/__tests__/comparison-special-models.test.tsx`      |
| pending | `frontend/src/features/newapi-management/components/__tests__/default-price-tiers.test.tsx`            |
| changed | `frontend/src/features/newapi-management/components/__tests__/loading.test.tsx`                        |
| changed | `frontend/src/features/newapi-management/components/__tests__/management-workflows.test.tsx`           |
| changed | `frontend/src/features/newapi-management/components/__tests__/model-price-actions.test.tsx`            |
| pending | `frontend/src/features/newapi-management/components/__tests__/official-image-prices.test.tsx`          |
| pending | `frontend/src/features/newapi-management/components/__tests__/partial-reference-prices.test.tsx`       |
| pending | `frontend/src/features/newapi-management/components/__tests__/platform-required-fields.test.tsx`       |
| pending | `frontend/src/features/newapi-management/components/__tests__/price-comparison-detail.test.tsx`        |
| pending | `frontend/src/features/newapi-management/components/__tests__/render-with-router.tsx`                  |
| pending | `frontend/src/features/newapi-management/components/__tests__/schedule-save-lifecycle.test.tsx`        |
| changed | `frontend/src/features/newapi-management/components/__tests__/tier-expression-comparison.test.ts`      |
| changed | `frontend/src/features/newapi-management/components/__tests__/time-pricing.test.tsx`                   |
| changed | `frontend/src/features/newapi-management/components/account-model-automation-dialog.tsx`               |
| changed | `frontend/src/features/newapi-management/components/account-model-automation-page.tsx`                 |
| pending | `frontend/src/features/newapi-management/components/account-model-automation-task-group-dialog.tsx`    |
| changed | `frontend/src/features/newapi-management/components/automation-editor-loader.tsx`                      |
| changed | `frontend/src/features/newapi-management/components/channel-configuration-step.tsx`                    |
| pending | `frontend/src/features/newapi-management/components/channel-create-dialog.tsx`                         |
| pending | `frontend/src/features/newapi-management/components/channel-credentials-step.tsx`                      |
| pending | `frontend/src/features/newapi-management/components/channel-form-layout.tsx`                           |
| changed | `frontend/src/features/newapi-management/components/channel-form.tsx`                                  |
| pending | `frontend/src/features/newapi-management/components/channel-group-card.tsx`                            |
| pending | `frontend/src/features/newapi-management/components/channel-impact-preview.tsx`                        |
| pending | `frontend/src/features/newapi-management/components/channel-model-selection.tsx`                       |
| pending | `frontend/src/features/newapi-management/components/channel-steps.tsx`                                 |
| changed | `frontend/src/features/newapi-management/components/model-prices.tsx`                                  |
| changed | `frontend/src/features/newapi-management/components/newapi-form-skeleton.tsx`                          |
| changed | `frontend/src/features/newapi-management/components/newapi-management-page.tsx`                        |
| changed | `frontend/src/features/newapi-management/components/official-tier-values.tsx`                          |
| changed | `frontend/src/features/newapi-management/components/platform-dialog.tsx`                               |
| changed | `frontend/src/features/newapi-management/constants.ts`                                                 |
| pending | `frontend/src/features/newapi-management/lib/__tests__/billing-expression-comparison.test.ts`          |
| changed | `frontend/src/features/newapi-management/lib/automation-schedule.ts`                                   |
| pending | `frontend/src/features/newapi-management/lib/automation-task-group.ts`                                 |
| pending | `frontend/src/features/newapi-management/lib/billing-expression-comparison.ts`                         |
| pending | `frontend/src/features/newapi-management/lib/channel-group-schema.ts`                                  |
| pending | `frontend/src/features/newapi-management/lib/channel-maintenance-schema.ts`                            |
| pending | `frontend/src/features/newapi-management/lib/channel-model-change.ts`                                  |
| changed | `frontend/src/features/newapi-management/lib/time-pricing.ts`                                          |
