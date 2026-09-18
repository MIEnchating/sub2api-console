# 调度问题修复记录（2026-09-16）

对应 [原审查记录](scheduling-policy-review-2026-09-16.md) 的 11 项问题。所有缺陷先通过隔离回归用例复现，再修改实现；开发与测试没有访问生产数据库、真实上游或通知渠道。

后续关于持续发现、稳定排位和 WS 满载恢复的实现见 [持续发现、稳定排位与满载恢复](scheduling-stability-and-overload-2026-09-16.md)。

| 原问题 | 修复结果 | 主要回归位置 |
| --- | --- | --- |
| 1. 保底留下最差账号 | 最小池纳入本轮未完成处置的候选，先熔断致命和低分账号 | `routing/__tests__/minimum_pool_selection_test.go` |
| 2. 旧策略目标继续写入 | 计算结果携带策略指纹，写入前校验当前策略 | `routingwrite/__tests__/calculation_policy_test.go` |
| 3. 排队写入未及时撤权 | 获得发送槽后及每次 HTTP 重试前核对授权；已发出失败保持失败语义 | `routingwrite/__tests__/sending_authorization_test.go` |
| 4. 高频成功无法满足保持时长 | 独立读取保持区间及边界请求，保留失败、空回复与新鲜期约束 | `routing/__tests__/recovery_hold_test.go`、`business/__tests__/recovery_traffic_test.go` |
| 5. 部分退管丢失基线 | 全部恢复成功后删除基线，部分成功只保存确认进度 | `routingwrite/__tests__/partial_release_test.go` |
| 6. 分组名称与 ID 碰撞 | 采集、探活及参与状态仅按稳定分组 ID 匹配范围 | `evidence/__tests__/group_scope_test.go`、`probe/__tests__/group_scope_test.go` |
| 7. 编辑范围外分组隐式关闭 | 无分组覆盖时守护开关默认开启，独立于全局参与范围 | `frontend/src/features/groups/__tests__/editor-participation.test.tsx` |
| 8. 已关闭分组显示参与 | 列表与策略快照共同纳入分组自身开关 | `business/__tests__/group_participation_test.go` |
| 9. 采集失败仍报告成功 | 完全不可用标失败并停止本轮计算；可用降级标部分失败 | `inspection/__tests__/collection_status_test.go` |
| 10. 零调度分回退历史分 | 零作为有效调度分参与健康门控 | `routing/__tests__/zero_routing_health_test.go` |
| 11. 关闭验证使冷却失效 | 明确远端成功且实际变更字段的审计可启动冷却，不放宽容量或恢复确认 | `routingwrite/__tests__/cooldown_test.go` |

表中后端测试路径相对于 `backend/internal/`。索引调整直接修改首次建库 schema，没有添加旧库升级；冷却查询不再强制绑定某个部分索引，查询计划测试仍要求按操作类型和账号索引查找。

## 满载错误

控制台将两种用户提供的 `response.failed` 文案统一作为可恢复的网关失败：`Our servers are currently overloaded. Please try again later.` 与 `The service is busy. Please retry later.`。此前前者被当作额度/限流，后者通常被当作未知错误。现在重复失败进入同一健康评分及网关处置规则，不自动认定凭据失效；显式 401、429 仍保留各自鉴权和限流语义。

主动探活在首个有效文本到达之前，从内层错误读取 HTTP 状态；外层 HTTP 200 不覆盖流内 503。缺少状态但明确繁忙时映射为 503，并遵守已有重试开关、次数、状态码白名单和发送前暂停检查。一次探活的多次尝试仍只保存一条结果，恢复结果保留 `[503, 200]` 的尝试状态。探活仍以首个有效文本判断可用，不验证完整生成过程；真实请求输出后的失败依赖运维记录采集。

回归位于 `routing/__tests__/capacity_classification_test.go`、`routing/__tests__/capacity_scheduling_test.go` 与 `probe/__tests__/capacity_failure_test.go`，覆盖两种原始事件、HTTP 200/缺失状态、真实 503、连续失败、明确鉴权/限流及有限重试恢复。价格优先、速度优先、均衡、稳定性四种策略均验证持续满载降低权重、不生成凭据清理目标。

控制台不转发生成请求。用户连接中的重试由 `/root/workspace/sub2api` 网关处理：已有输出前暂存和切号能力，输出后的请求不可安全重放。统一忙碌错误的分类能减少不必要的失败外露，但所有候选上游持续满载或已输出后失败仍会返回错误，不能保证消除重连。

本次同步修改该网关的 `backend/internal/service/openai_gateway_upstream_errors.go`，精确纳入 `The service is busy. Please retry later.`，使其与已支持的 overloaded 错误共用 503、请求级临时故障、现有同账号有限重试和换号逻辑。不增加重试预算，不触发 API Key 池的账号健康熔断或临时冷却，不改变调度 EWMA。原始事件覆盖 OAuth/API Key 与 native/passthrough 四条路径；输出后的失败仍只返回一个失败事件，不进行透明重放。未部署或重启任何服务。

## 验证

- Console 完整检查使用 `bash scripts/check-go.sh test`，覆盖普通包及自动发现的 `__tests__`。首轮只有账号工作台测试包触及默认 10 分钟总时限，其余包全部通过；该包使用 `go test -race -timeout=20m ./internal/accountworkbench/__tests__` 单独重跑，489.814 秒通过，没有断言失败或 race 报告。
- 完整检查之后收敛的冷却索引查询、缓存探活降级及四策略满载联动分别执行最新受影响 race 回归，全部通过。采集缓存用例也验证有可用证据时继续计算、无可用证据时停止计算。
- Console `bash scripts/check-go.sh vet` 以及最后修改模块的补充 vet、gofmt、`git diff --check` 均通过。
- 前端分组模块 10 个文件、53 项测试通过；`bun run typecheck`、涉及文件 ESLint 与 oxfmt 检查通过。
- Sub2API 网关相关 service/handler 普通测试及 race 测试通过，`go vet ./internal/service ./internal/handler`、gofmt、`git diff --check` 均通过。

所有任务启动的测试进程均已结束。代码保留在工作区，未提交、部署或重启服务。
