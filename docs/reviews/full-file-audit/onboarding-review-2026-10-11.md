# 开户剩余阅读与凭据回显修复（2026-10-11）

本次接续剩余阅读，补齐开户模块原有 18 项记录缺口，新增一项回归文件记录；原 14 项匹配记录复用。共 33 个模块文件，逐项记录见 [阅读台账](onboarding-review-2026-10-11.tsv)。只修复本次确认的凭据回显缺陷，不重开已验收容量分配、批量进度或开户流程优化。

## 确认缺陷与修复

上游将本次绑定 Key 放入普通输出或错误文本时，原有按字段名和 Bearer 格式的脱敏不能识别实际 Key，导致公共探活任务带回凭据。先用真实 `EnqueueProbe` 路径和隔离 HTTP 服务复现 JSON、SSE、业务失败、HTTP 失败及模型列表失败；未调用生产服务或真实凭据库。

修复在 JSON 解码后的字符串及字段名、模型值和错误详情中移除实际 Key，再截断和序列化；保留 JSON 数字精度、正常输出及可操作的错误原因。追加的失败用例确认长文本截断会留下 Key 片段、敏感字段名也可能含 Key，随后一并修复。最终十个场景通过，含 JSON 转义、输出及错误截断、字段名边界。仅保护本次凭据的直接回显与 JSON 转义，不声称识别任意编码或上游主动截断后的凭据片段。

本轮生产修改限于 `probe.go`；该文件已有动画和协议兼容改动完整保留。没有修改共享脱敏模块或动画执行规则。

## 阅读复用

`service_test.go` 复用 `backend-domains.tsv` 的已审 SHA-256 `09eb3a9819244a4ebe93104676d1676466d1e63f5206ad84994650efed424336`。已核对提交 `0824daf23147a185608bb4699296b749f15dcaec` 的内容匹配该哈希，再阅读当前唯一差异及 fixture 上下文：临时上游明确设置 `concurrency_limit=0`、`concurrency_status=unlimited`。没有重新阅读或重写整份约 2,900 行测试。

其他原有缺口沿用本次连续工作中已完成的全文阅读；登记前核对快照，只有本轮修复的 `probe.go` 变化。容量互斥、人工分组保护、稳定模型绑定、临时 Key 清理与兼容重试的已有行为均保留。

## 验证证据

原始输出及输入哈希位于 `/tmp/sub2api-onboarding-review-20261011/`。`credential-before.log`、`truncation-before.log`、`field-name-before.log` 为修复前失败证据，`credential-final.log` 为最终十场景通过记录。

最终受影响范围为 `./internal/onboarding ./internal/onboarding/__tests__ ./internal/api ./internal/api/__tests__ ./cmd/server`，使用 `bash scripts/check-go.sh test` 和 `vet`，明确包含独立测试目录与直接调用方。早期 `test.log` 的五包通过发生在最后边界修复前，不当作最终结果；以 `test-final.log`、`vet-final.log` 和 `validation-final-inputs.json` 为准。后端补充 `go build ./...`，输出 `build-final.log`；不启动服务器。

最终检查结果在同目录 `validation-results.json` 登记。未运行前端检查或全后端回归。并行外部工作已修改账号工作台以及相关 API、前端输入；旧工作台阅读哈希和旧前端构建结果不能描述为当前有效，本次不覆盖这些修改，也不将其自动重新加入实施队列。

最终五包竞态检查退出 0：开户普通包 90.737 秒、独立测试包 213.130 秒、服务入口 1.461 秒，三个包实际运行；两个 API 包由 Go 复用有效缓存。同范围 vet、后端构建、涉及文件 gofmt 和 diff 检查全部通过。最终 1,276 项后端验证输入前后哈希一致。本轮问题已关闭，开户约定范围退出实施队列；全局审查与其余缺失验证仍未完成。
