# 按钮与状态样式验证

验证对象为当前共享工作区，包含其他尚未提交的业务改动，不代表某个提交的完整 CI 结果。

- 最终 `bun run typecheck` 通过。
- 本次涉及的 42 个代码与测试文件 ESLint 通过，格式检查通过；`git diff --check` 通过。
- 共享按钮、标签、表格操作与动画工具栏定向回归：5 个文件、27 项通过；最终渠道操作回归：8 项通过。
- 工作台状态颜色回归 6 项通过；前置检测结果回归 16 项通过；流量格式与状态回归 12 项通过。
- 浏览器复用 3004 开发服务，所有 API 使用隔离 fixture；按钮尺寸、自定义接口布局与辅助操作颜色、主题对比度共 14 项通过，覆盖浅色桌面和深色窄屏。临时 Playwright 配置已清理。
- 完整前端测试执行期间工作区仍有改动：673 个文件中 657 通过、16 失败；3525 项中 3508 通过、17 失败，另有 1 项未捕获错误。不能宣称全量通过。
- 16 个失败文件按当前代码复查，9 个文件通过。渠道操作旧样式断言已更新，最终 8 项通过。余下 6 个文件仍失败，保留原测试和业务代码：
  - `account-settings-panel.test.tsx` 与 `onboarding-probe-models.test.tsx`：找不到旧的“继承的探活配置” note。
  - `settings-refresh-scope.test.tsx`：刷新接口范围仍期待 `/api/groups`。
  - `pool-mode-request-lifecycle.test.tsx`：池模式任务读取生命周期断言失败。
  - `detection-request-lifecycle.test.tsx`：共享观察者存在时取消请求的断言失败。
  - `schedule-save-lifecycle.test.tsx`：分组 fixture 形状不匹配，触发 `.filter is not a function`，保存断言失败。

本次没有调整超时门槛、删除失败用例或修改上述业务流程。
