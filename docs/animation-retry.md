# 动画检测失败重试

动画生成在同一任务内最多发起三次请求（首次加两次自动重试），仅针对连接重置、事件流意外结束、HTTP 408/429/500/502/503/504 等明确的短暂失败。单次请求或流读取超时不会重新发起整次生成，避免默认 120 秒超时累积成约 361 秒。前置检测不参与该重试流程。重试继续使用原账号、模型和生成指令，每次重新核对账号配置与凭据，并使用带 `-retry-N` 后缀的请求 ID。账号在整个任务期间保持占用，生成并发仍受原有上限约束。

默认等待 250ms、500ms；有有效 `Retry-After` 时遵守服务端等待时间。要求等待超过 15 秒时保留失败结果，交由用户稍后重试，不缩短服务端要求的间隔。取消任务或任务总超时会停止等待与后续请求。

鉴权错误、明确的额度不足、生成内容截断、外部资源引用、敏感信息回显等不会自动重试。中断的部分 HTML/SVG 不拼接、不展示；最终错误仍脱敏保留上游原因。结果的 `retry_count` 表示实际自动重试次数，在检测详情展示；旧记录缺失该字段时按未重试处理。自定义接口沿用同一任务内存中的凭据，不将凭据保存到任务或浏览器。

## OAuth 受控生成

当 Sub2API 账号详情通过 `credentials_status.has_access_token` 表示凭据存在、但不返回 Token 原文时，动画和前置检测改用受管理员鉴权保护的 `POST /api/v1/admin/accounts/:id/generate-preview`。Console 发送稳定账号 ID、模型、提示词、推理强度、请求 ID 和超时；Sub2API 使用其内部 OAuth 凭据及账号代理执行生成，确认上游流式完成后返回文本。Console 核对返回的账号 ID 和请求 ID，并继续执行 HTML/SVG 安全校验。

该入口需要配套 Sub2API 版本，不读取或导出 OAuth Token，不调用具有账号恢复或错误标记副作用的 `/test`。旧版 404/405 提示升级并刷新账号；账号原文 Token 仍可用时保留原直连流程。独立行为画像检测不在此次适配范围内。

受控生成不参与上述自动重试：管理连接中断时无法确认生成是否已经执行，错误保留给用户手动重试；Sub2API 也不重放 429。取消传递至上游，成功或失败均不写账号健康、恢复或额度状态，生成仍会产生正常用量。部署顺序为先升级 Sub2API，再升级 Console。

## 对话复用与断点续传

OpenAI 官方文档说明，可以通过再次传入消息历史，或使用支持的 `previous_response_id` 延续对话；`store: false` 也可由客户端维护消息历史。因此，“重试保留已有对话”是可行的，并不等于服务器能够从断流位置继续原来的生成。

当前动画检测只有一条生成指令。自动重试保留这条原始输入，重新生成完整结果，不把失败的部分输出当作已完成对话，也不伪造上游 response ID。OAuth 路径继续使用 `store: false`，不启用新的服务器会话存储。该实现不承诺断点续传、提示缓存命中或免除重复请求用量；重试仍可能产生 API 用量。

参考：

- [Conversation state](https://developers.openai.com/api/docs/guides/conversation-state)
- [Codex configuration reference](https://developers.openai.com/codex/config-reference)：分别定义 HTTP 请求和 SSE 中断的重试次数，不能据此推断所有兼容接口均支持恢复同一条生成流。

## 固定参数与结果统计

新检测固定使用 `gpt-6-astra`，前置检测 `medium`，鹈鹕动画 `low`。动画提示词要求单文件 HTML 与内联 SVG，可使用内联 JavaScript 驱动动画，不得主动发送网络请求；后端保留原文与实际提示词，预览保留模型返回的 HTML、CSS、SVG 和内联脚本，拒绝脚本外链、事件属性和外部资源引用。HTML 在仅允许脚本的 opaque-origin sandbox 中运行，CSP 禁止 Fetch 等网络连接、表单提交和子框架，沙箱禁止顶层导航；CSP 不能阻止脚本导航 iframe 自身，不能保证任意脚本绝对不会发出导航请求。旧 SVG 结果继续以图片展示。

Responses 用量从 JSON 或完成事件读取，Chat Completions 请求 `stream_options.include_usage` 并读到 `[DONE]` 或正常 EOF，保留 finish_reason 后的用量块；Messages 合并 message_start 与 message_delta 中报告的用量。受控 OAuth 预览允许可选 `usage.input_tokens/output_tokens/total_tokens`。缺失用量不补零，仅在输入与输出都存在时计算总计；TPS 为输出 Token / 最终一次生成耗时，不将失败重试和退避计入分母。任务总耗时仍包含各次请求和退避。
