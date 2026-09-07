# 前端功能模块全局审查

状态：分配范围完成。初轮 98 个功能生产文件均已阅读全文，9 类确定问题已先复现再修复。api.ts 已完整读取作为契约核对依据。根应用由 accounts_upstreams 与 operations 分段审查；共享 UI、构建入口与最终集成由根审查负责。

## 生产文件覆盖

- [x] `frontend/src/features/accounts/components/account-batch-probe-dialog.tsx`
- [x] `frontend/src/features/accounts/components/account-delete-dialog.tsx`
- [x] `frontend/src/features/accounts/components/account-detail-dialog.tsx`
- [x] `frontend/src/features/accounts/components/account-model-sync-dialog.tsx`
- [x] `frontend/src/features/accounts/components/account-operation-buttons.tsx`
- [x] `frontend/src/features/accounts/components/account-operation-controls.tsx`
- [x] `frontend/src/features/accounts/components/account-pool-cells.tsx`
- [x] `frontend/src/features/accounts/components/account-probe-dialog.tsx`
- [x] `frontend/src/features/accounts/components/account-recovery-status.tsx`
- [x] `frontend/src/features/accounts/components/account-settings-panel.tsx`
- [x] `frontend/src/features/accounts/components/account-sort-header.tsx`
- [x] `frontend/src/features/accounts/components/account-status-tabs.tsx`
- [x] `frontend/src/features/accounts/components/base-url-check-results.tsx`
- [x] `frontend/src/features/accounts/components/manual-priority-dialog.tsx`
- [x] `frontend/src/features/accounts/components/platform-probe-dialog.tsx`
- [x] `frontend/src/features/accounts/lib/account-deletion-progress.ts`
- [x] `frontend/src/features/accounts/lib/account-labels.ts`
- [x] `frontend/src/features/accounts/lib/account-pool.ts`
- [x] `frontend/src/features/accounts/lib/account-selection.ts`
- [x] `frontend/src/features/accounts/lib/account-sort.ts`
- [x] `frontend/src/features/accounts/lib/account-state.ts`
- [x] `frontend/src/features/accounts/lib/platform-probe-schema.ts`
- [x] `frontend/src/features/alert-policy/components/alert-policy-page.tsx`
- [x] `frontend/src/features/alert-policy/constants.ts`
- [x] `frontend/src/features/alert-policy/lib/alert-policy-schema.ts`
- [x] `frontend/src/features/alerts/components/alert-list-actions.tsx`
- [x] `frontend/src/features/alerts/components/notification-queue-status.tsx`
- [x] `frontend/src/features/alerts/lib/alert-display.ts`
- [x] `frontend/src/features/config/components/account-creation-policy-form.tsx`
- [x] `frontend/src/features/config/components/account-creation-settings-card.tsx`
- [x] `frontend/src/features/config/components/config-section-tabs.tsx`
- [x] `frontend/src/features/config/components/model-sync-settings-card.tsx`
- [x] `frontend/src/features/config/components/navigation-settings-card.tsx`
- [x] `frontend/src/features/config/components/platform-probe-models-form.tsx`
- [x] `frontend/src/features/config/components/settings-footer.tsx`
- [x] `frontend/src/features/config/constants.ts`
- [x] `frontend/src/features/config/lib/account-creation-settings-schema.ts`
- [x] `frontend/src/features/config/lib/model-sync-settings-schema.ts`
- [x] `frontend/src/features/config/lib/platform-probe-models-schema.ts`
- [x] `frontend/src/features/groups/components/group-allocation-dialog.tsx`
- [x] `frontend/src/features/groups/components/group-policy-editor-fields.tsx`
- [x] `frontend/src/features/logs/components/account-rate-sync-result-table.tsx`
- [x] `frontend/src/features/logs/components/log-details-dialog.tsx`
- [x] `frontend/src/features/logs/components/logs-center-page.tsx`
- [x] `frontend/src/features/logs/lib/log-display.ts`
- [x] `frontend/src/features/model-check/components/model-check-configuration-dialog.tsx`
- [x] `frontend/src/features/model-check/components/model-check-page.tsx`
- [x] `frontend/src/features/model-check/components/model-check-result.tsx`
- [x] `frontend/src/features/model-check/components/model-check-selection.tsx`
- [x] `frontend/src/features/model-check/lib/model-check-configuration-schema.ts`
- [x] `frontend/src/features/model-check/lib/model-check-schema.ts`
- [x] `frontend/src/features/newapi-management/components/channel-configuration-step.tsx`
- [x] `frontend/src/features/newapi-management/components/channel-form.tsx`
- [x] `frontend/src/features/newapi-management/components/channel-model-dialog.tsx`
- [x] `frontend/src/features/newapi-management/components/group-bindings.tsx`
- [x] `frontend/src/features/newapi-management/components/model-prices.tsx`
- [x] `frontend/src/features/newapi-management/components/newapi-management-page.tsx`
- [x] `frontend/src/features/newapi-management/components/platform-dialog.tsx`
- [x] `frontend/src/features/newapi-management/components/price-comparison.tsx`
- [x] `frontend/src/features/newapi-management/components/raw-pricing-source-dialog.tsx`
- [x] `frontend/src/features/newapi-management/constants.ts`
- [x] `frontend/src/features/newapi-management/lib/model-price-adjustment-schema.ts`
- [x] `frontend/src/features/newapi-management/lib/model-price-adjustment.ts`
- [x] `frontend/src/features/newapi-management/lib/pricing-number.ts`
- [x] `frontend/src/features/newapi-management/lib/schemas.ts`
- [x] `frontend/src/features/overview/components/overview-activity.tsx`
- [x] `frontend/src/features/overview/components/overview-page.tsx`
- [x] `frontend/src/features/overview/lib/overview-health.ts`
- [x] `frontend/src/features/pricing/components/pricing-page.tsx`
- [x] `frontend/src/features/pricing/components/revenue-analysis-page.tsx`
- [x] `frontend/src/features/profile/components/profile-page.tsx`
- [x] `frontend/src/features/request-trace/components/system-log-search-panel.tsx`
- [x] `frontend/src/features/request-trace/components/trace-account-actions.tsx`
- [x] `frontend/src/features/system-info/components/system-info-page.tsx`
- [x] `frontend/src/features/system-info/constants.ts`
- [x] `frontend/src/features/traffic-ranking/components/traffic-ranking-page.tsx`
- [x] `frontend/src/features/traffic-ranking/lib/traffic-ranking.ts`
- [x] `frontend/src/features/upstreams/components/onboarding-batch-workspace.tsx`
- [x] `frontend/src/features/upstreams/components/onboarding-candidate-visibility-filter.tsx`
- [x] `frontend/src/features/upstreams/components/onboarding-confirm-dialog.tsx`
- [x] `frontend/src/features/upstreams/components/onboarding-group-binding-select.tsx`
- [x] `frontend/src/features/upstreams/components/onboarding-heading-actions.tsx`
- [x] `frontend/src/features/upstreams/components/onboarding-key-cleanup-dialog.tsx`
- [x] `frontend/src/features/upstreams/components/onboarding-maintenance-actions.tsx`
- [x] `frontend/src/features/upstreams/components/onboarding-probe-action.tsx`
- [x] `frontend/src/features/upstreams/components/upstream-bound-account-select.tsx`
- [x] `frontend/src/features/upstreams/components/upstream-edit-dialog.tsx`
- [x] `frontend/src/features/upstreams/components/upstream-group-binding-audit.tsx`
- [x] `frontend/src/features/upstreams/components/upstream-group-binding-editor.tsx`
- [x] `frontend/src/features/upstreams/components/upstream-group-dialog-header.tsx`
- [x] `frontend/src/features/upstreams/components/upstream-group-history.tsx`
- [x] `frontend/src/features/upstreams/components/upstream-identity.tsx`
- [x] `frontend/src/features/upstreams/components/upstream-recovery-selection-toolbar.tsx`
- [x] `frontend/src/features/upstreams/lib/onboarding-candidate-visibility.ts`
- [x] `frontend/src/features/upstreams/lib/onboarding-requests.ts`
- [x] `frontend/src/features/upstreams/lib/upstream-edit-schema.ts`
- [x] `frontend/src/features/upstreams/lib/upstream-rate-labels.ts`
- [x] `frontend/src/features/vault/components/vault-page.tsx`

## 后续集成补充

根代理已补读并核对随后加入的 `frontend/src/features/newapi-management/components/batch-model-price-dialog.tsx` 及 model-prices/newapi-management-page 接线，覆盖影响预览、忙碌禁用、读回不匹配、失败重试与选择保留；并修正新增测试的浏览器 API 模拟和类型问题。最终所有前端生产文件列于 `global-review-coverage-2026-09-07.md`，全量 196 文件/994 项测试和其余门禁通过。下文局部集数字保留初轮真实进度。

## 发现与修复

1. Header 编辑契约：后端只返回名称后，关闭开关被 React Hook Form 判断为未变更，未提交清空；Header JSON 无可访问名称。独立失败回归证明 headers 为 undefined、输入框无法按名称查询。修复为显式清空状态、初始不回填秘密、留空保留/JSON 替换/关闭清空提示及 label/aria-invalid；2 文件 10 个测试通过，typecheck 和涉及文件 ESLint 通过。

2. 模型同步失败/取消无明细时被当成成功：3 个真实请求路径回归先失败；改为按 Task.status 展示服务端失败/取消原因，完整相关 13 个测试通过。
3. 账号设置首次读取占位值可提交：失败回归先观察到占位阶段保存按钮；改为读取阶段只展示加载状态，相关 10 个测试已通过。
4. 密码箱编辑后重新留空会覆盖原凭据，与页面承诺不符：三种字段的失败回归均复现了空值覆盖；已经修复为仅非空输入或显式清除时提交敏感字段，相关 10 个测试通过；typecheck 和相关 ESLint 通过。

5. 模型检测共同模型列表被固定高度父面板截断，无法滚动选择后面的模型：布局回归先失败，已为列表添加高度约束与内部滚动，相关 9 个测试通过。

6. 价格表达式顺次替换变量两侧系数会将同一乘法重复涨跌，且错误修改阶梯名称：4 个失败回归稳定复现；改为每段乘积仅缩放一个系数并跳过字符串；相关 19 个测试通过。

7. 密码箱索引首次读取期间允许新增，可能绕过同名覆盖确认：失败回归先证明按钮可用，改为索引成功读取前禁止新增和保存；相关验证与日志一起 38 项通过。
8. 日志详情仅刷新 result，状态、摘要和进度仍停在列表旧值：失败回归稳定复现；统一使用最新 Task.status/message/progress，相关验证与密码箱一起 38 项通过。
9. 价格配置“立即调整”未经影响范围确认就批量写入：失败回归先复现没有确认弹窗；增加账号明细、影响数量及二次确认，价格模块 3 文件 28 个测试通过。

## 验证与限制

- 全量功能模块命令 `bun run test src/features`：106 个文件、564 个测试。首次结果为 105 文件通过、1 文件中的 3 个旧加载 fixture 失败；该文件 `settings-scroll.test.tsx` 补充真实已加载配置后，最新 7/7 通过，其余 105 文件未发生本次修正。
- 根全量反馈的 `src/__tests__/runtime-settings-page.test.tsx` 同类旧 fixture 已按根要求补齐；最新 9/9 通过。此次唯一非 features 编辑仅限该测试的账号设置缓存 fixture。
- 新增失败回归：Header 3 场景、模型同步 3 场景、账号设置初载 1 场景、密码箱留空 3 场景及初载 1 场景、模型列表滚动 1 场景、表达式调价 4 场景、日志任务状态 1 场景、批量价格确认 1 场景。明确清除 Headers 路径另补通过回归。
- 最新 `bun run typecheck` 通过；涉及的全部生产文件、新增测试及两个 fixture 文件 ESLint 无 error；18 个功能改动文件 oxfmt 检查通过，根 fixture 单独格式化；`git diff --check` 通过。
- 失败/通过命令原始输出保留在 `/tmp`：`upstream-headers-*`、`model-sync-*`、`settings-initial-*`、`vault-preservation-*`、`model-list-scrolling-*`、`expression-adjustment-*`、`log-task-failing.log`、`task-vault-*`、`pricing-confirmation-*`、`frontend-features-all-tests.log`、`settings-scroll-passing.log`、`runtime-settings-passing.log`、`features-final-*`。
- 所有网络仅 mock Fetch 边界或读取显式查询缓存；未连接真实服务、生产数据库、密码箱或通知渠道。密码箱的 jsdom 交互用标准 fireEvent，避免该环境 userEvent 聚焦时的循环；真实浏览器交互未由本分工单独覆盖，最终全仓/浏览器检查由根审查汇总。
- 保留所有并行和既有改动；未提交。
