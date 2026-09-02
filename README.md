# Sub2API Console

Sub2API 的独立可视化控制面。业务规则全部由 Go 后端领域服务执行，`sub2api-skills` 只作为流程和策略参考，不是运行时依赖。当前版本提供账号与分组管理、上游同步、鉴权恢复、探活巡检、调度写回、告警、开户、运行历史和请求追踪；浏览器不会直接接触 SQLite、Admin Key、Token 或密码箱。

## 技术栈

- 前端：React 19、TypeScript、Rsbuild、Bun、TanStack Query、Tailwind 风格 CSS、Lucide
- 后端：Go 1.26、Gin、modernc SQLite、标准库 HTTP Client
- 业务数据：只使用 Console 自有 `data/sub2api-console.sqlite3`
- 业务 API：Console 自己的 Admin API 客户端，远程写操作统一读回并审计
- 任务状态：Console 自己的 `data/tasks.sqlite3`，与业务数据分离
- 业务任务：Go 任务执行器 + Console 任务库，按领域服务执行
- 实时任务：Go 调度器 + SSE
- 领域服务：`internal/inspection`、`internal/probe`、`internal/authrecovery`、`internal/upstreamsync`、`internal/routing`、`internal/notification`；外部网络调用均通过受控适配器进入。

页面入口：运营总览、账号管理、上游管理、分组调度、调度策略、自动巡检、日志中心、告警与通知、告警策略、请求查询、密码箱和系统设置。

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
- 监控模式只评估健康状态；调度模式保存调度目标；完全模式保存目标并按字段开关自动执行。
- 主动探测总开关只读取 `probe.enabled`；`health.source` 只决定常规健康证据来自真实流量还是主动探测。

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

首次打开会进入初始化页，需要设置控制台账号密码、Sub2API Admin Base URL 和 Admin Key。直接运行 API、请求确实来自本机回环地址且使用 `localhost` 或回环 IP 访问时可以直接初始化；其他连接必须在服务端配置至少 32 个字符的 `SUB2API_CONSOLE_SETUP_TOKEN`，并在初始化页输入相同令牌。Docker Compose 的浏览器请求会经过前端容器代理，API 侧不会把它识别为回环连接，因此首次使用 Compose 时也必须配置令牌，即使浏览器打开的是宿主机 `localhost`。令牌只通过 `X-Setup-Token` 请求头发送，不写入配置数据库，初始化完成后即不能再次使用该接口覆盖配置。

首次使用 Docker Compose 前生成一次性令牌：

```bash
export SUB2API_CONSOLE_SETUP_TOKEN="$(openssl rand -hex 32)"
docker compose up -d
```

初始化完成后可在下次重建 API 容器时从部署环境中移除该变量。控制台随后使用 HttpOnly 会话 Cookie 登录；Admin Key 只保存在后端 `data/console-config.sqlite3`，不会返回到浏览器。业务账号、分组、绑定、运行记录和告警只从 Console 自有业务库读取。

Docker 版前端通过同源 `/api` 反向代理到 API，远程访问时只需开放 `3004`（Compose 默认把 API 的 `8080` 仅绑定到宿主机回环地址）。生产环境应在反向代理层启用 HTTPS，并设置 `SUB2API_CONSOLE_COOKIE_SECURE=true`。若 TLS 在外层反向代理终止，还必须把该代理连接前端容器时使用的源地址或专用网段配置到 `SUB2API_CONSOLE_FRONTEND_TRUSTED_PROXY_CIDRS`，例如 `192.0.2.10/32`；默认空值不会采信客户端发送的 `X-Forwarded-For` 或 `X-Forwarded-Proto`。外层代理必须覆盖 `X-Forwarded-Proto` 为单个 `http` 或 `https` 值，并正确覆盖或追加经过验证的客户端地址。不要配置普通客户端网段、共享的不可信容器网段或 `0.0.0.0/0`；非法 CIDR 会使前端容器拒绝启动。

`SUB2API_CONSOLE_TRUSTED_PROXY_CIDRS` 是独立的 API 侧信任列表。Compose 默认不信任任何 TCP 来源；前端与 API 通过当前 Compose 项目专属卷中的 `/run/sub2api-console/api.sock` 通信，只有这个 Unix socket listener 会把请求标记为来自受信代理。同一 Docker 网络中的其他容器既不能通过重复 IP 冒充前端，也不能访问未挂载的 socket 卷。该方案不占用固定子网，多套 Compose 项目可以并行使用；宿主机端口可分别通过 `SUB2API_CONSOLE_API_PORT` 和 `SUB2API_CONSOLE_FRONTEND_PORT` 调整。显式配置 API 的 TCP 信任列表时，只能填写会规范化 `X-Forwarded-For`、`X-Real-IP` 和 `X-Forwarded-Proto` 的直接反向代理地址，优先使用单地址 `/32` 或 `/128`。若前端与 API 不同源，还需通过 `SUB2API_CONSOLE_FRONTEND_ORIGINS` 明确列出允许携带凭据的前端 Origin；不要使用通配符。

使用 Docker Compose 部署：

```bash
docker compose pull
docker compose up -d
docker compose ps
```

默认使用本仓库发布的 `ghcr.io/mienchating/sub2api-console-api:latest` 和 `ghcr.io/mienchating/sub2api-console-frontend:latest` 多架构镜像。生产环境建议通过 `SUB2API_CONSOLE_API_IMAGE`、`SUB2API_CONSOLE_FRONTEND_IMAGE` 固定到同一个版本标签，避免两个服务版本不一致。本地开发仍可使用 `docker compose up -d --build` 构建当前源码。

Compose 会等待 API 健康后再启动前端，并为两个服务配置自动重启和健康检查。API 容器启动时会调整挂载的 `./data` 和项目专属 socket 目录权限，随后以非 root 用户运行；不要把其他目录挂载到 `/app/data`。SSE 反向代理读写超时为一小时，长时间巡检不会被 Nginx 的默认超时截断。

## 发布

版本使用日期标签：当天首个版本为 `vYYYY.MM.DD`，后续版本依次为 `-2`、`-3`，禁止使用 `-1`。创建标签前必须提交 `.github/release-notes/<tag>.md`；具体硬性规则见 [发布说明流程](.github/release-notes/README.md)。

标签推送后，GitHub Actions 会先执行完整前后端检查和两个 Dockerfile 的预构建。全部通过后才会发布以下 amd64/arm64 镜像，并在两个镜像的多架构清单都验证成功后创建 GitHub Release：

- `ghcr.io/mienchating/sub2api-console-api:<tag>`
- `ghcr.io/mienchating/sub2api-console-frontend:<tag>`

## 验证

```bash
cd backend && go test -race ./...
cd frontend && bun run test && bun run typecheck && bun run lint && bun run build
```

首次克隆仓库后启用提交前检查：

```bash
git config core.hooksPath .githooks
```

`pre-commit` 会先检查暂存区空白错误，再执行与 CI 相同的前端格式检查。检查失败时提交会被阻止；运行 `cd frontend && bun run format` 修复格式后重新暂存并提交。

Console 不提供外部运行库实时读取或同步接口。主动探测只使用 Console 私有配置库中的授权信息和业务库中的账号记录；探测结果写回 Console 业务库。业务写回按当前控制台策略和权限执行，与 skills 数据库没有关联。
