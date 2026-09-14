# 字典审计清单

## 应进入统一领域字典

- 调度策略及兼容别名：`balanced`、`price_first`、`speed_first`、`reliability` 等。
- 任务状态：`queued`、`running`、`succeeded`、`failed` 等。
- 分组健康状态：`healthy`、`degraded`、`all_fused`、`excluded` 等，并统一状态色调。
- 运行结果状态、回退模式和自动执行字段：供任务结果、运行摘要和审计详情复用。
- 上游类型、账号类型和上游鉴权状态：已由 `domain-dictionaries.ts` 维护。
- 账号调度状态：`healthy`、`degraded`、`fused`、`paused`、`cost_blocked` 等统一展示。

## 由后端字典管理维护

- `platform`：管理平台同步的账号平台目录。
- `group`：管理平台同步的分组目录。

这两类数据具有业务配置属性，保留在后端 `dictionary_entries` 中，前端只接收白名单字段并按顺序展示。

## 保留在 feature 内部

- 告警原因、日志事件、Kuma 资源类型和表单校验选项：它们依赖 feature 上下文，或参与协议校验，不能由用户任意修改。
- 导航标题、按钮文案、错误提示和说明文字：不是枚举字典，不进入字典管理。

## 使用规则

所有稳定枚举展示必须通过集中字典或 feature 的 `constants.ts` / 映射函数读取；未知值不得静默映射为已知状态，应保留可识别的回退文案。业务目录使用稳定 ID、版本和后端同步结果，禁止按名称模糊匹配。
