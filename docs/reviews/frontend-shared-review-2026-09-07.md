# 前端共享模块审查

状态：全部 lib、hooks、components 生产文件人工审查完成；根应用全部分段也已完成，最终统一验证见全局审查记录。

## 库与 Hooks

- [x] `frontend/src/lib/browser-preferences.ts`
- [x] `frontend/src/lib/captcha-challenge.ts`
- [x] `frontend/src/lib/domain-dictionaries.ts`
- [x] `frontend/src/lib/group-policy-display.ts`
- [x] `frontend/src/lib/json-string-map.ts`
- [x] `frontend/src/lib/navigation-preferences.ts`
- [x] `frontend/src/lib/onboarding-entry.ts`
- [x] `frontend/src/lib/operation-feedback.ts`
- [x] `frontend/src/lib/query-client.ts`
- [x] `frontend/src/lib/scheduling-display.ts`
- [x] `frontend/src/lib/scheduling-strategy.ts`
- [x] `frontend/src/lib/sensitive-field.ts`
- [x] `frontend/src/lib/session-auth.ts`
- [x] `frontend/src/lib/task-refresh.ts`
- [x] `frontend/src/lib/task-result.ts`
- [x] `frontend/src/lib/task-state.ts`
- [x] `frontend/src/lib/utils.ts`
- [x] `frontend/src/lib/vault-entry-label.ts`
- [x] `frontend/src/hooks/use-client-pagination.ts`
- [x] `frontend/src/hooks/use-mobile.tsx`

## 共享组件

- [x] `frontend/src/components/account-health-score.tsx`
- [x] `frontend/src/components/account-recent-results.tsx`
- [x] `frontend/src/components/aria-date-primitives.tsx`
- [x] `frontend/src/components/confirm-action-dialog.tsx`
- [x] `frontend/src/components/data-table/empty-state.tsx`
- [x] `frontend/src/components/data-table/filter-menu.tsx`
- [x] `frontend/src/components/data-table/filter-toolbar.tsx`
- [x] `frontend/src/components/data-table/number-range-filter.tsx`
- [x] `frontend/src/components/data-table/pagination.tsx`
- [x] `frontend/src/components/data-table/search-field.tsx`
- [x] `frontend/src/components/data-table/table-action-button.tsx`
- [x] `frontend/src/components/data-table/table-panel.tsx`
- [x] `frontend/src/components/date-picker.tsx`
- [x] `frontend/src/components/date-time-picker-utils.ts`
- [x] `frontend/src/components/field-help-tooltip.tsx`
- [x] `frontend/src/components/multi-select.tsx`
- [x] `frontend/src/components/onboarding-selection-skeleton.tsx`
- [x] `frontend/src/components/page-actions.tsx`
- [x] `frontend/src/components/page-heading.tsx`
- [x] `frontend/src/components/page-layout.tsx`
- [x] `frontend/src/components/query-error-toast.tsx`
- [x] `frontend/src/components/refresh-button.tsx`
- [x] `frontend/src/components/result-summary-row.tsx`
- [x] `frontend/src/components/status-badge.tsx`
- [x] `frontend/src/components/task-startup-state.tsx`
- [x] `frontend/src/components/ui/badge.tsx`
- [x] `frontend/src/components/ui/button.tsx`
- [x] `frontend/src/components/ui/card.tsx`
- [x] `frontend/src/components/ui/checkbox.tsx`
- [x] `frontend/src/components/ui/combobox.tsx`
- [x] `frontend/src/components/ui/dialog.tsx`
- [x] `frontend/src/components/ui/dropdown-menu.tsx`
- [x] `frontend/src/components/ui/dropdown-search-focus.ts`
- [x] `frontend/src/components/ui/input.tsx`
- [x] `frontend/src/components/ui/progress.tsx`
- [x] `frontend/src/components/ui/segmented-control.tsx`
- [x] `frontend/src/components/ui/select.tsx`
- [x] `frontend/src/components/ui/sheet.tsx`
- [x] `frontend/src/components/ui/sidebar.tsx`
- [x] `frontend/src/components/ui/skeleton.tsx`
- [x] `frontend/src/components/ui/sonner.tsx`
- [x] `frontend/src/components/ui/switch.tsx`
- [x] `frontend/src/components/ui/table-overflow-tooltip.tsx`
- [x] `frontend/src/components/ui/table.tsx`
- [x] `frontend/src/components/ui/textarea.tsx`
- [x] `frontend/src/components/ui/tooltip.tsx`

## 根应用与构建边界

根已读 App.tsx 550–790（初始化、会话失效清理、主题导航、根 shell）、main.tsx、router.tsx、app-routes/__root.tsx、playwright.config.ts、rsbuild.config.ts、e2e/__tests__/session-cache.e2e.ts 及新 storage-unavailable.e2e.ts。App 其他分段、全部路由和样式的阅读随后完成，详见前端全局与 App 分段报告。

## 修复

1. 受限浏览器 localStorage 访问/写入异常导致整个登录页进入错误边界。实际浏览器先失败，增加 browser-preferences 包装并接入 App，桌面/移动 4 项 E2E 通过，涉及 lint 通过。
2. FilterMenu 选项异步缩短后 ArrowDown/Enter 仍按旧活动索引处理，选中错误分组。新失败交互用例后以当前有效索引移动，3 个相关文件 18 个测试通过，涉及 lint 和全前端 typecheck 通过。
3. MultiSelect 已打开时变为 disabled，底部清空按钮仍可清除所选值。新模块 disabled.test.tsx 先失败，清空按钮纳入禁用状态，3 个相关文件 18 个测试通过，涉及 lint 和全前端 typecheck 通过。

功能代理的密码箱测试类型错误已修复，最新 `bun run typecheck` 通过。最终全量前端测试和生产构建结果集中记录在全局审查报告。

4. **精确 Host 凭据被 www 别名抢占**：同时存在根域与 www 显式绑定时，原先仅按去掉 www 后的结果匹配，默认选择和排序受原始列表顺序影响。新增两项测试先失败；现在精确规范化 Host 优先、原有 www 别名仅作为后备。4 文件 26 个测试通过，覆盖真实鉴权表单和渠道表单调用方。日志 `/tmp/sub2api-vault-host-before.log` 与 `/tmp/sub2api-vault-host-after.log`。


5. **带尾斜线的路由导航错误**：`/newapi/`、`/accounts/`、`/newapi/groups/` 回退为概览，导致标题和导航错位；三项失败回归后规范化路径尾斜线。
6. **上游批量任务隐藏失败/取消**：批量鉴权无明细失败仅显示零计数，取消后也无状态；上游同步取消且无明细被显示为无目标。三项失败回归后增加明确终态与原因，保留已执行明细。与导航、既有同步/鉴权回归合计 4 文件 24 测试通过。原 App 8254–9095 所有函数已由根完整阅读；没有遗漏的归属空档。
7. **浅色账号状态文字对比度不足**：真实浏览器测得“健康”3.735、“降级”2.927，低于正文 4.5；新增按实际祖先背景合成的回归先失败。调整浅色 success/warning 语义色及 warning 前景色，账号列表全部 8 个桌面/移动浏览器回归通过，日志 `/tmp/sub2api-status-contrast-after.log`。
