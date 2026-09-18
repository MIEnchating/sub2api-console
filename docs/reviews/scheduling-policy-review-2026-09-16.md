# 调度链路审查记录（2026-09-16）

审查基于当前工作区（HEAD `7b92e6c972b97be1803029ef2ab31a6b116fb867`，包含既有未提交修改），不是仅检查本次 diff。范围覆盖策略保存与继承、四类权重算法、健康证据、降级、熔断与恢复、成本墙、共享并发、实际写回、退管、巡检编排及分组界面。

本轮为审查，未修改业务实现。下面的问题均通过隔离测试复现；已有测试通过不能证明这些边界正确。测试只使用临时 SQLite、隔离 HTTP 服务及受控时间，没有连接真实上游、生产数据库或通知渠道。

后续修复及满载错误处理记录见 [调度问题修复记录](scheduling-policy-fixes-2026-09-16.md)。本文保留修复前的复现结果。

## 已确认问题

### 1. P1：保底可能留下失效凭据，熔断仍可成功的账号

- 位置：`backend/internal/routing/service.go:1227`、`:1249`、`:1348`（`applyFuseBudgets` / `availableForMinimumPool`）。
- 触发：同组两个账号都进入待熔断，最小池为 1；A 返回 401、凭据失效、健康分 0；B 连续三次成功但首字为 20 秒、健康分 65，策略允许慢响应触发熔断。
- 实际：A 变为 `survivor` 并保持可调度，B 变为 `fused` 并停止调度，组内只剩无法成功的账号。
- 原因：待熔断账号提前统一标记为不可调度；处理顺序优先致命与低分账号，保底检查看不到尚未处理的其他候选，于是先强留最差账号。
- 修复方向：按整轮候选确定保底集合，优先留下仍有成功能力的账号，再决定熔断对象。
- 复现：`TestReviewMinimumPoolKeepsSuccessfulSlowAccountInsteadOfInvalidCredentials`。

### 2. P1：计算后变更策略，旧目标仍写入已经排除的分组

- 位置：`backend/internal/routing/service.go:234`、`backend/internal/inspection/runner.go:925`、`backend/internal/routingwrite/service.go:240`。
- 触发：先计算并持久化调度目标，随后保存策略将该组排除，再执行原目标。
- 实际：已排除组的两个账号仍被写入，一个被暂停，另一个并发从 4 增至 9，写回结果显示成功。
- 原因：`Apply` 才读取后续授权复核使用的策略快照，目标不携带计算时策略版本；之后的复核只能识别进入 `Apply` 以后的变更。
- 修复方向：调度目标绑定计算策略版本，在目标持久化、取得变更租约及实际发送前校验，不一致时跳过并重算。
- 复现：`TestReviewComputedTargetMustNotWriteAfterGroupExcluded`，使用真实计算、策略保存与写回服务。

### 3. P1：切换监控模式后，尚未发送的排队写入仍会执行

- 位置：`backend/internal/routingwrite/batch.go:175`、`:57`。
- 触发：写回并发为 1，两个不同参数的账号等待写入；第一个 PUT 执行时切换为监控模式。
- 实际：第二个尚在等待发送机会的 PUT 仍发出，结果 `Changed:2`、`Failed:0`。
- 原因：批次只在启动多个写入 goroutine 之前统一复核授权，等待并发槽结束后直接发送；模式切换不再被检查。
- 修复方向：取得实际发送机会后、每次远端变更前重新检查模式与策略；多步骤变更同样需要复核。
- 复现：`TestReviewPendingWritesStopWhenModeChangesDuringFirstWrite`。模式变更发生在受控 HTTP 处理期间，不依赖 sleep 或碰运气的竞态。

### 4. P2：持续成功流量使降级账号迟迟不能恢复

- 位置：`backend/internal/routing/service.go:337`、`:2784`（`recoverySpan`），`backend/internal/routing/recovery_status.go:28`。
- 触发：默认恢复保持 60 秒，默认数量窗口 60 条；降级账号持续每秒至少一次成功请求。
- 实际：提供 200 条连续成功、覆盖 199 秒，仍返回 `degraded`、健康分 100，原因是“健康保持 59/60 秒”。只要高频成功持续，旧成功不断被数量窗口挤出，此条件一直不能满足。
- 原因：健康保持时长从最新数量窗口内最早的成功样本计算，没有保留连续健康已经持续多久。
- 修复方向：独立记录连续健康开始时间，或按保持时长查询恢复证据，并确保期间没有失败。
- 复现：`TestReviewContinuouslyHealthyTrafficCanRecoverDegradedAccount`。

### 5. P2：退管部分失败后删除原始基线，失去恢复依据

- 位置：`backend/internal/routingwrite/service.go:833`、`backend/internal/business/routing_write.go:210`。
- 触发：账号原为并发 4、可调度，托管改为并发 6、暂停。自动交还控制权时，并发恢复成功，但恢复可调度状态返回 503。
- 实际：任务正确报告失败，但原始托管基线被删除，账号仍然暂停，后续不能按原始状态继续恢复。
- 原因：部分成功的读回仍携带 `ReleaseControl=true` 提交，数据库先删除基线，调用方随后才返回剩余错误。
- 修复方向：仅在所有需恢复字段均确认一致后删除基线；部分成功保留原始恢复目标及已经确认的进度。
- 复现：`TestReviewPartialAutomaticReleaseRetainsBaseline`。

### 6. P2：分组名称与稳定 ID 混用，误选或漏选自动探活

- 位置：`backend/internal/evidence/service.go:648`、`backend/internal/probe/service.go:1035`、`backend/internal/business/catalog.go:845`。
- 触发：账号 41 在 ID=7、名称 codex 的组，账号 42 在 ID=8、名称“7”的组。
- 实际：仅守护 ID=7 时，探活计划包含 `[41,42]`；排除 ID=7 时，计划变成 `[]`，本应仍探测账号 42。分组列表也把 ID=8 错报为排除。
- 原因：范围匹配把 GroupName 与 GroupID 放进同一个集合；探活执行时针对 selected 范围的二次过滤仍匹配名称，不能纠正误选。排除范围在 Plan 中已经漏选的账号也不会进入后续执行。
- 修复方向：业务范围只匹配稳定组 ID，名称仅用于展示；同步修正采集、探活和列表读模型。
- 复现：`TestReviewPlanUsesStableGroupIDDespiteNumericName` 的 selected/excluded 子用例，以及 `TestReviewNumericGroupNameMustNotMatchExcludedID`。

### 7. P2：编辑范围外分组，隐式写入关闭守护

- 位置：`frontend/src/App.tsx` 的 `GroupsPage.openEditor`，`enabled: override.enabled ?? group.participation_status === "participating"`。
- 触发：仅守护 ID=8，ID=7 未配置 override、处于范围外且允许编辑；用户只把 ID=7 的策略改成价格优先并保存。
- 实际：请求额外写入 `enabled:false`。以后把 ID=7 加回受管列表，它仍被后端跳过，必须另外开启分组守护。
- 原因：编辑器把由全局范围决定的 `participation_status` 当成分组自身 `enabled` 默认值。
- 修复方向：分别处理分组开关与全局范围；无 override 的分组开关沿用后端默认 true。
- 复现：`group-editor.test.tsx` 的真实 GroupsPage 点击、选择与保存交互。明确排除的分组不可编辑，此问题针对 selected 模式下暂未纳入的组。

### 8. P2：关闭参与守护后，列表仍报告正在参与且正常

- 位置：`backend/internal/business/catalog.go:863`。
- 触发：组中存在可调度账号，保存分组 override `enabled:false`。
- 实际：返回 `participation_status="participating"`、`status="healthy"`，而调度引擎和证据采集已经跳过该组。
- 原因：参与状态函数只检查全局范围，不检查分组 override 的 enabled。Groups 与 PolicySnapshot 共用该函数。
- 修复方向：读模型按与执行引擎一致的顺序合并分组开关和全局范围。
- 复现：`TestReviewDisabledGroupMustNotReportParticipating`，使用真实 Store 与临时数据库。

### 9. P2：真实流量采集全部失败，巡检仍标记成功

- 位置：`backend/internal/inspection/runner.go:907`、`:910`、`:1054`（`executeTask` / `strictEvidenceFallback`）。
- 触发：正常自动巡检读取流量，真实管理接口返回 HTTP 503，主动探测与恢复关闭。
- 实际：所有采集均失败，但任务与心跳返回 `succeeded`、文案“巡检完成”；错误只留在结果内部 `evidence.source_errors`。
- 原因：全局巡检允许采集降级继续执行，但编排层没有把实际采集错误纳入失败或部分失败状态。
- 修复方向：允许后续计算继续，仍将真实接口失败纳入任务结果；区分正常跳过、可用降级与真正的数据源错误。
- 复现：`TestReviewInspectionReportsTrafficCollectionFailure`，使用真实 Runner、Evidence、Routing、Store 与隔离 HTTP 503 服务。

### 10. P2：合法的零调度分被当成未初始化，绕过健康门控

- 位置：`backend/internal/routing/service.go:2216`（`strategyQuality`）。
- 触发：上轮致命错误使账号以 0 分保底；旧错误刚超过当前证据有效期，但仍在评分历史内，新出现一条 502，旧成功仍参与历史评分。
- 实际：温和降权规则算出 `routing_health_score=0`、`evidence_pending=true`，展示健康分为 55.3408；权重计算却回退展示分，给出 81.4475 的正权重。
- 原因：用 `routingHealth==0` 同时表示合法零分和未初始化，破坏前面按上一轮调度分限制的结果。
- 修复方向：明确表示调度分是否存在，不能以数值零判断缺失。
- 复现：`TestReviewPendingZeroRoutingScoreDoesNotBecomePositiveQuality`。属于较窄的证据过期边界，但实际可达。

### 11. P2：关闭写后验证时，普通成功写入不触发冷却

- 位置：`backend/internal/business/routing.go:308`、`backend/internal/routing/service.go` 的 `applyDeadband`。
- 触发：`writeback.verification=false`（当前默认值），启用智能扩容并设置 600 秒冷却。
- 实际：第一轮成功增加并发后，审计为 `remote_confirmed=true / readback_confirmed=false`，`LastApplyAt` 仍为空；下一轮立刻再次增加并发，未受 600 秒冷却限制。
- 原因：最近成功写入查询只接受读回确认的记录，但关闭验证的普通成功写入不会生成该标记；调权与扩容冷却都依赖这一时间。
- 修复方向：分别保存成功写入时间与读回确认事实，普通成功写入仍启动冷却；恢复和容量安全所需的强制读回不能因此放宽。
- 复现：`TestReviewVerificationDisabledMustStillEnforceCooldown`。

## 验证与范围

- 审查前段的后端相关 9 个测试包（含显式 `__tests__` 包）执行 `go test -race` 通过；Go 复用了有效测试缓存。同一范围的 `go vet` 通过。并行改动后的末轮常规回归收到 SIGTERM、退出码 143，没有完整通过结果。
- 审查前段的前端现有相关回归 25 个文件、148 个用例通过，范围为 policy、groups、scheduling-display 和 group-policy-display；同期 `bun run typecheck` 通过。并行修改后的末轮类型检查收到 SIGTERM 而终止，没有获得该次完成结果，不据此前结果断言后续并行改动全部通过。
- 额外复现使用 Go overlay 和临时 Vitest 配置，不向正式代码目录写入失败测试。统一运行再次触发全部 11 项问题；Go 复现启用 `-race`，无数据竞争报告，前端交互在请求值断言处失败。复现断言失败是上述问题的证据，不表示修复完成。
- 补充正向验证包括：成功恢复探针替代熔断前流量、绑定恢复后仍检查新致命证据、健康恢复后取消待删除、普通客户端错误不触发仅凭据错误删除。
- 四类策略公式、价格精度、组内预算、多分组稳定主组、全局和共享上游容量、已确认缩容后再恢复、独立下调、人工保护、探测与恢复开关分离均已检查；未把未能确认的猜测列为问题。
- 审查期间有其他任务继续修改工作区，新增探活暂停时段及退管范围保护。相关修改已补读，上述缺陷的关键分支仍存在。暂停窗口的时区、跨午夜及自动/手动边界已走读，尚未把恢复探测、排队与重试跨入暂停时段的全部分支作为本报告的完整验证结论。
- 未运行生产 E2E、全仓后端测试或前端构建；本轮没有修改业务代码。审查不是对全部可能运行状态的正确性证明。

## 复现材料

同目录的 `scheduling-policy-review-2026-09-16-repros.tar.gz` 保存复现源代码、执行脚本与最新输出。解压到临时目录后执行：

```bash
bun <解压目录>/sub2api-inspection-review-ztJqCO/run-repros.mjs <仓库根目录>
```

脚本要求当前项目的 Go/Bun 工具链及已安装的前端依赖；仅增加临时 overlay、临时 Vitest 配置和测试自身的隔离资源，结束后清理生成的运行目录。当前代码下预期非零退出；修复后应将有关用例整理到对应模块的 `__tests__/`，再确认转为通过。

末轮普通测试和类型检查的中断不影响已完成的隔离复现结果；它们限制了对其他并行修改的验证范围。本任务启动的进程均已结束。
