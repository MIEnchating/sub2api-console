# 前端全局审查记录

状态：共享模块、功能模块、路由、样式与配置生产文件人工阅读完成；App.tsx 全部分段也已完成，最终统一验证通过。测试通过不替代阅读覆盖。

## 实际范围与分工

- 根已完整阅读所有 `src/lib`、`src/hooks`、`src/components`，见 `frontend-shared-review-2026-09-07.md` 的逐文件库存。
- 功能代理已完整阅读所有 `src/features` 和 `src/api.ts`，见 `frontend-features-review-2026-09-07.md`；9 类缺陷均已修复并完成定向回归。
- 根已完整阅读 `main.tsx`、`router.tsx`、`app-shell-context.ts`、`env.d.ts`、全部 `app-routes`/`routes`，检查生成路由边界。
- 根已完整阅读 `styles.css`、`newapi-index.css`、`theme.css`、`theme-presets.css`；当前产品只切换明暗，不启用命名预设。检查字体、视口归属、内部滚动、移动输入字号、焦点与减少动画偏好。
- 根已完整阅读 `package.json`、`index.html`、`rsbuild.config.ts`、`vitest.config.ts`、`playwright.config.ts`、`tsconfig.json`、`eslint.config.mjs`、`knip.json`、测试 setup 与全部四个 E2E 文件。构建代理默认 localhost，E2E 全量 API 拦截并指向隔离端口。Docker/Nginx/CI 已在后端与部署范围覆盖。
- App.tsx 原始 420–790 行的会话、初始化、主题和 Shell 由根读完；其余前半段由 accounts_upstreams 读完、后半段配置/巡检/策略由 operations 读完；原 8254–9095 的任务展示由根补完。各分段报告列明缺陷与定向验证。

## 已验证修复

共享模块修复见专属报告。包含存储不可用登录崩溃、选项缩短后的键盘选择、多选禁用后仍可清空、精确 Host 默认凭据被 www 别名抢占。功能缺陷另见功能报告。

两项全量静态门禁误报已修正：动作 label 的唯一 const 字符串/条件表达式现在可安全解析；账号状态弹窗明确的初始/恢复焦点允许通过，已有交互测试保护。相关 3 文件 19 测试与 ESLint 通过。

## 最新验证

各专项回归已通过。最终 Vitest 196 文件/994 项、Playwright 26 项及 typecheck、lint、format、Knip、构建全部通过，详细结果统一见 `global-review-2026-09-07.md`，避免引用不同代码时点的旧结果。

未连接生产数据库、真实密码箱、真实上游或真实通知；保留所有原有工作区变更，未提交或发布。
