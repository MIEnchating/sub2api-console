# 测试并发验证（2026-09-30）

## 范围

工作区基于 `12a3d800e6d231a9db6fd6437e51c33074c2d431`，包含尚未提交的既有前后端改动。本记录对应当前工作区验证，不代表该提交的 CI 或发布成功。

本轮调整 Vitest worker 数量，并将一个开户交互测试中的逐字输入改为粘贴，保留真实输入交互、测试隔离、原有 5 秒用例超时及全部用例。最终配置使用 `max(1, min(8, availableParallelism() - 1))`，当前机器的 16 个可用 CPU 对应 8 个 worker，较小机器自动下调。

## 前端全量实测

在 `frontend/` 运行，三轮间业务源码未变，仅调整 worker 数量。耗时受同机在线服务及缓存影响，是单次实测，不代表所有机器的固定提速比例。

| Worker | 命令 | 文件 / 用例 | 耗时 | 结果 |
| --- | --- | --- | --- | --- |
| 4 | `bun run test --reporter=dot` | 522 / 2635 | 781.13 秒 | 全部通过 |
| 8（优化前） | `bun run test --reporter=dot --silent` | 522 / 2635 | 418.16 秒 | 1 个用例超时，未采用 |
| 6 | `bun run test --reporter=dot --silent` | 522 / 2635 | 534.79 秒 | 全部通过 |
| 8（优化后） | `bun run test --reporter=dot --silent --maxWorkers=8 --minWorkers=8` | 523 / 2637 | 412.26 秒 | 全部通过 |

8 worker 比本机 6 worker 的单次运行缩短约 22.9%，比 4 worker 缩短约 47.2%。首次 8 worker 运行中 `automatic-upstream-info.test.tsx` 超时；该用例逐字输入三个字段，改为粘贴后单测通过，全量 8 worker 也通过。`--silent` 仅关闭测试控制台输出，失败仍使进程返回非零。两次全量运行的业务源码及用例数量不同，这些数字只代表各自工作区的一次实测，不是严格的同源码对照实验。

原 6 worker 配置将常量 6 改为 CPU 自适应表达式后，本机仍为 6；当时该开户回归 1/1 通过，用例 4.58 秒、整次执行 16.25 秒。粘贴输入后单独运行用例约 2.06 秒；随后用命令行显式覆盖为 8 worker 跑完整回归，再把自适应上限调整为 8，避免对相同配置重复运行全套。

## 其余已验证结果与边界

- 根外壳从 `App.tsx` 分离，隔离构建首屏同步 JavaScript 从约 1.70 MB 降至 0.93 MB（未压缩），约减少 45%；这不是整站响应时间改善比例。
- 外壳相关 9 个测试文件 / 35 个用例、Uptime Kuma 相关 4 个文件 / 19 个用例在前序检查中通过，随后纳入上述全量回归。
- 前序后端验证：`go test -race -count=1 -timeout=20m ./internal/business` 通过，实测约 2 分 46 秒；`bash scripts/check-go.sh test` 与 `bash scripts/check-go.sh vet` 通过。完整脚本大量命中缓存，不能用其约 7 秒耗时声称未缓存全套也只需 7 秒。
- 系统信息页已将完整账号列表读取延后到打开缺少账号名称的旧任务明细时；相关测试、typecheck、lint 与格式检查通过。生产库只读查询显示 469 个账号的 ID 与名称读取约 5.33 ms，不能与完整账号 API 的历史约 2 秒耗时等同。
- 尚未完成整站在线 API 延迟、SQLite 热点及全部页面实际渲染的统一测量；本轮首屏拆分尚未部署到 3004。不得据此声明原整站卡顿任务全部完成。

后续按变更范围使用仍有效的结果；业务源码、测试、依赖和测试配置未变时不重复执行全套。新的修改仅使受影响验证失效，范围不明时才扩大验证。

2026-09-30 补充验证：首次完整 `bash scripts/check-go.sh test` 在同机并发负载下，`modelcheck/__tests__/TestAnimationAllSelectedAccountsExceedTwenty` 超过共享测试 helper 的 5 秒任务完成期限（约 9.62 秒），因此该次门禁失败。该包单独执行 `go test -race -count=1 -timeout=20m ./internal/modelcheck/__tests__` 通过；限制 Go 包级并发后，`GOFLAGS='-p=8' bash scripts/check-go.sh test` 完整通过，其中 `modelcheck/__tests__` 耗时 447.438 秒。此次复跑有缓存命中的包，不能把结果当作全包冷运行耗时。`git diff --check` 同时通过。共享 5 秒期限在高负载下仍有脆弱性，后续应把等待契约改为适合批量任务的明确完成条件，并单独衡量该包的长耗时。

## 倍率同步后的调度复核热点

生产任务记录显示，最近多轮倍率同步在 10 分钟期限内完成了约 467 个目标的倍率确认，但随后的成本分组与调度复核超时。旧实现对约 450 个已确认账号逐个执行完整调度计算和写回，并把同一超时错误重复追加。新增回归先复现了 450 次计算、450 次写回；改动后为一次全局计算和一次批量写回，远端写回目标仍限制为本次确认的账号。未变化倍率仍参与复核，手动成本墙和价格迁移仍按原有批量入口执行。

本次 Go 验证：受影响的 9 个包、`go test -race ./...`、`bash scripts/check-go.sh vet` 和最终一次 `bash scripts/check-go.sh test` 全部通过。完整脚本首次在同机并行测试负载下出现 `routingwrite` 4 秒保护用例超时；该用例独立 race 连续 5 次通过，随后完整脚本重跑通过。完整脚本的长包还包括 `routing/__tests__` 约 735 秒、`routingwrite/__tests__` 约 692 秒；运行期间存在重复全量测试争用，这些耗时不能作为空闲机器的基线。

为缩短 Go 回归时间，`backend/internal/routing/__tests__` 的 161 个顶层用例现在使用 `t.Parallel()`，并在该测试包的 `TestMain` 中将并行上限固定为 4，避免与其他 race 测试包同时运行时过度争抢 CPU。单包 race 实测从串行约 670 秒降至 187 秒；完整 `bash scripts/check-go.sh test` 在当前工作区通过，报告该包 196.876 秒。该优化只改变测试调度，不改变业务运行时。

前端轮询审计发现，顶栏、侧栏、系统设置和自动巡检页会对同一个 `auto-inspection` 查询分别创建 15 秒轮询；路由页错峰挂载时会产生额外请求。现由已认证外壳的顶栏负责 SSE 断线后的唯一轮询，其他观察者设置 15 秒缓存并只读取共享查询。新增回归覆盖侧栏延后挂载不会创建第二个定时器；相关 3 个测试文件共 33 个用例、typecheck、lint 和格式检查通过。

只读生产库测量还确认了一个待处理的后端热点：467 个账号、1,146,574 条 `account_stability_samples` 上，账号列表稳定性聚合查询单次约 1.69 秒（使用现有覆盖索引）。这部分仍需单独设计可验证的聚合或缓存方案，当前没有把它误报为已优化。

补充只读测量：467 个账号的原查询返回 883 组，耗时约 2.05 秒；不加账号 ID 筛选返回 892 组，耗时约 2.07 秒。`EXPLAIN QUERY PLAN` 显示原查询使用 `ix_account_stability_covering` 按账号和时间范围搜索，随后为分组使用临时 B-tree。该测量只说明冷查询仍昂贵，不代表完整账号 API 或页面渲染耗时；5 秒进程内缓存仍需用真实登录流量验证命中率。

该修复已进入当前运行的后端二进制。2026-09-30 07:10:14 UTC 后端服务重启后，恢复了 07:01:09 创建的 `account-rate-sync` 任务；该任务于 07:13:31 成功完成，更新 1 个、未变 446 个、跳过 17 个、失败 0 个。此前多轮任务在约 10 分钟处因 `context deadline exceeded` 失败。这次任务跨越重启，约 12 分 22 秒的创建至完成时间不能用作新版从头执行的耗时；重启至完成约 3 分 17 秒，也不能单独归因于本次改动。尚需等待自然触发的完整新任务样本，并测量在线 CPU、接口和页面响应；整站卡顿尚未完成统一验证。

## 稳定性查询与写回测试后续

在只读生产库副本上，对约 115 万条稳定性样本进行 467 个账号的分组统计，新 `(account_id,outcome,observed_at)` 索引把单次查询从约 1.89 秒降到 0.63 秒，883 组结果一致。业务 schema 升至 11；启动升级在事务内建新索引并删除旧的 `ix_account_stability_covering`、`ix_account_stability_window`。空库和旧库的查询计划与索引替换均有回归测试。此索引尚未迁移到运行中的生产业务库，副本结果不能当作在线接口耗时。

后端 `routingwrite/__tests__` 的 90 个顶层用例改为受控 4 路并行，仍使用各用例独立的临时数据库和测试 HTTP 服务。该包 `go test -race -count=1` 在当前机器上约 178.5 秒通过；之前一次完整回归中串行包约 601.7 秒。两次并非严格同负载对照，不能据此保证固定提速倍数。与索引相关的 `business`、`business/__tests__` race 分别约 177.0 秒和 331.9 秒通过，`check-go.sh vet`、`gofmt`、`git diff --check` 通过。此前一轮完整回归因修改过程中运行的旧版本断言和旧查询计划测试失败，不能记录为通过。

最终工作区执行 `GOFLAGS='-p=4' bash scripts/check-go.sh test` 完整通过，包含自动发现的全部 `__tests__` 包；其中 `routingwrite/__tests__` 为 169.249 秒，`routing/__tests__` 为 187.941 秒，`modelcheck/__tests__` 为 464.827 秒，`business/__tests__` 为 343.320 秒。该次运行有未变包缓存命中，不能作为全包冷运行基线。新索引仍未部署，线上收益尚待服务加载后核验。

## 部署前前端回归

最新全量单测 `bun run test --reporter=dot --silent` 在不与浏览器回归并发的情况下通过：523 文件、2646 用例，404.95 秒。日志为 `/tmp/frontend-full-test-clean-20260930.log`。渠道测试使用真实 memory Router 适配页面链接；动画请求的精确断言同步包含默认提示词，弹窗沿用共享焦点管理。

本地 Playwright worker 上限调整为 6，并按可用 CPU 自动下调；CI 仍保持每分片 2 worker。第一次完整浏览器回归为 859 通过、14 失败、5 项原有跳过，耗时 18.9 分钟。已修复同路径搜索参数切换时外壳遮罩导致的键盘焦点丢失；同步两个动画请求断言，并用可控浏览器时钟保护连接概要 15 秒缓存到期后刷新的行为。手机动画工具栏为取消入口预留固定 32px 空间，防止任务启动后换行导致高度跳动。相关 14 场景在桌面和手机项目中全部通过，42.7 秒，日志为 `/tmp/frontend-e2e-affected-fixed-20260930.log`。

上述浏览器修复后 `bun run typecheck`、`bun run lint`、`bun run format:check` 全部通过。全量单测之后的外壳和动画面板修改需补充受影响单测结果；最终完整浏览器回归及实际部署结果另行记录，不将首次失败回归记为门禁通过。

## 最终回归与部署

最终工作区回归结果：

- Vitest：523 个文件、2646 个用例通过，耗时约 404.95 秒，日志为 `/tmp/frontend-full-test-clean-20260930.log`。
- Playwright：873 个用例通过、5 个既有跳过、0 个失败，耗时约 19.5 分钟，日志为 `/tmp/frontend-e2e-final-fixed-20260930.log`。
- 受影响前端单测：19 个文件、90 个用例通过，日志为 `/tmp/frontend-affected-final-20260930.log`。
- `bun run typecheck`、`bun run lint`、`bun run format:check`、生产构建均通过；最近修改的动画工具栏测试文件单独 lint 通过。
- 后端 `GOFLAGS='-p=4' bash scripts/check-go.sh test`、`go test -race ./...` 和 `bash scripts/check-go.sh vet` 均通过。

前后端已于 2026-09-30 12:00 UTC 后重新部署。后端 systemd 服务 PID 为 `2716970`，监听 `8080`，候选二进制 SHA256 为 `5714a2778a861b9a4774617ea85a473062df29eb4408cd0eabe37890635f1ed9`；前端服务 PID 为 `2718283`，监听 `3004`。当前线上检查返回：前端 `/`、`/api/setup/status`、`/api/auth/session` 均为 `200`，未登录 `/api/accounts` 为预期的 `401`，后端直连 `/api/setup/status` 为 `200`。没有重复监听实例，临时验证服务已退出。

生产数据库已完成迁移核对：`PRAGMA user_version` 为 `11`，`ix_account_stability_grouped` 存在，旧索引 `ix_account_stability_covering` 和 `ix_account_stability_window` 已移除。迁移前备份及新前端、后端候选产物保存在 `/root/account-workbench-artifacts/performance-deploy-20260930/`。

## 13:15 UTC 再次部署

本轮最终候选重新通过前端 `typecheck`、`lint`、`format:check` 和生产构建，以及后端 `GOFLAGS='-p=4' bash scripts/check-go.sh test` 与 `bash scripts/check-go.sh vet`。后端以当前工作区源码重新编译并安装，SHA256 为 `e36dcbacda2311c367cc47c8c44a504390f4c4253ebf8fefb25fabbf821a23f2`。

2026-09-30 13:15:13 UTC 重启 `sub2api-console-backend.service` 与 `sub2api-console-frontend.service` 后，两者均为 `active`；后端 PID `2757113` 监听 `8080`，前端 PID `2757125` 监听 `3004`。前端 `/` 与后端 `/api/setup/status` 返回 `200`，`/api/auth/session` 返回未认证状态，未登录 `/api/accounts` 返回预期 `401`。重启后未发现重复监听实例或遗留 `go run` 临时进程。

## 稳定性缓存覆盖修复与再次部署

账号稳定性 5 秒缓存此前只校验有效期；当缓存由部分账号组成、后续请求包含新账号时，会把缓存未覆盖的账号误当成无样本。现改为仅在缓存完整覆盖本次请求的账号集合时命中，并新增部分覆盖回归测试。

本次修复后的 `internal/business` 受影响测试通过，单独竞态重跑了两个全量并发下出现时序超时的既有用例，均通过；`bash scripts/check-go.sh vet` 通过。一次同时运行 `go test -race ./...` 与 `bash scripts/check-go.sh test` 的全量竞态回归中，两个既有 4 秒并发保护用例出现上下文超时，单独重跑通过，未将该次波动记录为全量门禁通过。

前端生产构建成功，后端候选二进制成功构建并安装。2026-09-30 12:41 UTC 重启 `sub2api-console-backend.service` 与 `sub2api-console-frontend.service` 后，两者均为 `active`；后端 PID `2738091` 监听 `8080`，前端 PID `2738107` 监听 `3004`。`/api/setup/status` 返回初始化和目标配置均正常，前端首页返回 `200`。后端二进制 SHA256 为 `fcb40ae8d8f198be00cff75168de30c33fd95d112e27218f96912714c557642f`。

## Node 与 DOM 测试环境拆分

本轮只修改 Vitest 配置与测试 setup：`*.test.ts` 使用 Node，`.test.tsx` 与其他已有测试保持 jsdom。需要 DOM 的 TypeScript 测试可在首部声明 `// @vitest-environment jsdom`。原 DOM setup 完整移入 `src/test/setup-dom.ts`，仅由 DOM 环境加载；Node 环境保留 `vi.unstubAllGlobals()`，文件隔离、worker 上限、超时、测试与断言均保留。

相同的 94 个 `.test.ts` 文件 / 695 个用例在本机依次运行，命令为 `bun run test --reporter=dot --silent <全部 .test.ts 路径>`。路径通过 `rg --files src` 筛选并排序；没有并行执行另一套单测或浏览器回归，后台服务仍运行。

| 配置 | 实际耗时 | setup 累计 | collect 累计 | 环境初始化累计 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 全部 jsdom | 36.36 秒 | 40.94 秒 | 60.72 秒 | 99.15 秒 | 94 文件 / 695 用例通过 |
| Node + 按需 DOM setup | 21.20 秒 | 3.44 秒 | 69.69 秒 | 0.035 秒 | 94 文件 / 695 用例通过 |

该子集的实际耗时减少约 41.7%；累计阶段时间是多个 worker 的总和，不能相加当作实际耗时。这是两次顺序实测，受缓存、同机负载影响，不代表全套或其他机器的固定收益。原测试与源码未改动；类型检查、涉及文件 ESLint、oxfmt 检查通过。日志分别为 `/tmp/vitest-node-baseline-20260930.log`、`/tmp/vitest-node-candidate-20260930.log`。

共享 setup 拆分后的完整回归日志为 `/tmp/frontend-node-full-20260930.log`：523 个文件中 522 通过、1 失败；2646 个用例中 2645 通过、1 失败，实际耗时 455.52 秒。唯一失败是模板 JSON 编辑器的真实动态导入尚未完成时，`findByRole` 的等待超时；未将这轮记录为全量通过。该次 collect 累计为 1705.76 秒，不能据子集收益声称整套已经提速，也不能直接用与历史运行的差值量化配置回退。

`template-json-editor.test.tsx` 随后改为在 React `act` 中等待 `vi.dynamicImportSettled()`，再执行原有编辑、模型同步和提交断言。没有 mock 编辑器、增加固定 sleep、放宽超时或删除断言。该文件两项用例通过，耗时 7.83 秒（用例共 4.12 秒），日志为 `/tmp/template-json-editor-settled-20260930.log`。其他 522 个文件、业务源码、Vitest 配置与 setup 未再变化，按避免重复验证的规则保留其通过结果；当前结果由完整运行的 522 文件通过与最后受影响文件通过组成，不表述为修复后又执行了一次完整绿灯回归。

最终受影响检查：`bun run typecheck`、4 个相关 TS 文件的 ESLint 与 oxfmt 检查、`git diff --check` 均通过。候选文件校验确认，完整回归后仅 `template-json-editor.test.tsx` 发生变化；其最后两个用例已通过，其余结果继续有效。

按用户“完成以后重新部署前后端”的要求，于 2026-09-30 13:48 UTC 再次完成前端生产构建、`go build -o /tmp/sub2api-console-node-deploy-20260930 ./cmd/server`、后端二进制安装及现有 systemd 服务重启。本轮无 Go 源码修改，沿用前序有效 Go 检查结果。前端构建日志为 `/tmp/frontend-node-deploy-build-20260930.log`，产物总计 3951.3 kB，gzip 1337.4 kB。

后端 PID `2784729` 监听 8080，前端 PID `2784741` 监听 3004，两项服务均为 active；前端首页和后端 setup 接口均为 200，未登录账号接口为预期 401。运行二进制 SHA256 为 `fcb40ae8d8f198be00cff75168de30c33fd95d112e27218f96912714c557642f`。端口各只有一个实例，未发现遗留临时 `go run` 进程。
