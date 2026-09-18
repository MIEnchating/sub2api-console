# 账号实时流量

控制台通过受鉴权保护的 `/api/accounts/traffic`，由 Go 后端读取 Sub2API 的 `/api/v1/admin/ops/concurrency`。浏览器不直接访问管理接口。快照仅返回稳定账号 ID、当前请求数、排队数、计数跟踪能力及采集时间，不返回账号配置或凭据。

## 数据依据

依据本地 Sub2API 项目源码：

- `backend/internal/handler/admin/ops_realtime_handler.go`：并发接口要求开启运维监控和实时监控，返回 `enabled`、`timestamp` 和按 ID 索引的 `account`。
- `backend/internal/service/ops_concurrency.go`：`current_in_use` 来自账号当前并发，`waiting_in_queue` 单独统计；多分组账号在账号维度去重。
- `backend/internal/service/concurrency_service.go`：网关取得账号请求槽后开始转发，请求完成时释放；不限并发的路径可能不记录请求槽。
- `backend/internal/repository/concurrency_cache.go`：请求槽由 Redis 保存并过期清理。`ops_concurrency.go` 在读取失败时可能降级为零，因此零计数只能表示未观测到请求，不能证明空闲。

仅新鲜快照的 `current_in_use > 0` 标记“真实请求”。排队、历史请求、调度开启、活跃会话数、RPM 都不作为正在处理请求的判据。Console 探活及模型检测直接访问绑定上游，不经过 Sub2API 网关槽计数，不能用来制造此状态。此指标表示网关占用中的请求槽，不证明上游已开始生成；异常退出后未及时释放的槽可能等到过期清理才消失。

## 刷新与选择

账号管理、常规检测和动画检测使用相同查询资源，每 5 秒刷新。后端合并 3 秒内的读取，缓存绑定管理地址和凭据，目标变更后重新读取并校验。快照超过 15 秒不可用于选择；读取失败、监控关闭、账号缺失、不限并发零计数分别显示明确状态，不把未知值补零。

“选择实时流量”只替换当前筛选范围内的账号选择，最多 20 个，保留后续选择模型、范围确认和启动流程。动画检测跳过正在运行检测及不支持的平台。流量归零不会自动取消用户已选项或运行中的测试；下一次点击选择入口时使用最新快照。
