# 账号与上游审查记录（2026-09-07）

状态：分配生产文件人工审查和确认缺陷修复已完成；10 个领域包 race 和 11 个相关包 vet 通过，根代理随后完成 business 与全仓 race，最终结果见主审查报告。这里只记录实际人工审查和验证，不以测试通过替代覆盖。

## 已完成的逐文件人工审查

- `backend/internal/upstreamdetect/service.go`：公开指纹、URL 校验、禁止跳转、限时/响应大小/业务标识、降级检测。
- `backend/internal/upstreamdelete/service.go`：预览稳定 ID、别名范围锁、保护状态、目标变更、并发删除/读回、私有记录及投影提交、任务取消。
- `backend/internal/upstreamconfig/service.go`：创建/更新、稳定 Host 迁移、鉴权复核、私有和公共存储补偿、任务排队、配置读回。
- `backend/internal/upstreamauth/client.go`：平台/模式校验、登录/刷新/协议交互、凭据作用域、HTTP 业务成功与响应边界。
- `backend/internal/accountdelete/service.go`：单个和批量预览、稳定 Key 绑定、管理目标指纹、锁后确认、双端删除顺序、错误读回/投影对账/审计。
- `backend/internal/adminclient/client.go`、`model_pricing.go`：所有目录/详情/修改接口、稳定 ID 读回、创建 marker、提交不确定性、分页、证据读取、定点价格、重试和响应校验。
- `backend/internal/upstreamsync/client.go`、`billing.go`、`service.go`：分组/Key/余额读取、Key 创建和对账、分页、稳定 Token 计费、并发同步/锁后复读、刷新/错误持久化、账号同步任务派生。
- `backend/internal/accountops/service.go`、`model_sync.go`、`pool_mode_sync.go`：字段同步、账号控制、人工优先位、模型发现和应用、池模式同步、排队和执行边界、取消和部分写入。
- `backend/internal/onboarding/service.go`、`key_cleanup.go`、`probe.go`：开户意图冻结和恢复、Key 对账/清理、批量处理、探测流、稳定绑定与投影。现有明确测试约定开户允许两种运行模式，不将此行为误判为缺陷。
- `backend/internal/authrecovery/service.go`、`captcha.go`：刷新、密码箱/手动登录、验证码、多任务批次持久化、并发保存和删除、平台纠正、成功方式偏好。
- 分配的 `business` 生产文件已全部逐文件读取：`account_block.go`、`account_state.go`、`account_control.go`、`account_model_sync.go`、`account_operations.go`、`account_maintenance.go`、`account_recovery.go`、`onboarding.go`、`auth_recovery.go`、`platform.go`、`billing_quota_unit.go`、`upstream_auth_seed.go`、`upstream_configuration.go`、`upstream_delete.go`、`upstream_identity.go`、`upstream_catalog_identity.go`、`upstream_group_binding_audit.go`、`upstream_group_history.go`、`upstream_sync.go`。覆盖稳定身份、开户候选/恢复记录、事务投影、账单定点、审计和目录生命周期。

## 已确认并修复

1. 账号删除在完全模式排队后，切换为监控模式仍会执行远程 Key、账号及本地投影删除。增加执行锁后的运行模式复核。
   - 先失败：`go test ./internal/accountdelete -run TestQueuedDeleteRejectsRestrictedMode -count=1`，观察到 `keys=[key-8] accounts=[37] local=true`。
2. 上游删除存在相同排队后运行模式变化缺口，即使无账号也会删除私有鉴权和上游投影。增加执行锁后的运行模式复核。
   - 先失败：`go test ./internal/upstreamdelete -run TestQueuedDeleteRejectsRestrictedMode -count=1`，观察到 `auth=1 projection=1`。
3. 修改账号 Base URL 的路径大小写会保存新地址，但全字符串 `EqualFold` 错误地认定 URL 未变，关联账号不会同步。URL 比较现在只折叠 scheme/host，保留路径大小写。
   - 先失败：`go test ./internal/upstreamconfig -run TestUpdateQueuesAccountSyncWhenOnlyBaseURLPathCaseChanges -count=1`，保存 `/team` 后同步任务仍为空。
   - 修复后三个完整包通过：`go test ./internal/accountdelete ./internal/upstreamdelete ./internal/upstreamconfig -count=1`。
4. 上游 Key 列表用请求的 `page_size=1000` 推算已读数量；上游把页尺寸压低时，第一页面就被错误当作完整目录。缺少列表字段、早期空页、重复页和变化的 total 也会被当作成功目录。改为按实际稳定 ID 数确认完整性，完整性无法确认时拒绝发布目录。
   - 先失败：`TestListKeysReadsAllPagesWhenUpstreamCapsPageSize` 返回 1 项而非 2 项；`TestListKeysRejectsIncompleteOrMalformedCatalog` 的五个场景均错误返回成功。
5. New API 稳定 Token ID 经 float64 转换，`9007199254740992` 和 `9007199254740993` 被合并，金额计入错误 Token。整数 JSON 现在直接以整数精度解析。
   - 先失败：`TestReadNewAPIKeyUsagePreservesLargeStableTokenIDs` 返回单项金额 3，正确结果应为两个 Token 分别 1 和 2。
   - 修复后 `go test ./internal/upstreamsync -count=1` 完整包通过。
6. 鉴权恢复远程复核期间用户修改配置，旧恢复结果会覆盖新 Base URL 和 Token。提交时在上游锁内比较首次凭据快照，发现更改或删除时拒绝旧结果；平台纠正也移到复核之后。
   - 先失败：`TestRecoveryDoesNotOverwriteCredentialsChangedDuringRemoteVerification` 观察到新配置被旧恢复覆盖。
   - 补充先失败：`TestRecoveryDoesNotRestoreAuthRecordRemovedDuringVerificationWhileUpstreamRemains` 在保留上游但删除私有鉴权时恢复了被删除的记录；提交现在同时检查初始存在状态。
7. 倍率字段夹带 Base URL/归属上游字段时绕过监控模式和人工优先位限制。保护条件现在包含这两个字段。
   - 先失败：`TestFieldSyncRejectsEndpointChangesAlongsideMultiplierUnderProtection` 四个场景均访问远端。
8. 取消人工优先位任务排队后不再检查运行模式。增加执行锁后的模式复核。
   - 先失败：`TestQueuedClearManualPriorityRejectsRuntimeModeChangeBeforeRemoteAccess` 在监控模式仍访问管理 API。
9. 上游并发删除在 worker 收到任务但尚未执行时取消，未执行项错误为 nil，被计作成功。取消分支现在记录该项取消错误。
   - 先失败：`TestDeleteAccountsMarksDispatchedButCancelledAccountAsFailed` 结果为 `[<nil> context canceled]`。
   - 修复后 `go test ./internal/upstreamdelete ./internal/accountops -count=1` 完整包通过。
10. 批量鉴权恢复成功轮换 Token 后，公共投影持久化失败会恢复可能已失效的旧 refresh token，并可能覆盖后来配置。投影失败现在保留已复核凭据、返回投影错误和已完成结果。
    - 先失败：`TestBatchRecoveryPreservesRotatedCredentialsWhenProjectionPersistenceFails` 观察到新 Token 被旧 Token 替换。
    - 修复后 `go test ./internal/authrecovery -count=1` 完整包通过。
11. 旧验证码挑战会覆盖后来更新、删除或新建的鉴权配置/密码箱项。准备时分别记录持久化快照，提交持有对应租约时验证；允许未变更配置下使用手动登录候选。
    - 子审查先失败复现：`captcha_conflict_test.go` 的 4 个鉴权冲突和 3 个密码箱冲突；修复后 `go test -race ./internal/authrecovery -count=1`、`go vet ./internal/authrecovery` 通过。
12. Host 迁移在私有写失败或后续提交失败时，使用已取消的请求 context 补偿且隐藏回滚失败。现在用有超时的独立 context 补偿，并合并返回失败原因。
    - 先失败：`TestHostRenameCompensatesPublicIdentityAfterRequestCancellation` 观察到旧 Host 未恢复，以及回滚失败被隐藏。
13. 鉴权投影把不存在的 Host 自动添加/去除 `www.`，修改无关上游；真实稳定别名反而未匹配。现在从稳定身份表解析显式别名。
    - 先失败：`TestAuthRecoveryProjectionDoesNotGuessWWWIdentity` 将无关上游从已鉴权改为失效；`TestAuthRecoveryProjectionResolvesExplicitStableAlias` 未更新真实同身份上游。
14. 上游配置 GET、Create、Update 响应暴露自定义 Header 的 Token/API Key。响应保留 `headers: {}` 兼容字段，只返回 `header_names`；请求省略 headers 保留原值，显式 `{}` 清空，非空 map 全量替换。
    - 先失败：`TestConfigurationResponseExposesHeaderNamesWithoutCredentialValues` 发现两个自定义凭据值进入响应。
    - `TestConfigurationUpdatePreservesHeadersWhenOmittedAndClearsExplicitEmptyMap` 保护不变/清空更新语义；完整 upstreamconfig 包通过。
15. 管理平台批量倍率探测接受超过 int64 的账号 ID，忽略 ParseInt 错误并把 ID 截到 MaxInt64，可能请求错误账号。现在在整批请求前拒绝不能表示的 ID。
    - 先失败：`TestBatchMultiplierProbeRejectsIDThatCannotFitRequestInteger` 观察到非法 ID 请求远端且返回成功。
16. 管理账号/分组目录把缺失、null、负数 ID 当成有效稳定身份（nil 经 fmt.Sprint 变成非空字符串）。现在分页目录和旧版分组备用接口均验证稳定数字 ID。
    - 先失败：`TestManagementCatalogRejectsEntriesWithoutStableNumericIDs` 三个接口的九个无效身份场景均被接受。
    - 完整 adminclient 包通过。
17. 按 Key 局部同步遇到分组名称与目标分组 ID 相同时，先按名称取了错误分组。现在优先扫描稳定 ID，仅无 ID 匹配才使用精确名称后备。
    - 先失败：`TestPartialKeySyncPrefersStableGroupIDOverCollidingName` 保存了 `wrong-group`。
18. 模型应用远程写成功但读回失败时 `remote_write=false`；任务取消会丢失已完成项和写入证据。现在项目记录写入状态，任务聚合保留部分结果。
    - 先失败：`TestModelApplyReportsRemoteWriteWhenReadbackDoesNotMatch` 和 `TestModelTaskCancellationRetainsCompletedItemsAndRemoteWriteEvidence`。
19. 人工优先位预留后、远程写入前取消，会使用已取消 context 回滚并留下错误本地预留。现在补偿使用独立有限 context。
    - 先失败：`TestManualPriorityCancellationBeforeRemoteWriteRestoresLocalReservation` 留下 1 条无远程对应变更的预留。
20. 仅修改 Base URL 被空字段检查拒绝。将 Base URL/归属上游纳入字段存在性检查，同时保留监控和人工保护限制。
    - 先失败：`TestFieldSyncAllowsStandaloneBaseURLChangeInFullMode` 返回“至少提供一个需要同步的账号字段”；修复后完成写入和本地投影。
21. 批量池模式同步未遵守人工优先位保护。执行锁后检测人工优先位，作为跳过项记录并拒绝远程访问。
    - 先失败：`TestPoolModeSyncSkipsManuallyProtectedAccountBeforeRemoteAccess` 访问受保护账号的远程 API。

## 跨范围协调

- 根代理负责前端 Header 编辑契约同步：输入框不回填保存的 Header 值；以 header_names 展示已配置状态；未修改时不传 headers，修改输入为完整替换，显式关闭/清空为 `{}`。后端无需新增 parser 字段。
- 根代理确认并负责 management 包中 Base URL 自动同步人工保护/执行模式等同类问题，本审查没有改动该包。
- 开户允许两种运行模式、模型发现仅同步可用目录为当前显式功能语义；没有将这些路径等同于模型应用/删除等拓扑改动。

## 验证

上述缺陷均先看到失败回归，再实施修复；涉及测试仅使用临时 SQLite、httptest 和隔离凭据。所有受影响回归已看到最新普通测试通过；business 执行相关 AuthRecovery、PartialKeySync、ApplyUpstreamSync 和 CatalogSync 测试集，领域包执行完整包测试。

- `go vet ./internal/accountops ./internal/accountdelete ./internal/onboarding ./internal/adminclient ./internal/upstreamauth ./internal/upstreamsync ./internal/upstreamdetect ./internal/upstreamconfig ./internal/upstreamdelete ./internal/authrecovery ./internal/business` 通过。
- 相同 11 包的早期 `go test -race ... -count=1` 输出 10 个领域包通过；当时交接中的 business 已在最终全仓检查中通过。
- 最后的池模式人工保护修改补跑 `go test -race ./internal/accountops -run PoolMode -count=1` 和 `go vet ./internal/accountops` 均通过。
- 涉及文件 `git diff --check` 通过，已运行 gofmt。
- 根代理已完成 business race 和最终全仓 `go test -race ./...`，见 `global-review-2026-09-07.md` 的当前验证表。
