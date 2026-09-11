# Sub2API Console

Sub2API 的独立可视化控制面。业务规则全部由 Go 后端领域服务执行，`sub2api-skills` 只作为流程和策略参考，不是运行时依赖。当前版本提供账号与分组管理、上游同步、鉴权恢复、探活巡检、调度写回、告警、开户、运行历史和请求追踪；浏览器不会直接接触 SQLite、Admin Key、Token 或密码箱。

开户自动获取模型时，优先调用 Sub2API 管理端模型同步预览接口；接口失败或未返回可用模型时，使用本次开户的 Base URL 和上游 Key 请求 `/v1/models` 兜底。兜底必须返回业务成功且非空的模型列表；两条路径均失败时保留待续记录，重试复用已有 Key。已配置分组模型列表时继续使用配置值。

## 技术栈

- 前端：React 19、TypeScript、Rsbuild、Bun、TanStack Query、Tailwind 风格 CSS、Lucide
- 后端：Go 1.26、Gin、modernc SQLite、标准库 HTTP Client
- 业务数据：只使用 Console 自有 `data/sub2api-console.sqlite3`
- 业务 API：Console 自己的 Admin API 客户端，远程写操作统一读回并审计
- 任务状态：Console 自己的 `data/tasks.sqlite3`，与业务数据分离
- 业务任务：Go 任务执行器 + Console 任务库，按领域服务执行
- 实时任务：Go 调度器 + SSE
- 领域服务：`internal/inspection`、`internal/probe`、`internal/authrecovery`、`internal/upstreamsync`、`internal/routing`、`internal/notification`；外部网络调用均通过受控适配器进入。

页面入口：运营总览、账号管理、上游管理、分组调度、调度策略、自动巡检、Uptime Kuma、日志中心、告警与通知、告警策略、请求查询、密码箱和系统设置。

## Uptime Kuma 接入

Uptime Kuma 菜单包含四个独立页面：接入配置（`/uptime-kuma/config`）、监控管理（`/uptime-kuma`）、功能模板（`/uptime-kuma/templates`）和状态页管理（`/uptime-kuma/status-pages`）。页面通过侧栏导航，不提供页头互跳；配置保存与断开操作位于页头，管理列表使用通用表格、筛选、分页和行内操作，编辑在通用弹窗中完成。通知渠道及维护计划的控制台页面和管理接口已移除，远端已有配置保留。

在「Uptime Kuma → 接入配置」填写服务地址与 Uptime Kuma 设置中创建的 API 密钥。支持输入实例根地址或以 `/dashboard` 结尾的仪表盘地址，保存前会验证 `/metrics` 接口。官方 API 密钥通过 HTTP Basic Auth 的密码字段读取指标，本身没有监控项管理权限。

需要管理监控项时，同时填写仪表盘账号和密码；已启用两步验证的账号还需填写当前 6 位验证码。Go 后端通过原生 Socket.IO / Engine.IO 4 WebSocket 接口登录并保存会话。反向代理必须允许 `/socket.io/` 的 WebSocket 升级。登录会话失效后重新输入密码和当前验证码并保存，不会自动重放写请求。地址、密钥、密码和登录会话保存在后端私有配置库，API 只返回地址、账号、配置状态和版本；验证码不持久化。更换地址必须重新输入对应实例的密钥及管理凭据，禁止把旧实例的凭据自动发送到新地址；连接不跟随重定向。

- 只配置 API 密钥：查看监控状态、响应时间、证书有效期和上游提供的 24 小时在线比例，每分钟刷新。暂停项可能不在指标中，旧版没有提供的指标显示「暂无数据」。旧版 `/metrics` 不带稳定监控 ID 时仅展示数据，不通过名称绑定管理目标。
- 监控管理：新增 HTTP(S)、HTTP 关键字、TCP、Ping、DNS、Push 和分组；页头提供独立「新增分组」入口，分组表单仅设置名称及所属分组，支持子分组；编辑名称、地址、检测及重试间隔和分组。HTTP 支持请求方法、请求头、请求体、状态码范围、重定向、TLS 和 Basic/Bearer 鉴权。已有敏感字段留空保留，可显式清空；编辑不能改变类型，分组调整校验循环引用，已有通知关联保留。监控列表按稳定父组 ID 展示可折叠的层级，支持分组筛选；搜索和状态筛选保留匹配子项的父组，跨页仍显示完整所属分组。Push 上报地址仅在详情中按需读取，不进入常规列表或浏览器持久存储。
- 功能模板：保存监控类型、地址、检测间隔、正常状态码、超时、重试参数、TLS 忽略、反转判断，以及请求方法、请求头、请求体和编码。监控地址填写服务地址，选择接口模式后自动补全 Messages、Chat Completions 或 Responses 路径；切换模式时保留网关前缀和查询参数，不重复追加 `/v1`。列表只返回摘要，编辑时通过受鉴权保护的 no-store 详情接口回显请求头和请求体，关闭弹窗清理查询缓存。请求头默认留空，不以示例充当默认内容；模板编辑不提供独立鉴权方式，保存时清除旧模板的独立鉴权配置，需要认证时可填写请求头。已有监控不随模板修改而改变，监控表单仍可独立设置鉴权，不再提供「使用模板鉴权」选项，选择或切换模板保留已填的监控鉴权信息。
- 内置请求模式：Claude Messages（`/v1/messages`）、OpenAI Chat Completions（`/v1/chat/completions`）、OpenAI Responses（`/v1/responses`）及 Claude CLI。选择模式时由 Go 后端生成预设并填入可编辑请求体，后续保存以当前文本为准，不再覆盖修改；请求模型和发送消息提供独立输入，修改会同步到请求体；原始请求体默认收起，可展开查看或编辑，复杂内容只修改对应字段并保留其他参数。标准模式不自动填入请求头，直连 Claude 需自行设置 `anthropic-version`；显式选择 Claude CLI 会填入其专用请求头。请求体编码支持 JSON、表单（x-www-form-urlencoded）和 XML，套用时传递至 Kuma 的 `httpBodyEncoding`；JSON 内容会校验。默认预设非流式、输出上限 16 token，OpenAI 使用 `store=false`。请求规范参考 [Claude Messages](https://platform.claude.com/docs/en/api/messages/create)、[OpenAI Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)、[OpenAI Responses](https://developers.openai.com/api/reference/resources/responses/methods/create)。
- 状态页：新增、编辑和删除，配置标题、路径、说明、主题、域名、页脚、标签及证书显示，管理公开分组及监控项顺序。编辑保留原路径和未修改的展示设置；公告、历史事件、外观高级配置及尚未提供专属表单的监控类型仍在原仪表盘管理，不表示覆盖 Uptime Kuma 的全部原生功能。
- 暂停、恢复和删除需要确认目标及 ID；非空分组需先移出子项再删除。配置版本或监控项基本信息发生变化时，旧页面的写入会被拒绝，需刷新后重试。Uptime Kuma 原生接口没有跨客户端原子版本控制，因此其他仪表盘仍可能在校验后的极短窗口内修改同一项。
- 验证配置和远端写入创建后台任务，页面通过 Query 读取进度，日志中心保留结构化任务结果；任务创建审计与执行结果分开记录。连接中断后先核对远端状态及任务结果，不自动重试新增、修改和删除。断开接入只清除本地配置及凭据，不删除远端监控项。

接入层针对 Uptime Kuma 1.23/2.x 原生事件契约实现，测试使用隔离 HTTP/WebSocket 服务和临时数据库，不连接生产实例。Prometheus 文本解析使用 Apache-2.0 许可的 `prometheus/common/expfmt`，Socket.IO 的有限事件适配复用已有 Go WebSocket 库，不引入额外业务运行时。

## 调度与告警事实模型

控制面策略是调度参数的唯一配置来源。健康样本先生成账号健康评估，再由调度引擎生成目标，完全模式才允许自动执行远程变更：

```text
health_samples -> account_health_evaluations -> routing_decisions -> routing.writeback
                                                              -> operation_audit
```

- `routing_decisions`：本轮调度判定和期望状态，不代表已经在 Sub2API 生效。
- `accounts.routing_state`：远端读回一致后确认的当前生效状态；仅计算字段不得改写它。
- `operation_audit`：远程写入、读回和本地提交的执行事实，用于执行失败告警与日志。
- 调度异常告警读取 `routing_decisions`，文案统一称为“调度判定”；自动执行失败告警读取 `operation_audit`。
- 上游余额同步成功后立即评估并投递该 Host 的余额告警或恢复通知，不等待其他上游、探活、调度计算或写回完成；手动同步和鉴权恢复后的余额同步走同一流程。通知沿用告警策略、恢复开关和去重规则，投递失败保留记录并由后续告警评估重试。充值仍需等下一次成功读取余额才能被发现，通知正文时间表示告警评估时间，不是充值时间；可结合 `upstream.sync`、`upstream.balance_alert` 运行事件与投递记录排查延迟。
- 监控模式只评估健康状态；完全模式保存调度目标并按字段开关自动执行。
- 人工优先位账号不参与自动成本墙、熔断和调权；账号与分组详情显示“人工优先位”，不沿用历史自动处置或待执行目标。真实暂停、平台停用和上游错误仍正常展示，历史健康样本保留供查看。
- 主动探测总开关只读取 `probe.enabled`；`health.source` 只决定常规健康证据来自真实流量还是主动探测。

真实请求从 `GET /api/v1/admin/ops/requests` 采集结果与错误，并通过 `GET /api/v1/admin/usage` 按账号 ID、请求 ID 补充成功请求的 `first_token_ms`。总耗时与首字分开保存；使用记录接口失败时保留运维证据并报告首字采集失败，后续重试可以为已有样本补齐首字。

真实请求首字和主动探针测得的首个内容到达时间，都按健康分公式中的 `scoring.slow_ttfb_ms` 判断慢响应；默认严格超过 5000ms 时，单次按配置的响应慢分值（默认 65 分）参与短期、长期和综合评分。延迟处置使用独立的 `breaker.latency_ttfb_ms` 和次数窗口。当前状态、连续失败及恢复条件使用新鲜证据：真实流量按 `traffic.lookback_minutes`，探针按独立的 `probe.freshness_seconds`（默认 900 秒），不再随全局或分组探测间隔变化。短期、长期和综合评分按独立的 `scoring.history_window_minutes`（默认 1440 分钟，最多 7 天）回溯，再受短期 10 条、长期 60 条的数量窗口限制。有新鲜流量时使用流量历史及比最新流量更新的探针，否则使用探针历史；没有新鲜有效证据时不启用历史评分，不能凭旧成功记录确认当前健康或恢复。熔断账号沿用新鲜恢复样本评分，避免熔断前历史阻碍恢复。可在调度策略的「巡检与采样」及「健康分公式」中分别调整；历史回溯不会补抓或生成未保留的样本。P50/P95 和速度排序仍只使用真实请求首字，不混入探针或请求总耗时。页面显示有效样本数，正常成功探针标为“探测通过”。账号管理的健康评分详情分别展示本轮短期、长期实际样本数；短期样本是长期样本中最新的一部分，不重复相加。短期数量与同轮评分绑定，旧评估缺少记录时显示“未记录”，下一轮调度后补齐。设置中的“评分历史范围”和“当前探针有效期”表示时间范围，不是样本数量。

账号管理打开时，当前页账号的真实流量由独立采集通道检查，成功采集后间隔约 5 秒继续检查，不等待巡检中的主动探针。重复查看同一账号共享采集，最多并发 4 个账号；离开页面或翻页后取消不再观看的采集。失败后间隔 30 秒重试。实际刷新延迟还受上游日志可见时间、接口耗时和并发排队影响。

新请求通过需要会话鉴权的 SSE 逐条推送，以稳定结果 ID 去重追加，补充首字延迟时原位更新；色块从左到右由旧到新，仅显示最近 10 条。连接建立和重连时补取最近最多 100 条，浏览器仅在 React Query 内存中保存有界结果；大量请求超过补取窗口时只保留近期结果。页面显示连接和采集重试状态。健康分仍表示最近一轮调度的评估结果，可晚于实时色块更新。

启用健康降级后，有效健康分严格低于 `degrade.score_threshold` 即判定降级，不再要求先达到熔断层的失败或慢响应次数。短暂失败尚未达到确认阈值时，判定携带 `evidence_pending`，按半幅健康门控损失温和降权，不额外叠加降级优先级步长、负载折扣或并发伸缩。待确认调度分为 `min(上一调度分, (100 + max(weights.gate_floor, 当前健康分)) / 2)`；重复评估相同样本不会累计减分。例如门控底线 40、上一调度分 100、当前健康分 40 时，展示的健康分仍为 40、状态判定为降级，调度分暂用 70。权重最终仍按组内预算归一化，并受现有写入死区和冷却控制。

连续失败或滚动窗口达到现有确认阈值后，使用完整健康分及降级处置；已确认的降级不会因失败次数暂时减少而退回温和处置。中性客户端错误保留之前的调度判定，人工控制、致命错误、熔断保底、成本墙及连续成功、健康保持、恢复冷却规则继续优先执行。健康分恢复到 100 也不代表恢复条件已经全部满足。

## 模型参考价格

价格分组调整在每个已加入的互换组内，优先选择满足目标成本利润率（利润 ÷ 账号成本）且售价最低的分组。若所有候选分组均未达到目标，则回退到同平台、售价最高且能够覆盖成本的分组，并提示利润未达标；仅当所有候选分组均亏损时保留原分组。相同售价按稳定分组 ID 选择，不跨互换组迁移。例如目标为 25%、账号成本为 0.21、平价组售价为 0.20、旗舰组售价为 0.25 时，迁入旗舰组，实际成本利润率约为 19.05%。自动巡检完成价格分组调整后，再按新成员关系计算成本墙与健康状态；分组迁移不绕过人工保护、熔断或分组利润控制。

New API 的模型价格页优先读取 Sub2API 使用的公开远程价卡。对远程缺失的已配置或已启用模型，Go 后端使用系统设置中的 Sub2API 管理地址与 Admin Key，调用 `GET /api/v1/admin/channels/model-pricing?model=...` 获取默认价格。该接口需要管理员权限，默认价格也可能来自 Sub2API 自身加载的远程价卡或内置回退，不包含分组倍率和渠道自定义价格。

价格和“未找到”结果保存在本机 `console-config.sqlite3` 的 `model_pricing_cache` 表中，有效期为 24 小时，重启后仍可复用。过期后的首次读取会重新拉取；新增模型或需要立即更新时，点击模型价格页工具栏的“强制刷新参考价格”。缓存按 New API 平台和 Sub2API 管理地址隔离。刷新失败保留上次成功价格并显示提示，短时间内重试复用已成功查询的单模型缓存。管理密钥不会发送到浏览器或公开价卡地址。

模型价格页支持跨页勾选和全选筛选结果，每批最多同步 1000 个模型。批量同步先展示目标价格和跳过原因，确认后一次提交有效模型，并逐项核对读回结果。比较 Sub2API 默认价格时，未配置的缓存写入、1 小时缓存写入和图片输入与对应默认零值不产生差异；明确的非零价格差异以及输入、输出和缓存读取的零价仍正常比较。

比较、批量预览、单项同步和远程价格列表共用前端查询缓存，按后端返回的到期时间判定是否需要重新读取；读取旧缓存不会延长其 24 小时有效期。主动“强制刷新参考价格”仍立即请求后端更新。后台刷新保留已有价格、比较结果和分页，仅更新刷新按钮状态；参考价过期或读取失败时不能用旧数据执行批量同步。模型检测、渠道模型选择和无绑定 Key 扫描同样保留刷新前的列表与选择，并在刷新中或失败时禁用依赖新结果的写入操作。

## 本地启动

启动 API：

```bash
cd backend
go run ./cmd/server
```

再启动前端：

```bash
cd frontend
bun install
bun run dev
```

打开 <http://localhost:3004>。

运行时不需要安装或挂载 `sub2api-skills`。它只作为业务设计参考；Console 的数据库、任务调度和业务执行都在本项目内完成。

默认数据库均位于当前目录的 `data/`：`sub2api-console.sqlite3`、`tasks.sqlite3` 和 `console-config.sqlite3`。Console 启动和运行不需要任何外部运行库路径，也不会挂载或读取 `sub2api-skills` 的数据。

首次打开会进入初始化页，需要设置控制台账号密码、Sub2API Admin Base URL 和 Admin Key。远程访问时必须在服务端配置至少 32 个字符的 `SUB2API_CONSOLE_SETUP_TOKEN`，并在初始化页输入相同令牌。令牌只通过 `X-Setup-Token` 请求头发送，不写入配置数据库，初始化完成后即不能再次使用该接口覆盖配置。

首次使用 Docker Compose 时，在项目根目录复制带中文注释的配置模板；已有 `.env` 时直接编辑，不要覆盖：

```bash
cp -n .env.example .env
chmod 600 .env
openssl rand -hex 32
```

将生成的令牌填入 `.env` 的 `SUB2API_CONSOLE_SETUP_TOKEN`，访问端口默认是 `3004`，需要时修改 `SUB2API_CONSOLE_FRONTEND_PORT`。默认配置只有这两项。Docker Compose 自动读取根目录 `.env`，无需逐项 `export`；同名 shell 环境变量会覆盖文件中的值。`.env` 已被 Git 忽略，可提交的 `.env.example` 保持初始化令牌为空。Docker Hub 的 `DOCKERHUB_TOKEN` 继续保存在 GitHub Actions Secret 中。

启动前可以执行 `docker compose config --quiet` 检查配置格式，此命令不会启动容器或发布镜像。直接运行 Go 后端时仍通过进程环境传入配置，不会自动加载此 Compose `.env` 文件。

初始化完成后可在下次重建 API 容器时从部署环境中移除该变量。控制台随后使用 HttpOnly 会话 Cookie 登录；Admin Key 只保存在后端 `data/console-config.sqlite3`，不会返回到浏览器。业务账号、分组、绑定、运行记录和告警只从 Console 自有业务库读取。

单体服务通过同源路径提供页面和 `/api`，远程访问时只需开放 `3004`。多套 Compose 项目可使用不同的 `SUB2API_CONSOLE_FRONTEND_PORT` 并行部署；数据库统一保存在宿主机部署目录的 `./data`，路径无需配置。

使用 Docker Compose 部署：

```bash
docker compose pull
docker compose up -d
docker compose ps
```

默认使用本仓库发布的 `mienvirtuoso/sub2api-console:latest` 多架构单体镜像。镜像地址直接写在 `docker-compose.yml` 中；按版本部署时，将 `image` 标签改为已发布版本。本地开发仍可使用 `docker compose up -d --build` 构建当前源码。

Compose 为单体服务配置自动重启和健康检查。容器启动时会调整挂载的 `./data` 目录权限，随后以非 root 用户运行；不要把其他目录挂载到 `/app/data`。

## 发布

镜像在 GitHub Actions 中构建，并推送到 Docker Hub 的 `mienvirtuoso` 命名空间。首次配置时，在 Docker Hub 创建 `sub2api-console` 仓库并设为 Public。在 GitHub 仓库的 **Settings → Secrets and variables → Actions** 中添加 `DOCKERHUB_TOKEN`，值为 `mienvirtuoso` 账号具有 Read & Write 权限的 Docker Hub Access Token。令牌只保存在 GitHub Secret 中。

版本使用日期标签：当天首个版本为 `vYYYY.MM.DD`，后续版本依次为 `-2`、`-3`，禁止使用 `-1`。创建标签前必须提交 `.github/release-notes/<tag>.md`；具体硬性规则见 [发布说明流程](.github/release-notes/README.md)。

标签推送后，GitHub Actions 会执行完整检查并构建单体 Docker 镜像，发布 amd64/arm64 多架构镜像：

- `mienvirtuoso/sub2api-console:<tag>`

发布后会同时更新 `latest`；生产部署需要指定版本时，直接修改 Compose 中的 `image` 标签。

## 验证

```bash
cd backend && go test -race ./... ./internal/onboarding/__tests__
cd frontend && bun run test && bun run typecheck && bun run lint && bun run build
```

首次克隆仓库后启用提交前检查：

```bash
git config core.hooksPath .githooks
```

`pre-commit` 会先检查暂存区空白错误，再执行与 CI 相同的前端格式检查。检查失败时提交会被阻止；运行 `cd frontend && bun run format` 修复格式后重新暂存并提交。

Console 不提供外部运行库实时读取或同步接口。主动探测只使用 Console 私有配置库中的授权信息和业务库中的账号记录；探测结果写回 Console 业务库。业务写回按当前控制台策略和权限执行，与 skills 数据库没有关联。
