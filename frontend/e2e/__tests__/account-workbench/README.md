# 默认 Web 工作台回归范围

页面按原工具默认 Web 的四页组织，依据见 `docs/reviews/account-workbench-web-scope-2026-09-15.md`。

- `mixed-run` 覆盖 JSON、RT、邮箱资料同批输入，预览确认、官方浏览器接管、页签切换、统一导入和独立导出。
- `mixed-queue` 覆盖从任务详情按稳定父任务及范围恢复、刷新后重新连接、确认结束。
- `templates` 覆盖线上配置只读摘要、精确倍率、来源版本、保存并使用。
- `history`、`retry` 覆盖批次筛选、结果明细和失败项重试。
- `sms-attachment`、`sms-receipts` 覆盖当前授权费用确认及任务关联订单核对。
- `file-upload`、`layout`、`local-export` 和维护用例覆盖键盘、窄屏、独立初始化、维护委托及待上传状态。

原独立安全设置、资料库、全局清理、RT 再生、线上备份及旧授权控制台不再提供页面入口，因此删除对应入口的 E2E。其业务组件与后端测试仍保留；登录字段、代理、短信和导出行为由统一输入相关用例及 `src/features/account-workbench/__tests__/` 的组件测试保护。

浏览器测试只使用隔离 API fixture，不连接真实上游或提交真实账号。验证时复用已有前端开发服务。
