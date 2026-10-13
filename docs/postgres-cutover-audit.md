# PostgreSQL 切换后兼容性排查（2026-10-12）

本轮由上游编辑读取失败触发，检查运行工作树 `sub2api-console-cutover` 的数据库访问层、业务库、私有配置库、任务库与离线导入。修复同步到原工作区对应业务代码，保留两边已有的其他修改。

## 确认并修复的问题

| 路径 | 触发条件及原错误 | 修复与回归 |
| --- | --- | --- |
| `business/catalog.go` 的关联账号读取 | PG 将 `a.id IS NOT NULL` 返回为 bool，扫描到 int64 导致上游编辑失败 | 直接扫描到 `AccountExists`；覆盖账号存在、已删除、旧绑定和失效绑定 |
| `business/cleanup.go` 的删除范围复核 | 两个 `EXISTS` 结果扫描到 int，PG 删除预览及执行复核失败 | 使用 bool；覆盖唯一绑定通过、Key 变化拒绝、开户待续引用拒绝 |
| `business/onboarding.go` 的 Key 保护 | `EXISTS` 扫描到 int，PG 保护查询与未绑定 Key 投影清理失败 | 使用 bool；覆盖已绑定与空闲 Key、事务内投影清理 |
| `business/catalog.go` 的上游目录读取 | 主结果集尚未消费便查询派生名称与并发总量；单连接池超时，PG 快照报 `conn busy`，MySQL 快照报 `driver: bad connection` | 补充查询先完成，再打开主结果集；覆盖单连接编辑读取及同一只读快照读取 |

失败用例均在对应修复前执行。三库回归位于 `backend/internal/business/__tests__/database_upstream_configuration_test.go`、`database_key_protection_test.go` 和 `database_catalog_connections_test.go`。原工作区编辑服务回归位于 `backend/internal/upstreamconfig/__tests__/configuration_bindings_test.go`。

## 审查范围与结论

| 模块 | 检查重点与证据 |
| --- | --- |
| `dbconn`、schema 与迁移 | 参数绑定、引号与占位符、PG `RETURNING`、事务与快照、连接池、迁移边界；复核数据库契约中的初始化、并发与回滚路径 |
| `business` 目录、绑定、开户、清理 | 核对布尔表达式的扫描目标，保留 BIGINT 状态列的整数读取；修复上述四处路径，检查主查询与补充查询的连接生命周期 |
| `business` 调度、健康、倍率、告警、历史 | 核对 JSON/时间/数值排序方言与 NULL 处理；抽查多次查询、事务内读取及写入前结果集关闭；现有三库契约覆盖调度、证据补全、锁、历史与并发写入 |
| `configstore` | 复核配置版本更新、会话/字典事务、私有文档 JSON、二进制前缀范围与排序；现有配置契约通过 |
| `taskstore` | 复核历史分页与空页计数、JSON 摘要、恢复与压缩、清理时结果集释放；现有任务契约通过 |
| `dbtransfer` | SQLite 专用 schema/rowid 读取仅用于源库，目标按方言写入；现有隔离导入契约通过 |

搜索结果仅用作定位线索；不能把关键词命中数当作逐文件审查覆盖率。三库契约不等于所有历史 SQLite 测试都已在 PG/MySQL 执行。此次没有改变 schema、清理业务数据或调用真实上游执行测试。

## 验证记录

- `bash backend/scripts/check-db-contract.sh all`：三库连接 13.431s、配置 50.519s、任务 6.982s、业务 58.798s、离线导入 63.064s 均通过；这轮已包含 Key 保护修复，尚不包含随后新增的连接占用回归。
- 新增连接占用用例修复前：PG/MySQL 单连接超时、快照查询失败，SQLite 通过。修复后的业务契约另行执行；连接、配置、任务及导入代码未变化，复用上列有效结果。
- 最终 `bash backend/scripts/check-db-contract.sh business` 通过，57.245s；包含新增的单连接与快照用例。
- 运行工作树 `bash scripts/check-go.sh test` 全量竞态回归通过（显式自动发现 `__tests__`）：81 个测试包，36 个实际执行、45 个复用有效缓存。业务主包 210.790s、业务独立回归 837.214s；模型检测、开户、探活、调度、写回与 API 调用方全部通过。
- 运行工作树全量 `bash scripts/check-go.sh vet`、所改 Go 文件 gofmt 检查和 `go build -buildvcs=false -o /tmp/console-pg-scan-server ./cmd/server` 通过。
- 原工作区受影响六包 vet 通过；使用 `GOFLAGS=-run=Test(ConfirmAccountDeleteScope|ReconcileDeleted.*KeyProjection|DeleteAccountProjectionWithScope|UpstreamConfiguration|UpstreamAndGroupCatalogs|UpstreamGroups|ConfigurationReadPreservesBoundAccountAvailability)` 配合 `scripts/check-go.sh test` 执行定向竞态回归：business 6.722s、upstreamconfig/__tests__ 8.034s 通过；另外两个列入的包没有匹配用例，不计为测试覆盖。
- 2026-10-12 17:22:54 UTC 重启现有后端服务，新 server PID 为 1506085，工作目录与 cgroup 确认为 cutover 后端服务。服务 active、自动重启数为 0；8080 后端及 3004 前端代理的 `/api/setup/status` 返回 HTTP 200 和有效 JSON。启动后日志未命中 SQLSTATE、Scan error、conn busy 或 panic。未直接操作已登录浏览器的编辑弹窗，不把启动接口检查当作编辑接口验证；编辑业务路径由上述真实三库服务回归覆盖。

测试服务器由契约脚本临时创建，使用 25432/23306 端口，脚本退出清理容器；不会连接运行环境的 15432 端口。测试日志保存在本机 `/tmp/console-pg-*`，属于临时诊断材料。
