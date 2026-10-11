# 模型检测后端执行与进度优化（2026-10-11）

本轮集中审查 modelcheck 后端的账号准备、行为检测、动画/预检、自动计划、恢复、取消和结果边界，修复三个具体问题。日志与已收口的 routing / routingwrite 保持冻结。本报告不包含前端布局或浏览器验证，也不代表全局六项目标完成。

## 问题与改动

1. **行为检测开始后没有及时显示正在处理的账号。** 原实现仅消费开始事件，直到某个检测完成才保存状态；开始与完成使用两个通道，还可能先处理完成、后处理开始，让已完成账号重新出现在 active 中。现在用一个有序事件通道处理开始与完成，开始时立即保存实际 active 状态，完成计数保持为零；发布时复制 active map 和已完成结果切片，保护既有快照。通道缓冲随两个工作线程设置，不再随全部组合数增长。
2. **校验所选账号反复读取整个账号目录。** 原流程先读取 Accounts，再逐项读取 Account；保存多个启用计划会重复构建全目录映射。现在直接按稳定 ID 读取所选详情，并从详情获取账号状态；缺失、存储异常与返回 ID 不匹配分别拒绝。回归中保存两个计划不再调用全目录读取。原有逐账号详情查询仍然存在，不声称改成数据库批量查询。
3. **动画每个文本片段都重复保存“生成中”状态。** Responses、Chat、Anthropic 和 OAuth 流处理现在仅在首次非空文本时触发一次回调，后续继续累积文本及用量。重试的每个 HTTP 响应分别拥有首次输出通知，不改变有限重试、取消和最终产物处理。

行为检测新增开始状态持久化，是及时反馈所需的写入；它不属于动画重复状态写入的减少。两个路径的收益分别统计。

## 失败到通过的证据

新增三个职责独立的测试文件，使用隔离任务数据库、本地 HTTP fixture 或受控 OAuth Transport，不访问生产数据库、真实上游或密码箱。请求开始与释放通过通道控制，超时只作为失败边界。

| 回归                  | 修复前                               | 修复后定向竞态检查                                |
| --------------------- | ------------------------------------ | ------------------------------------------------- |
| 选中账号校验          | 尝试读取全目录；错误稳定 ID 被接受   | 仅选中详情、缺失/错误 ID/存储失败正确区分         |
| 行为检测状态          | 上游阻塞期间未保存 active            | 完成前即可读取 active，不虚构结果，完成账号不复活 |
| API Key 动画          | 237 个文本片段保存 237 次 generating | 保存 1 次，完整原文及可用 SVG 保留                |
| OAuth 动画            | 237 个文本片段保存 237 次 generating | 保存 1 次，完整原文及可用 SVG 保留                |
| Chat 回退及 Anthropic | 同一回调原先位于每片段分支           | 两条协议各保存 1 次并保留完整产物                 |

237 → 1 只指单个响应的 generating 状态快照，不是整个任务的全部数据库写入。没有测量生产延迟、RSS 或 SQLite 内存，不由次数变化推导这些指标。

原始输出位于 `/tmp/sub2api-modelcheck-20261011/`：`before.log`、`stream-before.log`、`oauth-stream-before.log` 为有效失败复现，`after.log`、`stream-after.log`、`compatible-stream-after.log` 为定向通过。早期 SVG 测试曾错误地把清洗后的 SVG 与原文逐字比较，已改为校验 Source 完整及 SVG 可用；该次断言错误不作为缺陷证据。

首次八包回归发现 `TestPreparedRunKeepsPublishedProfileSnapshot` 的旧详情 fixture 缺少稳定 ID；另一个人工优先账号 fixture 也补齐详情。保留原有业务断言。首次回归其余七包通过，但之后流式生产代码发生变化，因此这轮结果不能冒充最终候选通过。

## 验证范围

从 `backend/` 执行：

```bash
bash scripts/check-go.sh test ./internal/modelcheck ./internal/modelcheck/__tests__ ./internal/api ./internal/api/__tests__ ./internal/onboarding ./internal/onboarding/__tests__ ./internal/accountworkbench/__tests__ ./cmd/server
bash scripts/check-go.sh vet ./internal/modelcheck ./internal/modelcheck/__tests__ ./internal/api ./internal/api/__tests__ ./internal/onboarding ./internal/onboarding/__tests__ ./internal/accountworkbench/__tests__ ./cmd/server
```

最终八包竞态回归与 vet 均退出 0。6 包实跑：modelcheck（19.249s）、`modelcheck/__tests__`（509.048s）、onboarding（92.596s）、`onboarding/__tests__`（222.530s）、`accountworkbench/__tests__`（135.393s）、cmd/server（1.471s）；api 及 `api/__tests__` 两包由 Go 判定缓存有效并复用。最慢包约 8.5 分钟，不是测试卡住后反复重启。记录文件为 `test-final.log` 与 `vet-final.log`。涉及 Go 文件的 gofmt 与差异空白检查已通过；没有修改 TypeScript/TSX，因此本轮不重跑前端类型检查。既有未失效结果由 Go 缓存复用，不清缓存、不强制 `-count=1`。

## 审查范围与限制

本轮全文阅读并登记 80 个文件：运行源码、测试、fixture 和两个更新的旧测试；其余 modelcheck 文件沿用内容哈希仍匹配的既有审查记录。台账见 [modelcheck-execution-2026-10-11.tsv](full-file-audit/modelcheck-execution-2026-10-11.tsv)。阅读记录与回归结果分别记账，不将任一项替代另一项。

- 独立动画任务仍按账号创建等待协程，并保存与目标数成比例的结果；实际请求受独立/全局并发闸门限制。本轮未测量超大选择范围的峰值资源，也未将这一资源模型宣称为常数空间。
- Claude 采用有界分包工作线程，Sol 按受校验题库拆分请求；本轮没有改变评分、阈值或模型身份推断规则。
- SaveDraft 保留输入 payload 的引用；当前生产调用方为 API handler，提交后不继续修改该输入，返回视图单独克隆。内部调用方仍须遵守输入所有权，不能把返回值隔离等同于任意外部输入修改都安全。
- 既有部分超时测试使用真实时钟，一些 API 表格用例依赖前面的保存步骤；部分断言先索引后核对长度。这些属于测试维护限制，本轮不以全文阅读宣称已全面重写测试质量。
- HTML 预览仍依赖前端 opaque-origin sandbox 与 CSP；后端内容校验不证明允许执行的内联脚本绝对无法导航 iframe。该边界保留在既有规范中。

本轮三项问题与当前逐文件审查均已收口，下一模块进入模型检测前端，重点核对选择、任务状态、结果和预览交互；后端已确认问题不再无故追加零散优化。
