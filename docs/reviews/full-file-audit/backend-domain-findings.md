# 后端领域与 Uptime Kuma 审查修复素材

本分工逐文件审查后端领域 300 个文件、Uptime Kuma 前端 68 个文件，并交叉复审工作台队列及 OAuth 检查点增量。文件哈希和阅读记录分别保存在 `backend-domains.tsv`、`frontend-kuma.tsv` 和 `root.tsv`。下列 28 项按实际问题归并，包含 2 项性能优化；同一问题的多个边界用例不重复计数。

## 已完成修复

1. **历史筛选被无关损坏 JSON 阻断。** 按邮箱查询任务历史时，一条损坏结果会令整个 SQL 查询失败；现在以 `json_valid` 隔离损坏记录，保留有效匹配项。
   文件：[history_query.go](/root/workspace/sub2api-console/backend/internal/taskstore/history_query.go)。回归：[history_corruption_test.go](/root/workspace/sub2api-console/backend/internal/taskstore/__tests__/history_corruption_test.go)。

2. **常用配置读取加载全部私有大载荷。** 读取目标配置时原先一并扫描工作台凭据和 Kuma 模板内容；现在按所需键读取，`RuntimeSettings` 的完整排序键清单仍保留。
   文件：[store.go](/root/workspace/sub2api-console/backend/internal/configstore/store.go)。行为及性能验证：[settings_scope_test.go](/root/workspace/sub2api-console/backend/internal/configstore/__tests__/settings_scope_test.go)，基准见下表。

3. **损坏密码摘要导致鉴权异常。** 私有库中的 PBKDF2 摘要为空或格式不合法时，原先会进入不合法的派生参数路径；现在先校验算法、迭代参数、盐和摘要，再拒绝无效凭据，不创建会话。
   文件：[store.go](/root/workspace/sub2api-console/backend/internal/configstore/store.go)。回归：[authentication_corruption_test.go](/root/workspace/sub2api-console/backend/internal/configstore/__tests__/authentication_corruption_test.go)。

4. **请求链路混用首个账号名称。** 同一请求包含多个账号记录时，后续记录被标成首个账号；现在按每条记录的稳定账号 ID 读取并缓存展示信息。
   文件：[service.go](/root/workspace/sub2api-console/backend/internal/opstraffic/service.go)。回归：[trace_identity_test.go](/root/workspace/sub2api-console/backend/internal/opstraffic/__tests__/trace_identity_test.go)。

5. **缺失调度策略时模型设置 panic。** 账号存在但控制策略记录缺失时，设置探测模型会对空映射写入；现在显式返回错误，不修改账号，也不记录成功审计。
   文件：[account_control.go](/root/workspace/sub2api-console/backend/internal/business/account_control.go)。回归：[account_model_policy_test.go](/root/workspace/sub2api-console/backend/internal/business/__tests__/account_model_policy_test.go)。

6. **停用告警规则遗漏相关待发送通知。** 停用熔断规则或降级子类型后，绑定失效告警、恢复通知等仍可能待发送；现在同步关闭对应活动告警和待发送通知，保留其他规则状态。
   文件：[alert_policy.go](/root/workspace/sub2api-console/backend/internal/business/alert_policy.go)。回归：[alert_rule_disabling_test.go](/root/workspace/sub2api-console/backend/internal/business/__tests__/alert_rule_disabling_test.go)。

7. **批量鉴权恢复覆盖较新的上游状态。** 第二个 Host 完成时重新写入前缀结果，会覆盖第一个 Host 在其后产生的新失败状态；现在完整快照与本次 Host 状态更新分离，仅更新本次 Host，同时保留全部批次结果。
   文件：[auth_recovery.go](/root/workspace/sub2api-console/backend/internal/business/auth_recovery.go)、[service.go](/root/workspace/sub2api-console/backend/internal/authrecovery/service.go)。协作集成回归：[batch_progress_test.go](/root/workspace/sub2api-console/backend/internal/authrecovery/__tests__/batch_progress_test.go)。

8. **巡检租约和损坏冷却状态处理不严。** 已过期租约可被原持有者续期；损坏或 `null` 冷却记录可能触发提前执行、覆盖原状态或空映射 panic。现在拒绝过期续期，损坏状态明确报错并保留原记录。
   文件：[inspection.go](/root/workspace/sub2api-console/backend/internal/business/inspection.go)。回归：[inspection_state_test.go](/root/workspace/sub2api-console/backend/internal/business/__tests__/inspection_state_test.go)。

9. **账号元数据解析失败泄漏查询连接。** `RoutingAccounts` 在读取到损坏元数据后提前返回，没有关闭结果集；重复失败会耗尽 8 个只读连接。现在在结果集建立后立即安排关闭。
   文件：[routing.go](/root/workspace/sub2api-console/backend/internal/business/routing.go)。回归：[routing_connection_test.go](/root/workspace/sub2api-console/backend/internal/business/__tests__/routing_connection_test.go)。

10. **删除账号投影遗留策略引用。** 清理失效绑定或级联删除上游账号后，账号模型、暂停、排除和人工熔断 ID 留在策略中；现在同一事务清理被删除账号引用，并保留其他账号配置。
    文件：[account_maintenance.go](/root/workspace/sub2api-console/backend/internal/business/account_maintenance.go)、[cleanup.go](/root/workspace/sub2api-console/backend/internal/business/cleanup.go)、[upstream_delete.go](/root/workspace/sub2api-console/backend/internal/business/upstream_delete.go)。回归：[account_deletion_policy_test.go](/root/workspace/sub2api-console/backend/internal/business/__tests__/account_deletion_policy_test.go)。

11. **监控模式不接受有效的零降级阈值。** 阈值为 `0` 时被误当成缺省值，导致健康分 `50` 被按其他阈值判断；现在区分未配置与显式零值。
    文件：[readmodels.go](/root/workspace/sub2api-console/backend/internal/business/readmodels.go)。回归：[monitoring_projection_test.go](/root/workspace/sub2api-console/backend/internal/business/__tests__/monitoring_projection_test.go) 中 `TestMonitoringProjectionHonorsZeroDegradeThreshold`。

12. **监控模式覆盖管理端停用状态。** 管理端已停用且暂无样本的账号，展示状态会被健康推导覆盖；现在保留明确的停用状态。
    文件：[readmodels.go](/root/workspace/sub2api-console/backend/internal/business/readmodels.go)。回归：[monitoring_projection_test.go](/root/workspace/sub2api-console/backend/internal/business/__tests__/monitoring_projection_test.go) 中 `TestMonitoringProjectionPreservesManagementDisabledStatusWithoutSamples`。

13. **账号投影重复解析同一元数据。** 同一账号的多个展示字段反复解析较大的 JSON，约每账号 8 次；现在每账号解析一次并复用结果，减少耗时和分配。
    文件：[readmodels.go](/root/workspace/sub2api-console/backend/internal/business/readmodels.go)。性能验证：[account_projection_benchmark_test.go](/root/workspace/sub2api-console/backend/internal/business/__tests__/account_projection_benchmark_test.go)，并运行账号投影相关回归。

14. **非法延迟进入健康证据。** 负值、`NaN` 或无穷大延迟可进入流量统计；现在在证据入口拒绝这些值，避免污染健康和速度指标。
    文件：[service.go](/root/workspace/sub2api-console/backend/internal/evidence/service.go)。回归：[input_validation_test.go](/root/workspace/sub2api-console/backend/internal/evidence/__tests__/input_validation_test.go) 中 `TestInvalidTrafficLatencyCannotBecomeHealthEvidence`。

15. **错误的分组范围类型退化为全范围。** `managed_group_mode` 不是字符串时，原先可能按全分组采集；现在拒绝无效类型，不扩展业务范围。
    文件：[service.go](/root/workspace/sub2api-console/backend/internal/evidence/service.go)。回归：[input_validation_test.go](/root/workspace/sub2api-console/backend/internal/evidence/__tests__/input_validation_test.go) 中 `TestNonStringManagedGroupModeCannotBroadenCollection`。

16. **取消后的采集结果被记为成功。** 流量请求已分派但执行前取消时，空结果会丢失账号 ID 并进入成功处理；现在保留稳定账号 ID 和取消错误，不记录成功采集。
    文件：[service.go](/root/workspace/sub2api-console/backend/internal/evidence/service.go)。回归：[cancellation_test.go](/root/workspace/sub2api-console/backend/internal/evidence/__tests__/cancellation_test.go)。

17. **上游改名锁错密码箱条目。** 修改 Host 并写默认密码箱项时，原先保护旧名称而写入新名称，可能与其他写操作并发覆盖；现在锁定实际将写入的目标条目。
    文件：[service.go](/root/workspace/sub2api-console/backend/internal/upstreamconfig/service.go)。回归：[vault_rename_test.go](/root/workspace/sub2api-console/backend/internal/upstreamconfig/__tests__/vault_rename_test.go)。

18. **普通设置可占用人工保留优先级。** 普通账号设置提交或排队期间扩大人工保留区间后，仍可能写入保留值；现在排队前和锁内执行前均校验保留范围。
    文件：[service.go](/root/workspace/sub2api-console/backend/internal/accountops/service.go)。回归：[settings_priority_test.go](/root/workspace/sub2api-console/backend/internal/accountops/__tests__/settings_priority_test.go)。

19. **账号数值设置接受非十进制或无界输入。** 负载倍率等输入允许分数、十六进制、下划线或过大指数；现在使用已有十进制工具的长度和指数边界，在排队前拒绝不合法输入。
    文件：[service.go](/root/workspace/sub2api-console/backend/internal/accountops/service.go)。回归：[decimal_input_test.go](/root/workspace/sub2api-console/backend/internal/accountops/__tests__/decimal_input_test.go)。

20. **排队中的模型发现越过新增人工保护。** 排队后账号进入人工优先位，任务仍可能访问远端；现在取得锁后再次核对人工保护，受保护账号不执行模型发现。
    文件：[model_sync.go](/root/workspace/sub2api-console/backend/internal/accountops/model_sync.go)。回归：[model_discovery_protection_test.go](/root/workspace/sub2api-console/backend/internal/accountops/__tests__/model_discovery_protection_test.go)。

21. **排队中的主动探活越过新增人工保护。** 人工优先位或人工熔断在排队期间生效后，探活仍发送请求并写入健康证据；现在在管理目标锁内、出站前批量复核，跳过受保护账号且不生成证据。
    文件：[protection.go](/root/workspace/sub2api-console/backend/internal/probe/protection.go)、[service.go](/root/workspace/sub2api-console/backend/internal/probe/service.go)。回归：[queued_protection_test.go](/root/workspace/sub2api-console/backend/internal/probe/__tests__/queued_protection_test.go)。

22. **模拟通知消耗真实待发送告警。** `Deliver(ctx, true)` 虽未发送网络请求，却建立并完成真实投递记录，使之后正式投递跳过告警；现在模拟仅返回计算结果，不变更真实投递状态。
    文件：[service.go](/root/workspace/sub2api-console/backend/internal/notification/service.go)。回归：[dry_run_test.go](/root/workspace/sub2api-console/backend/internal/notification/__tests__/dry_run_test.go)，验证模拟后正式发送仍投递 3 条。

23. **流式开户探活首段成功掩盖后续失败。** 第一段文本返回后立即成功，忽略后续业务错误、嵌套 `response.failed` 或损坏事件；现在检查完整数据流，仅在后续事件均有效时接受生成结果。
    文件：[probe.go](/root/workspace/sub2api-console/backend/internal/onboarding/probe.go)。回归：[probe_stream_test.go](/root/workspace/sub2api-console/backend/internal/onboarding/__tests__/probe_stream_test.go)，覆盖三种首段文本后的失败。

24. **Kuma 请求体快捷编辑损坏数字精度。** 编辑模型或消息时整段 `JSON.parse`/`JSON.stringify` 改变未编辑的大整数和小数；现在通过已有 JSON 解析库定点替换目标节点，保留其余原文，并按 JSON 最后同名属性生效的语义处理重复属性。
    文件：[request-body.ts](/root/workspace/sub2api-console/frontend/src/features/uptime-kuma/lib/request-body.ts)。回归：[request-body.test.ts](/root/workspace/sub2api-console/frontend/src/features/uptime-kuma/lib/__tests__/request-body.test.ts)，包括 `9007199254740993` 和高精度小数。

25. **Kuma 新增鉴权未校验必填凭据。** 新建 Basic/Bearer 监控且凭据为空时，前端允许提交；现在对应字段阻止提交，同时继续允许编辑既有鉴权时留空保留凭据。
    文件：[schemas.ts](/root/workspace/sub2api-console/frontend/src/features/uptime-kuma/lib/schemas.ts)。回归：[monitor-auth-validation.test.ts](/root/workspace/sub2api-console/frontend/src/features/uptime-kuma/lib/__tests__/monitor-auth-validation.test.ts)。

26. **Kuma 接受倒序正常状态码区间。** `299-200` 满足旧正则但不合法；现在同时校验区间起止顺序，并在状态码字段提示，正常区间及单项仍可提交。
    文件：[schemas.ts](/root/workspace/sub2api-console/frontend/src/features/uptime-kuma/lib/schemas.ts)。回归：[validation.test.ts](/root/workspace/sub2api-console/frontend/src/features/uptime-kuma/lib/__tests__/validation.test.ts)。

27. **Kuma 非法维护日期没有可见字段错误。** 数组项为 `0`、`32`、小数、非数字或未完成分隔项时，Zod 阻止提交但界面只读取数组级消息；现在显示中文日期错误并同步 `aria-invalid`，合法的多日期录入仍正常提交。
    文件：[maintenance-form.tsx](/root/workspace/sub2api-console/frontend/src/features/uptime-kuma/components/maintenance-form.tsx)。回归：[maintenance-dates.test.tsx](/root/workspace/sub2api-console/frontend/src/features/uptime-kuma/components/__tests__/maintenance-dates.test.tsx)。没有修改原有日期录入方式。

28. **工作台队列恢复丢失已完成账号名称。** 成功结果从私有队列重新解析时只传入凭据和邮箱，`JSON account` 等原名称变成空串或邮箱；现在将原名称一并交由真实解析器校验，最终预览保留原名。
    文件：[queue_storage.go](/root/workspace/sub2api-console/backend/internal/accountworkbench/queue_storage.go)。公共恢复流程回归：[queue_result_name_test.go](/root/workspace/sub2api-console/backend/internal/accountworkbench/__tests__/queue_result_name_test.go)。

## 性能基准

下列为本轮修复前后已执行的隔离基准记录，数值四舍五入；并非生产吞吐承诺。原始基准终端输出没有另存为独立日志，基准代码已保留。时间受同机并行测试影响，主要结论也由每次操作的分配量下降支持。

| 基准                                                  | 显式 fixture                                      | 修复前                    | 修复后                   |
| ----------------------------------------------------- | ------------------------------------------------- | ------------------------- | ------------------------ |
| `BenchmarkTargetSettingsWithUnrelatedPrivatePayloads` | 临时私有库；两个无关工作台/Kuma 记录，共约 1.8 MB | 11.47 ms/op；3.93 MB/op   | 0.102 ms/op；1.68 KB/op  |
| `BenchmarkAccountsWithLargeMetadata`                  | 临时业务库；20 个账号，每项约 32 KB 元数据        | 76.230 ms/op；34.58 MB/op | 19.296 ms/op；8.51 MB/op |

在 `backend/` 可重跑当前版本：

```bash
go test ./internal/configstore/__tests__ -run '^$' -bench '^BenchmarkTargetSettingsWithUnrelatedPrivatePayloads$' -benchmem
go test ./internal/business/__tests__ -run '^$' -bench '^BenchmarkAccountsWithLargeMetadata$' -benchmem
```

## 验证范围

行为缺陷先通过失败回归确认，再实现修复并观察通过；性能优化以对应基准和业务语义回归验证。Go 测试使用临时数据库及隔离 HTTP/浏览器适配器，没有连接生产数据库、密码箱、上游或 QQ Bot。

- 本分工后端修复均已运行相应模块及 `__tests__` 的受影响竞态测试。最终队列/检查点集合 `go test -race ./internal/accountworkbench/__tests__ -run 'Test(MixedQueue|OAuthQueue|AutomaticOAuthCheckpoint|AutomaticCheckpoint|Queue)' -count=1 -timeout=120s` 通过，用时 78.488s，包含第 28 项新回归。
- Kuma 全模块 `bun run test src/features/uptime-kuma --maxWorkers=2 --minWorkers=1`：27 个测试文件、117 项通过；随后补充重复属性兼容并完成最终代码后，`request-body.test.ts` 的 14 项通过。两次结果存在重叠，不相加计数。
- 最终 `bun run typecheck` 通过；本分工改动的 7 个 Kuma TS/TSX 文件 ESLint 和 oxfmt 检查通过。
- 一次高并发 Kuma 全模块执行曾发生旧 JSON 编辑器懒加载测试等待超时；该文件单独运行及降低工作线程后的全模块运行均通过，未为此修改业务逻辑或增加固定等待。
- 整仓最终测试、完整 Go race/vet、构建及最新并发变更的核对由主审查报告统一记录。本报告不将早期失败日志或其他分工仍在运行的检查宣称为通过。

本次素材整理仅新增本文档，未修改业务代码。
