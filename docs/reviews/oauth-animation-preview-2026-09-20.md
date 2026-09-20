# OAuth 动画受控生成适配

## 问题与实现范围

Sub2API 账号详情返回 `credentials_status.has_access_token=true`，但移除了 `credentials.access_token` 原文。原动画任务把脱敏当成凭据缺失，在发送生成请求之前失败。

本次修改涉及 Console 和配套 Sub2API 两个工作区：

- Sub2API 新增受管理员鉴权保护的 `POST /api/v1/admin/accounts/:id/generate-preview`，接收模型、自定义提示词、推理强度、请求 ID 和超时。内部使用账号 OAuth 凭据和代理，固定请求官方端点，拒绝重定向和自动重放。
- 独立生成预览不调用普通账号测试流程，不自动刷新授权，也不写账号错误、恢复或额度状态；请求结果不参与健康或自适应并发统计。
- 后端只在收到完整生成结果后返回文本，拒绝空结果、截断或超大响应和凭据回显。HTTP 与流式错误在解码后脱敏。
- Console 在凭据被隐藏时使用此接口，核对稳定账号 ID 和请求 ID，继续执行 SVG 安全校验，并支持前置问题检测、原有组合任务和调度入口。
- 不支持此接口的旧版提示升级，不退回具有账号状态副作用的 `/test`。原文凭据可用时保留已有直连流程；独立行为画像检测不在本次适配范围内。

接口契约见配套 Sub2API 仓库的 `docs/account-generation-preview.md`；Console 行为与重试边界见 [动画检测失败重试](../animation-retry.md)。

## 验证记录

验证针对本次工作区代码，不代表某个提交的完整 CI 成功。Console 工作区还存在其他开发任务的变更，本次未提交、推送或部署。

| 检查 | 结果 |
| --- | --- |
| Console：新增隐藏凭据动画、前置问题、旧版接口提示用例，修复前运行 | 稳定失败，确认复现原问题 |
| Console：受影响 OAuth 动画、组合任务、管理客户端用例 | 通过 |
| Console：`bash scripts/check-go.sh vet` | 通过，包含 `__tests__` 包 |
| Console：`bash scripts/check-go.sh test` | 执行了完整竞态检查；账号控制、调度写入出现等待超时，动画和调度写入回归包触及默认 10 分钟总时限，其余包通过 |
| Console：`GOMAXPROCS=2 go test -race -p 1 ./internal/accountops ./internal/routingwrite ./internal/routingwrite/__tests__ ./internal/modelcheck ./internal/modelcheck/__tests__ ./internal/adminclient/__tests__` | 全部通过，首轮四个超时包均完成复核 |
| Sub2API：新增接口成功、失败、凭据脱敏、代理、输入校验、流式完成与大小限制用例 | 通过 |
| Sub2API：禁止重定向与重放的回归用例 | 先确认失败，修复后通过 |
| Sub2API：`go vet -p 2 ./...` 及最终受影响包静态检查 | 通过 |
| Sub2API：原有 OAuth、请求保护和插件相关 `-tags=unit` 用例 | 通过 |
| Sub2API：`go test -race -p 2 ./...` | 完整执行但未通过；service 包存在既有数据竞态和 WebSocket 用例超时，见下文；此轮包含修复前的重定向回归失败，最终新增接口回归已单独通过 |
| Sub2API：最终代码 `GOMAXPROCS=2 go test -race -p 1 ./internal/service ./internal/handler/admin -run TestGenerationPreview -count=1` | 通过 |
| Sub2API：golangci-lint v2.13.0，覆盖 service、admin handler、routes 包 | 最终结果 0 issues；已修复首轮三处 errcheck 问题 |
| 两仓库：涉及 Go 文件 `gofmt`、`git diff --check` | 通过 |

没有修改 TypeScript/TSX，因此未运行前端类型检查或界面回归。测试使用隔离 HTTP 服务、内存模拟传输或临时数据库；未使用真实账号发起生成请求。

### 尚未通过的既有全量用例

针对下列测试在最终工作区执行 `GOMAXPROCS=2 go test -race -p 1 ./internal/service -run '^(TestAccountHealthSettingsWakeWorkerAndResetInterval|TestUsageCleanupServiceExecuteTaskDashboardRecomputeError|TestOpenAIGatewayServiceRecordUsage_SparkShadowUsesCurrentParentBillingSetting|TestOpenAIGatewayServiceRecordUsage_ShadowUsesParentCredentialTierContract|TestOpenAIWSHTTPBridgeAcceptsFirstFrameAboveLegacy16MiB)$' -count=1`，仍可复现失败：

- `TestAccountHealthSettingsWakeWorkerAndResetInterval`：`account_health_test.go:130` 注册 SQL mock 期望时，后台健康 worker 同时查询该 mock。
- `TestUsageCleanupServiceExecuteTaskDashboardRecomputeError`：`usage_cleanup_service_test.go:583` 读取测试 stub 的计数时，后台重算协程同时写入。
- 两个 `TestOpenAIGatewayServiceRecordUsage_*` 影子账号计费用例报告数据竞态。
- `TestOpenAIWSHTTPBridgeAcceptsFirstFrameAboveLegacy16MiB`：`openai_ws_http_bridge_test.go:1921` 报告 `context deadline exceeded`。

这些测试及对应业务文件没有本次改动，复核不调用新增生成预览入口。新增功能的最终竞态用例、相关旧流程单元测试和静态检查通过，但不能据此宣称整个 Sub2API 仓库的完整质量门禁通过。

## 生效条件

先发布配套 Sub2API，再更新 Console；仅更新 Console 无法让旧管理端返回隐藏的 OAuth Token。新入口当前支持直接保存凭据的 OpenAI OAuth 账号，不支持凭据影子、Agent Identity、Prism 和图片模型。生成请求仍消耗正常上游用量。

本次未部署或重启线上服务。
