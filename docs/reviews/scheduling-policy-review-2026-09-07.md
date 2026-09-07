# 调度策略审查记录（2026-09-07）

> 本文保留修复前的审查与失败证据。10 项确定问题的后续修复、正式回归及状态见 [调度专项问题闭环](scheduling-policy-fixes-2026-09-07.md)；最终统一验证见 [全局审查记录](global-review-2026-09-07.md)。

基于当前工作区文件审查，包含已有未提交改动。确认 **10 项问题：4 项 P1、6 项 P2**。本次没有修改业务实现，也没有访问生产数据库、真实上游或通知渠道。

P1 表示可能误处置账号、撤销人工保护或显著阻碍服务恢复，应优先修复；P2 表示在特定配置或状态下产生错误调度结果。

审查覆盖 `routing` 的评分、证据选择、状态机、多分组归属、成本墙、权重、并发和自动处置；`evidence` 的采集与恢复探测；`inspection` 的运行模式、周期、租约、取消及计算/写回衔接；`routingwrite` 的人工保护、基线、字段开关、读回和删除；相关 `business` 持久化与策略校验，以及前端策略编辑、调度展示和恢复提示。本结论针对代码行为，未核验线上实际配置或流量分布。

## 已确认的问题

### 1. [P1] 普通客户端 403 会绕过“仅处置凭据失效”并进入删除队列

- 位置：[service.go:1654](/root/workspace/sub2api-console/backend/internal/routing/service.go:1654)。
- 当 `cleanup.trigger_status_codes` 非空时，计数分支只判断状态码，后面的 `cleanupOnlyAuth` 分支完全不执行。默认触发码包含 401、403，但评分器把没有明确鉴权失效证据的真实流量 403 视为中性客户端错误。
- 复现：两个账号的分组，开启自动删除、保留最后账号和 `only_auth_errors=true`；账号 41 最近三条为 `403 / model access denied`，观察期已满足。健康评估有效样本数为 0、调度状态仍为 healthy，目标却包含 `cleanup_action=delete`。
- 影响：有效凭据可能因为模型访问权限、客户端请求问题被自动暂停、停用或删除。写回的删除分支只校验保护状态和远端对象，不会再次检查错误分类。
- 建议：处置复用统一分类器，让错误码过滤与“仅凭据失效”成为同时满足的条件；中性客户端错误不得进入凭据处置计数。
- 用例：`TestReviewNeutralClientErrorsCannotTriggerCredentialOnlyDeletion`。

### 2. [P1] 同一轮已判定恢复健康，仍会按旧错误删除账号

- 位置：[service.go:1585](/root/workspace/sub2api-console/backend/internal/routing/service.go:1585)、[service.go:1684](/root/workspace/sub2api-console/backend/internal/routing/service.go:1684)。
- 自动处置仅检查窗口命中数、观察起点和少数保护状态，不排除本轮已恢复的 healthy 账号，也不会在恢复时清除既有观察起点。
- 复现：合法配置 `scoring.short_window=2`，处置窗口 5 条、401 命中阈值 3 次、观察期 30 分钟。原熔断账号已有 40 分钟的观察记录，最新两条流量成功、此前三条 401。引擎计算健康分 82、连续成功满足恢复条件、`schedulable=true`，随后仍生成删除目标。
- 影响：健康判定和处置目标互相矛盾；[写回删除分支](/root/workspace/sub2api-console/backend/internal/routingwrite/service.go:588)会直接执行该目标，甚至先把已接流量的账号摘除。
- 建议：明确恢复成功时撤销处置观察和待执行目标；删除执行前再次验证当前状态及处置依据。
- 用例：`TestReviewRecoveredAccountCancelsPendingAutomaticDeletion`。

### 3. [P1] 熔断恢复探测被旧失败流量覆盖，默认可能等待约两小时

- 位置：[service.go:2082](/root/workspace/sub2api-console/backend/internal/routing/service.go:2082)。
- 只要流量回溯窗口中还存在一条样本，就完全使用流量作为健康证据。成功恢复探测没有覆盖入口；`withCriticalProbeEvidence` 只允许更新的致命失败探测覆盖。
- 复现：已生效熔断的账号，10 分钟前流量 401，4 分钟前及 1 分钟前主动探测均成功。结果仍为 fused、健康分 0、连续成功 0，显示凭据仍失效。
- 影响：采集器会按恢复周期继续付费探测，但这些成功结果不进入恢复判定；在没有新流量时，要等最后一条失败流量超过 `traffic.lookback_minutes`，默认 120 分钟。普通较新探测失败也存在被旧成功流量遮蔽的问题。
- 建议：为已生效熔断账号选择熔断后恢复证据；区分流量历史回溯与当前健康证据新鲜期。性能统计可以继续使用历史真实流量。
- 用例：`TestReviewSuccessfulRecoveryProbesOverridePreFuseTraffic`。

### 4. [P1] 策略页面保存旧草稿会覆盖其他页面刚更新的人工保护

- 位置：[App.tsx:11893](/root/workspace/sub2api-console/frontend/src/App.tsx:11893)、[App.tsx:1855](/root/workspace/sub2api-console/frontend/src/App.tsx:1855)。
- 页面第一次加载后始终保留原 draft；30 秒刷新得到的新策略不会更新未编辑字段。保存时提交整个 draft，只有 mode 被移除。后端 [policy.go:215](/root/workspace/sub2api-console/backend/internal/business/policy.go:215)会据传入数据替换高级分区字段，且无版本冲突检查。
- 复现：页面最初加载 `paused_account_ids=[]`，后台策略更新为 `["41"]`，前端真实 Query 刷新已拿到新值；点击保存，实际 PUT 请求仍带空暂停列表。测试使用真实组件、Query 和 request，仅 mock 网络边界。
- 影响：保存无关策略时可能覆盖新增的暂停、排除、人工熔断或其他策略修改。列表是否立即改变远端接流量状态，还取决于对应账号的其他生效状态；人工保护配置丢失本身已确认。
- 建议：保留最新服务端基线，提交用户实际修改的字段；为策略引入版本校验或明确的并发冲突处理。后端高级分区目前采用替换语义，不能只改前端为随意省略字段。
- 用例：`policy_review.test.tsx` 中“其他页面新增暂停账号后保存未修改的守护范围，应保留新的暂停保护”。

### 5. [P2] 全局并发上限实际上被每个主分组分别使用

- 位置：[service.go:1334](/root/workspace/sub2api-console/backend/internal/routing/service.go:1334)、[service.go:1451](/root/workspace/sub2api-console/backend/internal/routing/service.go:1451)。
- 每个主分组单独调用 `applyScaling(owned, config)`，独立计算 allocated 和 headroom，全局账号总额未被扣除。
- 复现：两个不同主分组各有一个并发 45 的健康账号，全局上限 100、扩容触发比例 0.4、步长 10。本轮两个目标都变为 55，总额由 90 增至 110。
- 影响：启用扩容及并发自动写回后，可在多分组下突破全局限制。默认扩容和并发自动写回均关闭。
- 建议：以账号稳定 ID 去重后统一预算，分组只决定账号的扩容资格，最后统一分配剩余额度。
- 用例：`TestReviewGlobalConcurrencyBudgetSharedAcrossPrimaryGroups`。

### 6. [P2] 单账号并发下限可以再次突破剩余预算

- 位置：[service.go:1477](/root/workspace/sub2api-console/backend/internal/routing/service.go:1477)。
- 步长增加虽然限制为 headroom，但随后的最小值 clamp 不受 headroom 约束，只会事后扣减，可能把预算扣成负数。
- 复现：同组两个账号当前并发均为 1，全局上限 5，单账号下限 3。即使未达到扩容触发比例，两个目标仍变为 3，总额达到 6。
- 影响：即便修正跨分组统计，单组仍能超限；账号数量与下限组合不可满足时没有明确报错或降级策略。
- 建议：把下限补齐纳入统一预算，并显式处理 `账号数量 × 下限 > 总上限` 的不可满足约束。
- 用例：`TestReviewMinimumConcurrencyMustRespectRemainingGlobalBudget`。

### 7. [P2] 扩容触发比例使用配置容量，未衡量实际负载

- 位置：[service.go:1458](/root/workspace/sub2api-console/backend/internal/routing/service.go:1458)；前端 [App.tsx:12971](/root/workspace/sub2api-console/frontend/src/App.tsx:12971)描述为“负载率达到阈值”。
- 公式是 `已配置并发总额 / 全局并发上限`。当前并发使用量、排队和请求压力均未参与判断，RoutingAccount 也没有对应的实际使用量字段。
- 复现：健康账号仅配置并发 80，上限 100、触发比例 0.8、步长 5，没有任何当前负载输入仍得到 85 的目标。
- 影响：接近配置上限会在冷却后持续扩容；配置容量低于触发比例时，即使实际已饱和，也无法凭真实压力触发扩容。
- 建议：接入可核验的实际利用率/排队证据；无法获得时应明确为静态容量分配策略，并调整产品描述。
- 用例：`TestReviewIdleHealthyAccountDoesNotScaleFromConfiguredCapacityAlone`。该用例证明扩容无需负载证据，不代表读取过线上闲置指标。

### 8. [P2] 合法的零倍率被按倍率 1 评分

- 位置：[service.go:1952](/root/workspace/sub2api-console/backend/internal/routing/service.go:1952)。
- `resolveRate` 接受非负倍率，零是已知有效值；但策略评分仅在 `Sign()>0` 时使用它，零倍率落回默认 1。
- 复现：健康和速度相同的账号，倍率分别为 0、0.1，价格优先策略、权重预算 400。免费账号只得到约 87.5，收费账号得到 312.5。
- 影响：免费资源被错误排在付费资源之后。
- 建议：显式定义零成本的最高价格得分，区分缺失倍率与零倍率，并避免用除零近似制造无穷值。
- 用例：`TestReviewPriceFirstPrefersFreeAccountAtEqualHealthAndSpeed`。

### 9. [P2] 合法价格指数会溢出，导致价格排序退化为平均分配

- 位置：[service.go:1967](/root/workspace/sub2api-console/backend/internal/routing/service.go:1967)、[service.go:1988](/root/workspace/sub2api-console/backend/internal/routing/service.go:1988)。
- 配置允许价格指数最大 100，先对价格倒数幂运算再相对归一化。倍率 0.0001 的 `(1/rate)^100` 溢出为 Inf，最佳项 `Inf/Inf` 为 NaN，质量和随之变成 NaN，最终走零质量的平均预算分支。
- 复现：倍率 0.0001 和 0.01、指数 100，其余条件相同，结果权重都为 200。
- 影响：提高价格敏感度反而让价格差异失效，且没有配置错误提示。
- 建议：先归一化到有界比例再做幂运算，或在对数域比较；检查所有评分中间值为有限数。
- 用例：`TestReviewPermittedLargePriceExponentPreservesPriceOrdering`。

### 10. [P2] 绑定恢复直接回池，跳过同轮致命健康错误

- 位置：[service.go:864](/root/workspace/sub2api-console/backend/internal/routing/service.go:864)。
- `binding_invalid → active` 分支直接设置 healthy 和 schedulable=true，位于后续凭据致命错误判断之前。目录重新出现只证明对象存在，不能证明凭据可用。
- 复现：账号此前 binding_invalid、当前目录绑定 active、最新健康结果 Fatal=true 且健康分 0，仍输出 healthy、允许调度。
- 影响：重新出现但凭据仍错误的账号会至少被错误回池一轮；后续才重新触发熔断。
- 建议：绑定恢复只解除绑定限制，然后继续执行当轮健康、上游阻断和熔断规则。
- 用例：`TestReviewRestoredBindingStillChecksNewFatalEvidence`。

## 已观察到但未计入确定缺陷的约定

- **在途策略变更**：等待写回租约时关闭 priority 自动写入，旧批次仍发生一次写入。页面明确提示“用于下一轮调度”，因此未把缺少即时中断能力单独算作缺陷。若产品希望开关是紧急停止，应另行增加执行前复核和取消机制。复现用例 `TestReviewDisablingPriorityWriteWhileAcquiringLeasePreventsMutation` 的失败是对该更强契约的验证。
- **排除账号与交还基线**：引擎为 excluded 账号生成 ReleaseControl，但写回的人工保护会跳过它，基线保持存在；全局恢复也保护这些账号。用例 `TestReviewExcludedAccountCanReleasePreviouslyCapturedControl` 已记录结果。应明确排除是保留当前值并停止管理，还是恢复接管前基线；本次未把后一种约定强行视为既定需求。
- **保底优先于致命熔断**：现有测试明确要求最后账号即使凭据致命失效也保底强留。这是当前策略取舍，不是本次发现的实现回归；保底数量不能等同于真实可用容量。
- **多分组共享账号参数**：主分组决定最终状态和账号字段，各组权重取平均；任一受管分组在成本墙内就可能保留账号。这是当前明确的账号级归属模型，不能理解为每个分组都有独立的账号调度字段。
- README 仍提及“调度模式”，运行时代码实际仅有监控、完全两种模式；建议同步文档，未计入调度算法缺陷。

## 验证与复现材料

- 现有后端相关测试通过：`go test -race ./internal/routing ./internal/routingwrite ./internal/inspection ./internal/evidence ./internal/business`。各包分别约 2.319s、28.755s、12.256s、1.077s、347.353s。
- 现有前端策略页面、调度展示、分组策略展示、人工优先位和恢复状态共 5 个文件、37 项测试全部通过。
- `bun run typecheck` 通过。
- 临时 Go overlay 新增 11 个隔离用例：9 个对应上述确定问题、2 个用于核实产品约定；临时前端用例 1 个对应旧草稿覆盖。最新运行均出现预期失败，Go 复现也启用了 race。测试失败代表问题仍存在，并非已经修复。
- Go 用例复用既有测试中的存储/远端边界 fixture；写回用例使用临时 SQLite 和内存 Admin；前端使用真实组件、真实 Query、统一 request，mock Fetch 边界。未触达真实业务渠道。
- 未改动生产 Go/TS/TSX 文件，没有执行全仓库 `go test -race ./...`、生产构建或浏览器 E2E；上述通过结果仅覆盖列明范围。

复现材料：[scheduling-policy-review-2026-09-07-repros.zip](/root/workspace/sub2api-console/docs/reviews/scheduling-policy-review-2026-09-07-repros.zip)。包含测试源、overlay、Vitest 配置、失败输出及运行说明；当前解压目录为 `/tmp/sub2api-scheduling-review-20260907`。

建议先修复处置资格、恢复证据和旧草稿覆盖这四项 P1；再处理两处并发预算、扩容信号与价格评分，最后补上绑定恢复的状态机回归。应先将相应失败用例转为正式模块测试，再实现修复。
