# 已收口模块：官方价格后端（2026-10-11）

账号管理前端本轮范围已收口，日志保持冻结。继续按模块集中审查、修复和受影响验证。

入口 55 文件中 13 项哈希匹配复用，42 项已完整补审。新增两个回归文件后为 57/57；两项修复及受影响三套竞态回归、四包 vet 已通过，详见 [模块报告](../officialpricing-2026-10-11.md)。下表保留进入模块时的缺口状态，不再代表待办。重点为官网请求/解析、价格缓存、条件与精度保留、峰谷价格及失败降级；使用隔离 HTTP fixture，测试不访问生产服务或真实上游。

| 入口状态 | 文件 |
| --- | --- |
| pending | `backend/internal/officialpricing/__tests__/claude_context_test.go` |
| pending | `backend/internal/officialpricing/__tests__/claude_duplicates_test.go` |
| changed | `backend/internal/officialpricing/__tests__/providers_test.go` |
| pending | `backend/internal/officialpricing/__tests__/testdata/claude.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/grok.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-fable-5-1.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-fable-5.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-haiku-4-5.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-haiku-5-5.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-mythos-5-1.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-mythos-5.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-opus-4-5.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-opus-4-6.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-opus-4-7.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-opus-4-8.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-opus-5-5.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-opus-5.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-sonnet-4-5.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-sonnet-4-6.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-sonnet-5-5.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/claude-sonnet-5.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/gpt-5.4-pro.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/gpt-5.4.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/gpt-5.5-pro.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/gpt-5.5.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/gpt-5.6-luna.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/gpt-5.6-sol.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/gpt-5.6-terra.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/gpt-6-astra.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/gpt-6-luna.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/model-details/gpt-6-sol.md` |
| pending | `backend/internal/officialpricing/__tests__/testdata/openai.md` |
| pending | `backend/internal/officialpricing/__tests__/western_providers_test.go` |
| pending | `backend/internal/officialpricing/claude.go` |
| changed | `backend/internal/officialpricing/deepseek.go` |
| pending | `backend/internal/officialpricing/grok.go` |
| changed | `backend/internal/officialpricing/kimi.go` |
| pending | `backend/internal/officialpricing/markdown_pricing.go` |
| pending | `backend/internal/officialpricing/openai.go` |
| pending | `backend/internal/officialpricing/openai_images.go` |
| changed | `backend/internal/officialpricing/providers.go` |
| changed | `backend/internal/officialpricing/tiers.go` |
