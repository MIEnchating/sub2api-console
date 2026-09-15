# 后端边界与上游凭据审查素材

本分工完整阅读后端边界 315 个文件，以及上游、密码箱前端 75 个文件。记录位于 `backend-boundaries.tsv` 和 `frontend-upstreams.tsv`；后端记录另含与领域分工交叉审查的 `business/routing_write.go`，共 316 行。所有文件均按最终内容计算 SHA256，最后核对本分工没有未审文件或哈希变化。

## 后端修复

| 已确认问题 | 修复与回归文件 |
| --- | --- |
| 官网 Token 分档阈值含非法数值时可能空指针 panic，分数边界也被接受 | `officialpricing/tiers.go` 在运算前验证有界十进制和整数阈值；GLM、MiniMax、Qwen 解析器使用同一检查。`officialpricing/__tests__/token_conditions_test.go`。 |
| 以日期开头的纯文本邮件被误当成 JSON 数字；每个验证码重新扫描所有时间戳造成平方级开销 | `workbenchprovider/mail_parse.go` 区分文本与结构化响应，只解析一次时间戳，通过二分查找匹配最近的有效时间。`workbenchprovider/__tests__/mail_timestamp_test.go`。 |
| 倍率探测错误直接返回上游凭据，截断后再脱敏也可能遗漏 | `adminclient/client.go` 各失败分支先脱敏，再限制诊断长度。`adminclient/__tests__/probe_errors_test.go`。 |
| Kuma 方法及 DNS 类型使用字符串包含判断，接受 `GET|POST` 等组合 | `uptimekuma/monitor_options.go` 改为完整枚举匹配。`uptimekuma/__tests__/option_validation_test.go`。 |
| 批量鉴权恢复反复写入前缀结果，既增加写入量又覆盖较新的 Host 状态 | `authrecovery/service.go` 记录完整批次快照，但仅更新本次完成 Host；领域分工实现对应持久化接口。`authrecovery/__tests__/batch_progress_test.go`。本项与领域报告相同，不重复计数。 |
| 管理接口和上游余额接受分数、十六进制、下划线或无界指数 | `adminclient/client.go`、`upstreamsync/client.go` 复用有界十进制解析，拒绝不符合协议的数值。两个模块的 `__tests__/decimal_validation_test.go`。 |
| New API 无效远端价格覆盖可用缓存、等价十进制误判为不同、上下文阈值转换溢出 | `newapimanagement/service.go` 校验完整目录后替换缓存，按精确十进制比较，并检查整数阈值范围。`newapimanagement/__tests__/decimal-catalog_test.go`。 |
| 取消行为检测后读取已关闭结果通道形成忙循环 | `modelcheck/service.go` 正确处理关闭通道，取消后任务可结束。`modelcheck/__tests__/cancellation_test.go`。 |
| 行为检测将 HTTP 200 业务错误或不完整响应当成成功，返回模型字段还可能包含凭据 | `modelcheck/direct.go` 检查各协议实际成功与完整性，并对返回模型元数据脱敏。`modelcheck/__tests__/direct-response_test.go`。 |
| 工作台接口把任意非空 Cookie 与管理 Bearer 组合当成已登录会话 | `api/account_workbench.go` 查询实际登录会话并拒绝伪造、过期或已注销 Cookie。`api/__tests__/workbench_session_test.go`。 |
| 字典排序和浏览器输入请求未采用严格 JSON 校验 | 对应 API handler 使用现有严格解析入口，拒绝尾部 JSON、未知字段及错误内容类型。`api/__tests__/dictionary_requests_test.go`、`browser_requests_test.go`。 |
| 短信订单列表及核对没有按工作台使用范围隔离 | API 向领域服务传递并校验明确 scope，前端由主分工同步适配。`api/__tests__/account_workbench_sms_receipts_test.go`。 |
| 每条流量样本重新解析相同评分策略 | `routing/scoring.go` 创建一次批量分类器，响应映射复用已验证策略。`routing/__tests__/classification_batch_test.go`。 |
| 数字分组名称可冒充策略中的稳定分组 ID，导致错误接管或释放 | `routing/service.go` 仅按分组 ID 匹配范围；`1`、`1.00`、`1e0` 的倍率按精确数值比较，避免无变化时判为外部修改。`routing/__tests__/control_scope_test.go`。 |
| 恢复多个账号调度基线时，每个账号读取完整基线表 | `business/routing_write.go` 提供按账号索引查询，`routingwrite/service.go` 优先使用单账号读取。`routingwrite/__tests__/baseline_lookup_test.go` 验证两个账号恢复不再执行两次全表读取。 |
| 删除活动 OAuth 检查点后浏览器仍接受输入，并可能重建已撤销状态 | `browserlogin/oauth_checkpoint_worker.go` 校验活动 ID、owner、lease，在删除前冻结对应浏览器会话。真实隔离 Chromium 回归 `browserlogin/__tests__/oauth_checkpoint_revocation_test.go`。 |
| 清空负载倍率后，回读省略字段或空值也被当成远端已清空 | `management/service.go` 要求回读明确包含 JSON `null`；缺失、空响应及未变化值均失败，保留本地状态。`management/__tests__/defaults_readback_test.go`。 |

上述路径均相对于 `backend/internal/`。错误修复先通过回归确认失败，再修改并验证通过；性能优化通过基准或实际查询行为验证。

## 前端修复

| 已确认问题 | 修复与回归文件 |
| --- | --- |
| 上游 Token、密码箱密码作为 mutation 参数残留在 React Query 共享缓存 | 两个编辑器只在组件提交内存中持有敏感载荷，发起请求时立即取出并清空；mutation 不携带秘密参数，使用 `gcTime: 0`。`upstreams/components/__tests__/editor-credential-lifecycle.test.tsx` 和 `vault/components/__tests__/credential-cache.test.tsx` 分别覆盖成功与失败请求。 |
| 上游后台刷新余额会重置正在编辑的地址和 Token | `upstream-edit-dialog.tsx` 按 Host 初始化，仅在当前表单无修改时接受后台数据。上述编辑器回归验证新余额显示且草稿保留。 |
| 保存上游 A 后切换 B，A 的迟到响应会修改 B 的缓存、表单或关闭 B 弹窗 | 保存结果绑定提交时的 Host；旧响应更新其原查询，对当前编辑器不执行重置与关闭。上述编辑器回归通过受控网络响应验证。 |
| 首次无绑定 Key 扫描占用任务等待状态并禁用关闭 | `onboarding-key-cleanup-dialog.tsx` 使用 `ContentLoading`，扫描期间保留关闭入口；真实删除任务仍保持操作保护。`key-cleanup-refresh.test.tsx` 验证可访问状态、无虚构进度和键盘关闭。 |
| 密码箱取消按钮仅关闭弹窗，未立即清除组件中的密码等草稿 | `vault-page.tsx` 统一关闭路径，清除表单、覆盖确认载荷及提交引用。 |

上述路径相对于 `frontend/src/features/`。另外完善既有密码箱测试的组件和 QueryClient 清理，对懒加载 JSON 编辑器使用可见状态等待；没有添加固定休眠或替代业务逻辑的 mock。

## 性能证据

| 隔离 fixture | 修改前 | 修改后 |
| --- | --- | --- |
| 一封含 1000 个带时间验证码的邮件历史 | 2.245 s/op，约 152 MB/op，约 300 万次分配 | 39 ms/op，约 2.11 MB/op，约 1.1 万次分配 |
| 按同一策略分类 1000 条样本 | 7.333 ms/op，约 1.67 MB/op，15,004 次分配 | 2.232 ms/op，约 91 KB/op，3,013 次分配 |

基准使用显式 fixture，未访问外部服务；共享机器的耗时存在波动，分配量变化也支持对应优化。重跑当前版本：

```bash
go test ./internal/workbenchprovider/__tests__ -run '^$' -bench '^BenchmarkTimestampedMailHistory$' -benchmem
go test ./internal/routing/__tests__ -run '^$' -bench '^BenchmarkSampleClassificationBatch$' -benchmem
```

## 验证记录

- 本分工后端修改均已运行受影响普通包及 `__tests__` 的 race 检查。最后追加的 routing、routingwrite、management 三个模块的两类包全部通过。
- 浏览器检查点删除缺陷先在真实 Chromium 中复现，再修复通过；网络命名空间内的 11 个 OAuth 检查点/自动化集成用例全部通过，未连接真实 OAuth 站点。`browserlogin` 完整 race 检查通过。
- 上游和密码箱广泛回归曾完成 48 个文件，其中 47 个文件通过，剩余为既有懒加载编辑器超时；完成等待与清理调整后，密码箱全模块及 Key 刷新共 5 个文件 16 个用例通过。
- 最终上游编辑器、地址、Headers、Key 弹窗的 4 个文件 14 个用例通过；随后 Key 读取/刷新最终 2 个用例再次通过。上述运行有重叠，不相加为总用例数。
- 最终 `bun run typecheck`、7 个改动 TS/TSX 文件的 ESLint 和 oxfmt 检查通过。
- 整仓最终 Go 脚本、前端完整测试、生产构建和其他并行变更由主报告记录；本记录不把执行中的任务视为完成。

## 并行增量复核

收尾期间完整复审新增的 OAuth 授权来源安全操作，以及浏览器检查点转安全会话的接口、私有状态恢复、原始到期时间、稳定身份确认、任务与产物边界。工作台来源相关 11 个文件另记录到 `root.tsv`；后续工作台检查点领域增量由主分工继续复审。本段属于对并行开发结果的复核，不计入上表本分工缺陷修复数量。

- 来源操作复核覆盖控制台 owner、有效授权、scope、用户 ID、邮箱、工作区、目标指纹和取消锁；受影响来源/API race 分别用时 8.565s 和 3.663s，通过。
- 追加 `browserlogin/__tests__/security_checkpoint_identity_test.go`，用真实 Chromium 验证用户确认身份后，密码重新验证请求仍携带已确认邮箱。编译前并行实现已经补上该邮箱回退，本用例首次执行即通过，不作为失败到通过的缺陷修复证据。
- 既有自动检查点用例曾因隐式 autofocus 时机未提交验证码；独立运行通过后，改为先读取画面并明确点击输入框，再执行文本输入，消除测试对自动聚焦时机的依赖。
- 最终使用 race 编译的 26 个检查点及安全操作用例在独立网络命名空间中全部通过，包括真实 Chromium 的状态迁移、密码/TOTP、请求防重放、截图隔离及 OAuth 恢复。没有真实外部网络访问。
- 最新完整 `go test -race ./internal/browserlogin/__tests__ -count=1 -timeout=120s` 通过，用时 3.964s；该默认入口之外的真实浏览器用例已由上一项显式启用运行。
- 新改动 Go 测试的 gofmt 检查及浏览器专项测试文件的 vet 均通过。最终浏览器边界文件哈希全部匹配。
