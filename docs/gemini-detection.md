# Gemini 检测支持

Gemini API Key 账号支持动画、前置糖果题、多模型、账号自动计划及分组任务。接入前账号连接测试复用相同协议，并保留用户选择的模型、提示词和临时 Key 清理。自定义接口可以选择字典中已启用的 Gemini；其凭据仍只在执行内存中使用。Gemini OAuth 不在本次支持范围内。

Go 后端使用原生 `POST /v1beta/models/{model}:streamGenerateContent?alt=sse`，通过 `x-goog-api-key` 请求头鉴权。Base URL 可为根地址或带 `/v1`、`/v1beta` 的代理地址；已有代理路径保留。模型 ID 可带 `models/` 前缀，但不得包含路径穿越或查询参数。原生接口不可用时直接报告失败，不回退其他平台协议。

流式及完整 JSON 响应均需确认 `finishReason: STOP`。只解析正文，忽略 `thought` 内容；安全拦截、输出截断、缺少完成标记和错误事件均不得作为成功结果。保留 `modelVersion` 和实际用量，Gemini 输出用量与思考用量分开；总用量未返回且思考用量未知时不推算总量。不同 Gemini 模型的思考参数不兼容，因此沿用上游默认配置，不发送 OpenAI/Anthropic 的思考参数，也不把 low/medium 写成已生效参数。

账号及自定义接口模型读取使用原生 `/v1beta/models`、`pageToken` 分页；只列出声明支持 `generateContent` 或 `streamGenerateContent` 的模型。普通接入前模型下拉沿用现有上游模型发现逻辑。后台任务的取消、恢复、账号互斥和动画隔离展示规则保持一致。
