# 后端增量复核

审查范围：`backend/internal/api`、`backend/internal/configstore`，以及账号工作台的 `export*`、`source_profile*` 和 `cleanup_files.go`。新增和变化文件逐个完整读取，包含 `api/server.go` 与 `configstore/store.go`；内容哈希记入对应审查台账。

## 已修复问题

### 本地登录资料导出返回错误范围

`source_profile_export.go` 将本地登录资料写入 `local-export` 私有命名空间，但任务成功结果调用托管导出报告函数，返回 `scope: managed`，造成任务中的产物归属与实际文件不一致。

已改用 `localExportMetadataReport`。新增 `source_profile_export_scope_test.go`，通过真实配置存储、任务和私有文件目录验证任务范围、产物 ID 和本地文件列表一致，且不访问管理目标。修复前该用例稳定失败并输出 `scope: managed`，修复后通过。

### 新增测试使用错误的现有接口签名

持续开发新增的 `source_profiles_test.go` 与 `source_profile_authorization_test.go` 分别向 `DeleteLocalExport` 和 `ReadOAuthBatch` 多传一个参数，导致整个账号工作台测试包无法编译。已按照现有服务、API 调用及其他测试的契约修正参数，没有改变生产接口。

## 验证

- `go test -race ./internal/accountworkbench/__tests__ -run '^Test(SourceProfile|LocalExport|LocalRefreshInput)' -count=1 -timeout=120s`：通过，19.021 秒。
- 新增资料重新授权及安全结果测试后，再执行 `go test -race ./internal/accountworkbench/__tests__ -run '^TestSourceProfile' -count=1 -timeout=120s`：通过，19.705 秒。
- 补读最新资料授权测试中的导出和短信确认场景后，再执行全部 `TestSourceProfile` 竞态测试：通过，18.389 秒。
- `go test -race ./internal/configstore/__tests__ -run '^TestWorkbenchSourceProfile' -count=1 -timeout=90s`：通过，1.559 秒。
- `go test -race ./internal/api/__tests__ -run '^TestWorkbench(Local|Regeneration)' -count=1 -timeout=120s`：通过，17.788 秒。
- 新增 `account_workbench_source_profiles_test.go` 后，执行 `go test -race ./internal/api/__tests__ -run '^TestWorkbenchSourceProfile' -count=1 -timeout=120s`：通过，9.613 秒。
- API、配置存储、账号工作台及其 `__tests__` 包的 `go vet`：通过。
- 修改文件的 `gofmt` 和 `git diff --check`：通过。

整个后端的完整测试、竞态检查与构建由主审查任务汇总。

## 完整竞态检查发现的测试竞争

完整 `check-go.sh test` 执行时，`upstreamdelete/service_test.go` 的 `TestDeleteReportsAccountsRemovedBeforeALaterRemoteFailure` 报告两个 HTTP 处理器同时读写 `deleted` map。该夹具仅需要记录账号 41 是否已删除，已改为 `atomic.Bool`，并在读取时明确匹配账号 41，保留账号 42 的失败路径。

该数据竞争在完整竞态检查中已实际复现，之后单独重复原用例未再次触发。修复后 `go test -race ./internal/upstreamdelete -count=10` 全包连续十次通过，用时 1.746 秒；该包 `go vet` 通过。未修改生产删除逻辑。

## 最终后端检查

2026-09-14 21:09 UTC，最终 `GOFLAGS=-p=4 bash scripts/check-go.sh test` 进程退出码为 0，67 个有测试的包通过，包含全部 28 个自动发现的 `__tests__` 回归包。工作台回归包用时 496.348 秒，API 回归包 105.206 秒，上游删除包 1.109 秒；其余结果包含 Go 的有效测试缓存。日志中没有失败或数据竞争报告。

最新完整 `GOFLAGS=-p=4 bash scripts/check-go.sh vet`、`GOFLAGS=-p=4 go build ./...` 均通过。结束后再次核对全部后端文件，未审文件和已审内容变化均为 0。最终完整竞态日志位于 `/tmp/sub2api-full-audit/final-backend-race.log`。
