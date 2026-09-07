# 运行领域审查记录（2026-09-07）

本记录对应全局审查中的运行、巡检、路由、探活、告警和日志范围。工作区包含既有变更和其他审查代理的并行修改；以下仅记录本次实际检查、复现和修复结果。分配生产文件的人工覆盖及九类确定缺陷修复已完成，全仓最终验证由主审查汇总。

## 已完整阅读的生产文件

- `backend/internal/evidence/service.go`
- `backend/internal/opstraffic/service.go`
- `backend/internal/probe/service.go`、`manual_batch.go`
- `backend/internal/modelcheck/service.go`、`checker.go`、`configuration.go`、`direct.go`、`profiles.go`
- `backend/internal/inspection/runner.go`、`scheduler.go`、`manual.go`
- `backend/internal/routing/service.go`、`scoring.go`、`recovery_status.go`
- `backend/internal/routingwrite/service.go`、`batch.go`
- `backend/internal/notification/service.go`
- `backend/internal/notificationtarget/service.go`、`gateway.go`
- `backend/internal/alerting/service.go`
- `backend/internal/logs/service.go`、`maintenance.go`
- `backend/internal/systeminfo/collector.go`
- `backend/internal/business/evidence.go`、`probe.go`、`model_check_configuration.go`、`inspection.go`、`health_history.go`、`account_block.go`、`account_state.go`、`status_values.go`、`group_probe_models.go`、`routing.go`、`routing_write.go`、`group_policy.go`、`group_allocation.go`、`policy.go`、`manual_priority.go`、`alert_evaluation.go`、`alert_policy.go`、`alerts.go`、`history.go`

## 已复现并修复

| 问题 | 失败证据与修复 | 最新验证 |
| --- | --- | --- |
| QQ Bot HTTP 200 业务失败被记为投递成功；数字消息 ID 丢失精度，无效类型被接收 | `notification/response_contract_test.go` 先复现 `code != 0`、`errcode != 0`、`success:false` 带 ID 的误判，及布尔、对象、零、超大整数 ID。使用 `UseNumber`，校验业务状态、ID 类型和完整 JSON 响应。 | `go test ./internal/notification -count=1` 通过（1.570s） |
| 日志超大页码导致整数溢出和切片 panic | `logs/pagination_test.go` 先复现 `Page:math.MaxInt`；在乘法前检查页码范围，超范围返回空页。 | `go test ./internal/logs -count=1` 通过（0.008s） |
| Claude 模型检测取消后尚未执行的请求计为成功；解析接受尾随第二个 JSON 数组 | `modelcheck/cancelled_requests_test.go` 先复现取消后成功计数为 6 和两个数组被接受；初始化未执行请求的错误状态，解析后要求 EOF。 | `go test ./internal/modelcheck -count=1` 通过（1.560s） |
| 巡检倍率同步执行批量参数仍采用调度器旧配置 | `inspection/rate_batch_policy_test.go` 先复现当前策略批量 3、实际任务批量 7；执行采用计划阶段已解析的当前策略参数。 | `go test ./internal/inspection -count=1` 通过（2.214s） |
| 路由写入和交还控制权在等待租约后未复核运行模式 | `routingwrite/runtime_mode_test.go` 先复现租约获取期间切换监控模式，两个入口仍调用远端 mutation；获得租约后重新读取并校验模式。 | `go test ./internal/routingwrite -count=1` 通过（3.929s） |
| 健康样本字符串时间排序选错最新证据，并在保留窗口中删除更新样本 | `business/health_time_order_test.go`、`routing/sample_time_order_test.go` 先复现整秒/小数秒及不同时区排序错误；持久化和路由筛选统一 UTC 固定九位精度，保留索引排序。 | routing 全包通过（0.636s）；business 本次所有新增/修改回归 race 通过（14.376s）。旧 probe retention 断言已改为真实时间等值断言；跨范围的流量排行边界由主代理处理 |
| 上游未来流量被当作新鲜证据，压制主动探测和后续拉取 | `evidence/future_samples_test.go` 先复现真实 Collect 路径接收未来一小时样本并跳过探活，以及已存未来 probe/traffic/fetch 时间阻止后续工作；过滤未来超过一分钟的样本并让超前的已存时间过期，与路由已有容错一致。 | evidence 全包 race 通过（1.100s），含一分钟时钟容错边界 |
| 多分组账号的次分组详情丢失健康分与样本数 | `business/group_allocation_test.go` 既有 canonical-account 场景增加健康评估契约后稳定失败；去除按成员分组匹配健康评估的条件，按稳定账号 ID 复用唯一评估。 | 分组分配与健康时间相关测试通过（1.056s） |
| QQ Bot 投递结果不确定时，告警运行记录和任务仍标记成功 | `business/alert_delivery_status_test.go` 先复现 `Uncertain:1` 返回 succeeded；改为 failed，并提示“待确认 1 项；请先核实 QQBot 投递结果”，让任务状态沿用失败结果。 | 对应回归与告警评估相关测试通过（0.483s） |

测试仅使用临时数据库和模拟 HTTP 边界，不连接生产数据库、真实密码箱或 QQ Bot。

## 范围完成情况

已完整阅读全部分配生产文件。租约等待期间模式变化、健康样本时间顺序及未来样本均已稳定复现、修复并验证。尚未确认的候选不计入缺陷数量。主审查负责最终全仓 `go test -race ./...`。

给主审查的协调信息：本代理 `/root/operations` 已实际运行，全部 inventory 已完成。`business/evidence.go`、`business/probe.go` 两个健康样本持久化入口已将 `observed_at` 规范为 UTC 固定九位小数，匹配 taskstore 的时间存储约定；只修复本代理范围，不改公共库。旧 probe retention 断言已替换为真实时间等值断言；相关测试已通过。

## 验证进度与交接

- 受影响八包 `go vet` 在最终两处 business 变更后再次通过。
- 涉及文件 `gofmt -l` 和 `git diff --check` 无输出。
- 八包 race 已结束：`evidence`（1.100s）、`inspection`（12.733s）、`modelcheck`（18.548s）、`notification`（24.663s）、`logs`（1.032s）、`routing`（2.634s）、`routingwrite`（33.546s）全部通过。`business`（361.567s）唯一失败项是主审查新增的 `TestTrafficRankingIncludesSubsecondRequestsAtWindowStart`，属于主审查的流量排行边界修复；未出现竞态报告。本次 business 所有新增/修改回归另跑 race 通过（14.376s），含在八包 race 启动后新增的次分组健康评估与不确定投递状态修复。
- 全仓 `go test -race ./...` 由主审查统一执行；本报告不将范围内结果当作全仓验证。

## 后续已核实的候选与边界

- 全局并发预算问题已由后续专项稳定复现并修复，包括受限 scope、范围外容量、稳定 ID 去重、下限和未确认缩容边界；详见 `scaling-budget-fixes-2026-09-07.md`。
- `BeginAlertDelivery` 已先持久化 commit_unknown，可保护发送后取消造成的最终记录失败不被自动重发；无需为取消路径盲目重复发送。
- 健康时间格式规范面向当前首次建库系统，不增加旧库迁移；其他跨模块时间读写由主审查汇总。

最终全仓 race 与各项前后端检查均已通过；以上“验证进度与交接”保留早期运行的真实结果，当前结论以 `global-review-2026-09-07.md` 为准。
