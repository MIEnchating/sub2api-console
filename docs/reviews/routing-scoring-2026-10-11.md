# 调度评分配置复用（2026-10-11）

本次继续调度决策与写入模块。此前的批量确认修复保留，日志模块保持冻结。当前改动减少健康投影、调度决策和手动优先级规划中的重复配置解析，不改变评分规则或引入全局缓存。

## 原因与实现

`parseEngineConfig` 在一次评估开始时已经解析评分与错误分类配置，但每个账号的历史评分和关键探针判断仍重新解析同一份策略，重复构造事件分数、状态码集合和匹配模式。

`evaluateRoutingHealth`、`scoreRoutingHealth` 和 `withCriticalProbeEvidence` 现在接收本次评估已有的配置。评分函数只读共享的 map 和切片，安全窗口仅修改局部结构体副本。分组覆盖不修改评分配置；下一次评估仍读取最新策略并重新校验，空账号列表也不会绕过无效配置校验。外部 `HealthScore`、`ClassifySample` 接口保持原契约。

改动涉及五个生产文件、两个既有测试文件和一个新增回归/基准文件。`manual_priority_order.go` 中原有的重复预留位置处理改动早于本次，本次仅调整健康计算调用，不把其他已有工作区改动计入成果。

## 验证与基准

新增契约测试覆盖自定义成功/致命分数、自定义致命模式在健康投影与调度决策中的一致性、两次评估间修改策略后立即生效，以及没有账号时拒绝无效评分配置。这些测试在优化前后均通过，本次属于性能重构，不声称复现了功能失败。

隔离基准使用真实 `ProjectHealth` 路径、固定时钟和内存存储 fixture：1000 个账号，每账号 5 条成功样本。修改前后使用相同 fixture、工具链和命令，各运行三组：

```bash
go test ./internal/routing/__tests__ -run '^TestHealthEvaluation' -bench '^BenchmarkProjectHealthBatch$' -benchmem -benchtime=20x -count=3
```

| 场景     |    修改前耗时范围 |    修改后耗时范围 | 修改前分配字节中位数 | 修改后分配字节中位数 | 分配字节降幅 |
| -------- | ----------------: | ----------------: | -------------------: | -------------------: | -----------: |
| 流量样本 | 58.97–63.14 ms/op | 56.92–62.97 ms/op |      23,925,740 B/op |      22,860,788 B/op |      约 4.5% |
| 主动探针 | 48.90–50.68 ms/op | 42.77–46.51 ms/op |      14,629,437 B/op |      12,510,805 B/op |     约 14.5% |

探针场景耗时中位数由 49.39 降至 43.13 ms/op，约下降 12.7%；流量场景耗时区间重叠，不宣称稳定提速。累计分配量不是存活堆内存、SQLite 内存或进程 RSS；fixture 不包含数据库 I/O，不外推生产接口延迟。原始输出保存在 `/tmp/sub2api-routing-scoring-20261011/{before,after}-benchmark.log`。

受影响检查从 `backend/` 执行，包含 15 个显式包：

```bash
bash scripts/check-go.sh test ./internal/routing ./internal/routing/__tests__ ./internal/routingwrite ./internal/routingwrite/__tests__ ./internal/inspection ./internal/inspection/__tests__ ./internal/api ./internal/api/__tests__ ./internal/probe ./internal/probe/__tests__ ./internal/onboarding ./internal/onboarding/__tests__ ./internal/upstreamsync ./internal/upstreamsync/__tests__ ./cmd/server
bash scripts/check-go.sh vet ./internal/routing ./internal/routing/__tests__ ./internal/routingwrite ./internal/routingwrite/__tests__ ./internal/inspection ./internal/inspection/__tests__ ./internal/api ./internal/api/__tests__ ./internal/probe ./internal/probe/__tests__ ./internal/onboarding ./internal/onboarding/__tests__ ./internal/upstreamsync ./internal/upstreamsync/__tests__ ./cmd/server
```

竞态回归与静态检查均退出 0。15 个包全部通过，14 个本次执行，`internal/inspection` 复用 Go 缓存；不是全后端重新执行。已检查本次修改的 Go 格式与差异空白；没有 TypeScript 修改，不重复前端门禁。日志分别保存在同一临时目录的 `affected-test.log`、`affected-vet.log`。

## 审查边界与后续

已接续完整阅读 `routing/service.go`，并复核修改后的配置传递链路；调度决策与写入的生产源码已有全文审查记录。新增逐文件记录见 [本次台账](full-file-audit/routing-scoring-2026-10-11.tsv)，剩余项以 [当前快照](full-file-audit/current-status-2026-10-11.md) 为准。

本次还完整阅读健康证据、历史评分、恢复、容量范围、稳定排序、探针性能、手动排序和写入授权相关的既有测试。`routing/service_test.go` 仅核对本次修改附近的关键探针用例，不因此登记整个大文件已审。其余既有测试审查仍未完成；准备阶段按账号创建 goroutine 的峰值影响也没有测量，不把源码阅读等同于全部性能风险关闭。

下一步完成调度模块剩余测试与准备阶段并发的核对后，再进入模型检测后端。没有提交、推送、部署、重启服务或操作生产数据。
