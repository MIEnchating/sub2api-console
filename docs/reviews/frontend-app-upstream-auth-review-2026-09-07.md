# App 上游鉴权专项审查

日期：2026-09-07。父任务授权范围：`frontend/src/App.tsx` 的上游单项任务弹窗及 `ManualAuthForm` / `ManualAuthHeadersEditor`，新增测试放在 `frontend/src/components/__tests__/`。已保留其他审查任务的并发修改，未整体格式化 App。

## 审查与修复

已阅读全文核对上述表单、Headers 编辑器，以及 UpstreamsPage 的单项任务状态、启动 mutation、轮询 query、菜单入口和弹窗关闭逻辑；辅助检查了 FormField、Select、Dialog、task-state 和对应 API 契约。

| 问题 | 用户影响 | 修复 |
| --- | --- | --- |
| 鉴权字段只有可见文本，没有标签与控件关联 | 辅助技术无法可靠识别鉴权方式、Token、Admin Key、User ID、用户名、密码、密码箱项等字段 | 为表单和 Headers 编辑器分配 `React.useId()`，用 `htmlFor` / `id` 关联各模式下的可见标签与输入控件 |
| Headers 解析错误没有反映到文本框的可访问状态 | 错误出现后，文本框既未标记无效，也未关联错误原因 | 添加 `aria-invalid` 与 `aria-describedby`；错误说明继续使用 `role="alert"`，重新输入后清除无效状态和描述关联 |
| 单项鉴权恢复、余额同步运行时，Escape 仍会关闭弹窗并清空 task ID | 隐藏关闭按钮未阻止键盘关闭；后台任务继续运行，但页面轮询及终态反馈中断 | 在 `onOpenChange` 同时检查启动 mutation 与非终态任务状态，完成后恢复正常关闭 |

生产改动仅涉及 App 中上述两个窄范围。没有修改后端、共享 FormField、API 契约或其他 feature。

## 先失败后修复

- `/tmp/manual-auth-accessibility-failing.log`：默认 Sub2API / New API / 自定义平台凭据标签，以及 Headers 文本框名称，4 个回归用例在修复前失败。
- `/tmp/manual-auth-error-failing.log`：实际提交无效 JSON 后，断言文本框 `aria-invalid="true"` 失败，实际属性缺失。
- `/tmp/upstream-action-startup-failing.log`：余额同步任务创建中按 Escape 后，预期仍存在的任务弹窗消失；失败发生在关闭后的可见状态断言。

最终新增回归包含：三类平台的初始凭据标签、模式选择器名称及键盘打开、密码箱模式、手动账号密码及保存名称、Headers 名称和错误清除，以及鉴权恢复 / 余额同步在创建中和执行中按 Escape 保留跟踪、轮询得到完成状态后允许关闭。

## 最新验证

2026-09-07 15:21 UTC 启动的受影响测试全部通过：4 个文件，22 个用例。

```text
bun run test src/components/__tests__/manual-auth-form.test.tsx \
  src/components/__tests__/manual-auth-accessibility.test.tsx \
  src/components/__tests__/upstream-action-dialog.test.tsx \
  src/components/__tests__/upstream-task-terminal-state.test.tsx \
  --maxWorkers=2 --minWorkers=1
```

- 测试日志：`/tmp/upstream-auth-affected-tests.log`。
- `bun run typecheck` 通过：`/tmp/upstream-auth-typecheck.log`。
- App 与两个新增测试文件的 ESLint 通过：`/tmp/upstream-auth-lint.log`。
- 两个新增测试文件的 `oxfmt --check` 通过；涉及文件的 `git diff --check` 通过。

测试使用真实组件、Router、React Query 和 API 包装，网络仅在 Fetch 边界返回隔离 fixture，不连接真实端点。为避开 JSDOM 26 / NWSAPI 对顶层伪类和隐藏原生 select 的递归，测试采用浏览器 API 兼容处理；选择器包装在模块加载前安装，文件结束时恢复，其他 mock 与 QueryClient 在每例后清理。该兼容处理不改变业务逻辑。这里的验证范围为上述专项，完整前端与浏览器检查由根审查任务统一汇总。
