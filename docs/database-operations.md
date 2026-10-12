# 数据库配置、升级和迁移

## 支持范围

默认 PostgreSQL；也可显式选择 MySQL 或 SQLite。隔离契约矩阵使用 PostgreSQL 18、MySQL 8.4 与 Go 驱动内置 SQLite，其他版本不在已验证范围。Go 后端继续使用 `database/sql`；三个 Store 在服务器上共用一个**专用数据库**，SQLite 保留三个文件。不要与其他应用共用表空间。

服务仍是单实例调度。PG/MySQL 服务入口持有数据库会话锁，第二个控制台或迁移进程拒绝启动；锁连接断开会取消后台任务并关闭服务。该保护不构成高可用选主或外部请求的绝对 fencing。事务仅覆盖本地写入，不包含网络请求；恢复外部操作仍按原有任务恢复规则核对目标和版本。

## 新部署

默认 `docker-compose.yml` 包含 API 和 PostgreSQL 18。复制 `.env.example`，分别执行两次 `openssl rand -hex 32`，设置独立的 `SUB2API_CONSOLE_POSTGRES_PASSWORD` 和 `SUB2API_CONSOLE_SETUP_TOKEN`。内置库不映射宿主端口，数据库数据位于 `postgres-data` 命名卷；`./data` 用于本地文件及旧库检测。不要执行会删除数据卷的 `docker compose down -v`。

```sh
docker compose config --quiet
docker compose up -d
```

直接运行开发服务时通过进程环境设置：

| 变量 | 含义 |
| --- | --- |
| `SUB2API_CONSOLE_DB_DRIVER` | `postgres`（默认）、`mysql`、`sqlite` |
| `SUB2API_CONSOLE_DB_DSN` | PG URL 或 MySQL DSN；服务器数据库必填，不回退 SQLite |
| `SUB2API_CONSOLE_DB_MAX_OPEN_CONNS` | 每个 Store 最大连接数，默认 16 |
| `SUB2API_CONSOLE_DB_MAX_IDLE_CONNS` | 每个 Store 空闲连接数，默认 4，不大于最大连接数 |
| `SUB2API_CONSOLE_DATA_DIR` | 本地数据目录，也用于检查默认旧库路径 |

服务器有三个独立池，默认合计上限 48，实例锁另占一个连接。迁移和启动初始化会短暂使用额外连接。SQLite 始终保持单写池和独立只读池，不使用服务器池并发设置。

PG URL 形状为 `postgres://用户:经过URL编码的密码@主机:5432/数据库?sslmode=verify-full`；MySQL DSN 形状为 `用户:密码@tcp(主机:3306)/数据库?tls=true&charset=utf8mb4`。本地隔离开发环境可按实际情况设置 TLS，远程部署配置证书验证。不要把凭据提交到仓库。Go 开发命令不会自动读取 Compose `.env`。

实例与迁移互斥使用会话锁，必须直连数据库或使用保持会话绑定的代理；不支持 PgBouncer 的 transaction/statement pooling 模式。禁用预处理缓存不能替代会话绑定。

MySQL 使用 InnoDB、utf8mb4 和区分大小写的比较。账号必须具备专用库内的 DML、建表/索引、ALTER、触发器和存储函数权限。启用 binlog 的服务器需要管理员按部署策略配置 `log_bin_trust_function_creators=1`，或提供适当的函数创建权限；应用不会自行修改全局设置。禁用 `multiStatements`，迁移逐条执行。

外部 PG/MySQL 使用以下覆盖文件，关闭内置 PG 依赖（Compose 2.24+）：

```sh
docker compose -f docker-compose.yml -f docker-compose.external.yml config --quiet
docker compose -f docker-compose.yml -f docker-compose.external.yml up -d
```

在 `.env` 中设置相应 driver 和完整 DSN。仅更换 DSN 不会自动迁移数据。

## 保留旧 SQLite

显式设置 `SUB2API_CONSOLE_DB_DRIVER=sqlite`、清空 `SUB2API_CONSOLE_DB_DSN`。Compose 同样使用上述 external 覆盖文件。默认文件仍为 `data/console-config.sqlite3`、`data/sub2api-console.sqlite3`、`data/tasks.sqlite3`；可分别通过原有 `CONFIG_DB`、`DATA_DB`、`TASK_DB` 环境变量配置绝对路径。网络数据库不能同时设置这些 SQLite 路径变量。

SQLite 配置库、任务库记录 `PRAGMA user_version` 基线，拒绝高于当前程序的版本；首次建表/补建索引在事务内提交。业务库继续使用既有显式旧库验证与升级规则。不兼容结构报错，不能通过清空数据启动。

## SQLite → PG/MySQL

这是离线操作，必须先停止所有源库写入，包括后台任务。目标必须是新建、专用于 Console 的空数据库；已有表、失败导入或已有业务数据都会拒绝。命令只接受环境变量 DSN，不接受命令行密码。

先用 Go 构建工具（发布镜像也包含 `/usr/local/bin/db-migrate`）：

```sh
cd backend
go build -o /安全工具目录/db-migrate ./cmd/db-migrate
```

安全配置目标 `SUB2API_CONSOLE_DB_DSN` 后执行；下面的目录均替换为实际绝对路径，备份目录和报告文件必须尚不存在：

```sh
/安全工具目录/db-migrate \
  -source-stopped \
  -driver postgres \
  -config /源数据/console-config.sqlite3 \
  -business /源数据/sub2api-console.sqlite3 \
  -tasks /源数据/tasks.sqlite3 \
  -backup-dir /私有备份/本次迁移 \
  -report /私有备份/本次迁移报告.json
```

MySQL 只需改 `-driver mysql` 并设置其 DSN。迁移期间不要重启源控制台。工具按以下顺序执行：

1. 取得目标实例锁，再持有三个源库的写锁；通过 SQLite 备份 API 复制已提交的 WAL 数据。原始备份文件权限 0600，目录 0700，不覆盖现有文件。
2. 在备份目录的 `prepared/` 副本上执行当前旧库升级规则，原库及原始备份保持不变；检查完整性和外键。副本可能按现有升级规则补建表、规范化旧数据或同步内置字典。
3. 记录目标 `console_data_import` 的 pending 状态，创建三模块结构。任何中断或 MySQL 部分 DDL 均保留启动阻止标记。
4. 在单个数据事务中按关联顺序导入，保留主键、原始 JSON、凭据、零 ID、目录插入顺序；重建恢复时间索引、任务历史投影、本地负数事件计数器并对齐自增序列。
5. 对准备后的每张源表和目标表核对行数、主键顺序及长度分隔的 SHA-256 内容摘要。类型、长度、唯一性、关联或摘要不兼容即失败，不截断或跳过。报告仅含模块、表名、行数和整表摘要，不含凭据或业务行。
6. 核对全部成功后，与数据一起提交 complete 标记。提交结果不明时报错，需检查目标标记，不能盲目重试。

启动新服务时保留原数据目录有助于检测误切换：发现旧 SQLite 文件且目标没有完成标记，会拒绝启动。对于曾使用自定义路径的旧库，迁移时务必传入真实路径；服务无法自动发现任意目录中的旧库。

导入失败后保留源库和报告，检查不兼容的表；使用另一个新空库重新演练。工具不会自动删除失败目标。完成后检查登录、账号/分组、任务历史、恢复与调度配置，再恢复正常运行。新库一旦接收写入，旧库即成为过期快照，不能直接切回而不处理新增数据。

## 版本迁移、失败恢复与降级

PG/MySQL 在 `console_schema_migrations` 按模块保存连续版本、语句校验和、dirty 状态。启动迁移持有专用会话锁；三模块全部完成前不会启动调度任务。程序拒绝未来版本、修改过的历史迁移以及 dirty 记录。

PG 的本项目结构迁移在事务内执行，失败回滚。MySQL DDL 可能隐式提交，失败保留 dirty 并拒绝下次启动；**不能把 dirty 直接改成 0，也不能删除迁移记录伪装成功**。当前恢复流程为：停止服务，保留失败库供诊断，修复迁移问题，再从升级前的完整备份恢复到新的专用库并重新升级；导入失败则使用原始 SQLite 备份重新导入空库。本版本不提供猜测结构的自动 repair。

部署升级前仍需备份。PG 可用配置好凭据的 `pg_dump --format=custom` 并用 `pg_restore` 演练恢复；MySQL 使用包含 routines/triggers 的一致性逻辑备份或经过验证的物理备份。SQLite 必须处理 WAL，可停止服务后备份三个完整数据库，或使用 SQLite 备份 API，不能运行中只复制主文件。

事务、锁和版本条件保护的是原子性、并发约束与陈旧编辑，不会恢复已提交的删列数据。当前没有任意版本 down migration：回退程序前必须确认结构兼容；不兼容时恢复对应版本备份。优先采用增加字段、过渡兼容、最后清理旧字段的升级顺序。

## 验证

`backend/scripts/check-db-contract.sh all` 在隔离 PostgreSQL 18/MySQL 8.4 容器上运行宿主 Go 竞态契约，并包含完整 SQLite 导入演练；容器仅作为测试依赖。普通后端完整回归使用 `bash scripts/check-go.sh test`，包含独立 `__tests__` 包。两种入口均不得指向生产库。契约覆盖不等于生产规模的容量或性能保证。
