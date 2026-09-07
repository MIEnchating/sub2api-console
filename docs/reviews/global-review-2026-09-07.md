# 全局审查记录

状态：本轮全局审查与修复完成。全部分配范围已完成人工审查，已确认缺陷已修复，最终前后端统一检查通过。保留工作区已有及并行变更，未提交或部署。

## 范围与完成条件

- 以当前工作区为准，保留已有及同时进行的改动；此前修复不替代本轮覆盖。
- 覆盖所有人工维护的后端 Go 包、前端功能与共享模块、API 契约、启动和部署配置、CI 与测试入口。
- 生成的路由树、锁文件和二进制资源检查其生成、依赖和加载边界；不将第三方依赖源码列为逐行审查范围。
- 各模块检查主要流程、失败与取消路径、并发和事务、输入与身份校验、敏感信息边界，以及前端状态和可访问性。
- 缺陷先用失败用例复现，再修复；按影响范围运行回归测试，最后运行全量检查。
- 所有覆盖项完成、已确认缺陷修复并验证后，才宣布本轮完成。尚未审查或无法验证的内容必须明确列出。

## 覆盖清单

当前逐文件索引：[123 个后端生产文件、207 个前端生产文件及工程入口](global-review-coverage-2026-09-07.md)。已与最终验证快照核对，当前源文件没有后续变化。

| 范围 | 包与目录 | 状态 | 审查证据与后续工作 |
| --- | --- | --- | --- |
| 启动、部署、CI | `backend/cmd/server`、`config`、Dockerfile、entrypoint、Compose、Nginx、`.github`、`.githooks` | 人工审查完成，门禁回归通过 | 已读启动与关闭、代理 socket 身份/权限、全部发布阶段、镜像和代理配置；两种镜像构建及隔离代理集成通过，工作流脚本 36/36 通过 |
| 存储与基础安全 | `configstore`、`sqliteutil`、`taskstore`、`redact`、`naming`、`runtimepolicy` | 人工审查完成 | 已完整读取生产文件；修复数据库路径、敏感值脱敏、任务时间排序及清理边界，见下文 |
| 并发与任务基础设施 | `taskrunner`、`taskcontext`、`mutationguard`、`targetguard` | 人工审查完成 | 完整读取 runner/feed/context、租约续期与过期看门狗、嵌套互斥、目标快照；同时核对 business/mutation_lease.go、mutation_protection.go |
| 上游适配 | `adminclient`、`upstreamauth`、`upstreamsync`、`upstreamdetect` | 人工审查完成 | 身份、响应契约、重试、分页、重定向、计费精度 |
| 账号及开户 | `accountops`、`accountdelete`、`onboarding`、对应 `business` 文件 | 人工审查完成 | 确认范围、人工保护、写回与读回、回滚、幂等 |
| 上游配置及鉴权恢复 | `upstreamconfig`、`upstreamdelete`、`authrecovery`、对应 `business` 文件 | 人工审查完成 | 稳定身份、凭据迁移、验证码、恢复与清理 |
| 证据、探活与模型检测 | `evidence`、`opstraffic`、`probe`、`modelcheck`、对应 `business` 文件 | 人工审查完成 | 采样、错误分类、请求构造、健康结论与结果落库 |
| 巡检、调度及调权 | `inspection`、`routing`、`routingwrite`、对应 `business` 文件 | 人工审查完成 | 策略、人工控制、调度计算、并发写回和取消 |
| 告警、通知与运维日志 | `alerting`、`notification`、`notificationtarget`、`logs`、`systeminfo`、对应 `business` 文件 | 人工审查完成 | 状态迁移、发送确认、重试去重、脱敏、清理边界 |
| 管理同步、定价及报表 | `management`、`newapimanagement`、`pricing`、对应 `business` 文件 | 人工审查完成 | 同步完整性、金额精度、缓存、写入一致性、时间窗口 |
| 领域存储完整性 | `business` 全部文件与 schema | 人工审查完成 | 在上面业务审查之外，核查外键、删除关系、事务、分页和查询边界 |
| API 全部路由 | `backend/internal/api`、`frontend/src/api.ts` | 前后端人工审查完成 | 已完整读取 server.go 与 login_throttle.go，逐组检查请求/权限/稳定 ID/任务/SSE/错误映射；Header 契约已适配；改密并发会话签发缺陷已修复并通过全仓 race |
| 前端基础与共享 UI | `src/lib`、`hooks`、`components`、主题和构建配置 | 人工审查完成 | 已完整阅读全部共享文件、主题和配置；共享组件、任务终态、导航、存储与对比度修复见 frontend-shared-review 报告 |
| 前端全部功能及路由 | `src/features`、`App.tsx`、`app-routes`、`routes`、根应用 | 人工审查完成 | 全部 feature、App 原始 1–末尾分段与路由均读完；修复加载保护、任务失败/取消、鉴权焦点和字段关联、策略并发等，见分段报告 |
| 跨模块验证 | Go、Vitest、Playwright、typecheck、lint、Knip、format、build、部署测试 | 全部通过 | 最新结果记录在下方统一验证表，覆盖最终集成后的代码 |

## 此前修复

以下是上一轮局部审查的结果，仅作为本轮复核输入：

1. SSE 在后续推送和心跳时重新校验会话。
2. New API 价格写入出现不确定结果时，将当前字段纳入回滚。
3. 主动退出与会话过期统一清理业务查询和变更缓存。
4. 登录表单为账号与密码关联可访问标签。

## 本轮发现及验证

1. **数据库路径被误作 SQLite URI 参数**：business/configstore/taskstore 的 `file:` 拼接对 `?`、`#`、`%` 不转义，造成打开错误文件或启动失败，并可能绕开实际文件权限设置。先新增三个包的 `database_path_test.go`，均在三类特殊文件名上失败；改用共享 `sqliteutil.DSN` 后通过。保留中文空格文件名的成功路径。
2. **CI/发布遗漏浏览器回归门禁**：两个流程未执行 Playwright，发布还遗漏 Knip。新增 `.github/scripts/validate-browser-gates.test.mjs` 两个失败用例后补齐步骤；部署脚本/工作流测试 36/36 通过。两类 Docker 镜像已构建成功，代理隔离集成测试通过。
3. **错误消息脱敏泄漏完整或部分凭据**：`Authorization: Bearer ...` / Basic 会只遮蔽协议名，带空格和转义引号的密码仅遮蔽首段，`admin_key` 未识别。新增六种复现均失败，另补不完整引号值用例，修改完整值匹配后脱敏测试通过；跨调用方已纳入后端全量回归。
4. **任务按字符串比较可变精度时间**：同秒 `.1Z`、`.12Z` 顺序错误，时区偏移也会误排；日志清理以整秒为界会误删之后的任务。新增三个最新任务失败用例和一个误删除失败用例，统一持久化为 UTC 固定九位小数，并统一清理/回收边界；taskstore 全包最新测试通过（6.347s）。

## 验证边界

本轮不连接生产数据库、真实密码箱、真实上游或真实通知渠道。外部 HTTP 响应、取消与部分提交通过隔离服务验证；不把本地通过结果解释为生产联调或发布成功。已检查全部人工维护的分配范围，生成文件与依赖按其生成和运行边界检查。

## 根侧后续审查与修复

人工阅读已补完 `management/service.go`、`newapimanagement/service.go` 与 `pricing_cache.go`、`pricing/service.go` 与 `revenue.go`，以及 `business/store.go`、`schema.go`、`catalog.go`、`readmodels.go`、`management_sync.go`、`pricing_catalog.go`、`revenue_catalog.go`、`newapi_management.go`、`pricing_backups.go`、`traffic_ranking.go`、`cleanup.go`、`billing_quota_unit.go`。账号/上游、运维、前端基础、功能模块及完整 App 分段报告均已完成。

5. **New API 分组倍率补偿缺失**：GroupRatio 提交后返回 503 或请求取消时，新旧三端倍率不一致。`group_binding_rollback_test.go` 先两项失败，补入当前不确定写入的回滚，并使 New API、管理端、本地补偿使用有界 `WithoutCancel` context；New API 全包通过。
6. **New API 价格精度和配置解析**：数值 JSON 经 float64 丢失 `9007199254740993` 与高精度小数；倍率接受分数/十六进制/超限指数；配置接受尾随 JSON，非字符串或空白/冲突键被静默丢弃。`pricing_input_test.go` 全部先复现失败，再改为 UseNumber、EOF 校验及严格键值检查；完整 New API 包通过（3.227s）。
7. **账号维护运行模式和人工保护**：五类远程维护排队后切换监控模式仍执行写入；上游 Base URL 同步绕过人工优先、暂停、排除和熔断。`management/runtime_protection_test.go` 先观察到 1–4 次远程写入，补锁后模式与逐账号保护检查后管理包通过（1.021s）。
8. **价格分组调整绕过监控模式**：`pricing/runtime_mode_test.go` 先观察到监控模式仍改变远端和本地分组；补执行锁后运行模式复核，定价包通过（0.771s）。
9. **共享十进制解析资源边界**：目录倍率、管理端倍率、收入和成本、管理快照及配额单位使用无界 `big.Rat.SetString`，接受 `1e1001`、`1/2`、`0x10`。三个领域包的 `decimal_input_test.go` 先失败，再引入 `decimalutil.Parse` 对长度、十进制语法和指数做解析前限制；已完成相关包或针对性通过。
10. **写后失败隐藏远程副作用**：Base URL 写入成功但读回不一致时，汇总 `remote_write` 仍为 false。`base_url_write_result_test.go` 已先复现，改按实际写入次数统计；根领域完整回归已通过，最终统一结果见下文。
11. **损坏存储 JSON 被部分接受**：公共对象解析只读取第一个值，尾随第二段 JSON/垃圾仍被当成可用文档。`business/json_input_test.go` 已先复现，两个入口补 EOF 校验；根领域完整回归已通过，最终统一结果见下文。

上述流量排行、渠道目录、平台删除补偿、Header 保密契约均已闭环。未形成确定复现的候选不计入已修复问题。

12. **New API 渠道目录不完整**：上游缩小页尺寸后漏掉既有渠道，缺少列表/提前空页/损坏列表项仍被当成权威不存在。`channel_catalog_test.go` 先失败，改为按实际稳定项目数核对 total 并严格校验列表；模型与公开端点返回 HTTP 200 业务失败时拒绝数据或退回配置地址。完整 New API 包已通过。
13. **流量排行窗口与去重边界**：整秒起始窗口漏掉同秒小数请求；julianday 方案又会让纳秒边界外记录覆盖边界内同 request 记录。两项均先失败；持久化与查询统一固定九位 UTC，保持索引范围并在去重前精确过滤。索引测试改为验证实际生产查询及时间上下界，全部排行测试通过。
14. **平台删除取消后丢失绑定**：私有删除失败时使用已取消 context 恢复绑定，回滚失败。`platform_delete_rollback_test.go` 先复现 bindings 为 nil，改为有界独立补偿 context 后通过。
15. **价格调整取消被当成无需修改成功**：`pricing/cancellation_test.go` 先复现取消后 `Unchanged:1,error:nil`。现在未执行项有明确取消结果并返回取消错误，保留部分执行结果；清理租约延后到返回，避免把正常释放信号误认作业务取消。完整 pricing 包通过。
16. **渠道数值稳定 ID 丢失**：`channel_result_test.go` 先复现 id/channel_id 为超大 JSON 整数时响应缺少 ID；现在直接保留数值字面量并输出字符串，完整回归通过。
17. **改密并发仍可创建旧认证会话**：`api/login_rotation_test.go` 在认证与创建会话之间通过受控时钟边界旋转真实临时库密码，先观察到 HTTP 200 和有效 Cookie。登录/初始化/资料修改的会话签发现在在同一事务内校验凭据并写入；登录与资料回归通过。
18. **倍率写入取消仍返回成功**：`management/rate_cancellation_test.go` 先观察到 `failed:1,error:nil`。现在返回取消错误并保留部分结果，为未执行项补充明确取消状态；完整管理回归与全仓 race 已通过。

分配覆盖与详细证据：`global-review-accounts-upstreams-2026-09-07.md`（21 类修复）、`operations-review-2026-09-07.md`（9 类修复）、`frontend-review-2026-09-07.md` 和 `frontend-features-review-2026-09-07.md`（均已完成）。未确认的候选不计入修复数量；多个报告重叠的条目不机械相加。

## 前端与调度专项闭环

- [前端共享模块](frontend-shared-review-2026-09-07.md)：浏览器偏好存储不可用不再阻断登录；筛选键盘索引和禁用多选修正；精确 Host 凭据优先；导航尾斜线、任务终态与浅色状态文字对比度修复。
- [前端功能模块](frontend-features-review-2026-09-07.md)：全部功能生产文件与 API 契约审查完成，9 类问题定向修复。
- [App 账号/上游/开户](frontend-app-accounts-review-2026-09-07.md)、[鉴权交互](frontend-app-upstream-auth-review-2026-09-07.md)、[配置/巡检/策略](frontend-app-operations-review-2026-09-07.md)：覆盖完整 App，各自记录加载、失败、取消、确认、编辑和可访问性回归。
- [调度专项 10 项闭环](scheduling-policy-fixes-2026-09-07.md)：自动处置资格、恢复证据、旧草稿并发保护、全局预算、下限约束、容量文案、价格评分与绑定恢复均已修复。原始审查与压缩包保留失败证据。
- 补充策略变化阈值的有界十进制校验；README 删除已不存在的“调度模式”，与两种运行模式契约一致。

全量验证期间工作区还并行新增了模型价格批量选择/同步和默认零价格比较，以及健康证据待确认调整。保留这些改动并核对与本次修复的交互；最新批量价格测试补齐 PointerEvent 浏览器边界模拟、纠正 Testing Library 查询选项类型，并纳入最终全量检查。这些并行功能不计为本审查独立新增的功能。

## 最新统一验证

| 检查 | 最终结果 | 日志/说明 |
| --- | --- | --- |
| Go 竞态 | `go test -race ./...` 通过 | 40 包，38 个测试包通过、2 个无测试；`/tmp/sub2api-review-go-race-complete.log` |
| Go 静态检查 | `go vet ./...`、gofmt、`git diff --check` 通过 | `/tmp/sub2api-review-go-vet-complete.log` |
| Go 构建 | 通过 | `/tmp/sub2api-review-go-build-complete.log`；隔离产物 `/tmp/sub2api-review-server-complete` |
| Vitest | 196 文件、994 项全部通过 | `bun run test --maxWorkers=4 --minWorkers=1`；`/tmp/sub2api-review-vitest-complete.log` |
| Playwright | 桌面浅色与移动暗色 26/26 通过 | `/tmp/sub2api-review-e2e-latest.log` |
| TypeScript | 通过 | `/tmp/sub2api-review-typecheck-complete.log` |
| ESLint | 全量通过，0 warning/error | `/tmp/sub2api-review-lint-complete.log` |
| 格式 | 414 个匹配文件通过 | `/tmp/sub2api-review-format-complete.log` |
| Knip | 通过 | `/tmp/sub2api-review-knip-complete.log` |
| 前端生产构建 | 通过 | `/tmp/sub2api-review-build-latest.log`；总产物约 2479.3 kB，gzip 860.1 kB |

最后一轮全量 Go 执行保留有效缓存，并对最终 routing 实现重新运行（2.344s）。前一轮业务库完整 race 为 451.164s；其余包此前已通过，最终集成全量退出码为 0。较早的一轮恰逢并行健康逻辑修改而失败，不作为通过证据。

前端早期全量恰逢新增批量价格实现接入而失败；后续两项界面测试在同时跑浏览器和多份后端测试时出现等待超时，单独复核 6/6 通过。最终降低 worker 并发运行完整 994 项全部通过，没有扩大断言等待时间或跳过测试。

两种 Docker 镜像构建、隔离代理集成，以及 `.github/scripts/*.test.mjs` 36/36 已通过；后续未改动这些部署实现。最终 Go 与前端构建另列最新结果。
