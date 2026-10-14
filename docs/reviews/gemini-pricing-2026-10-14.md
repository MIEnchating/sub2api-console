# Gemini 官方价格接入

新增官方来源 https://ai.google.dev/gemini-api/docs/pricing ，使用既有公开 HTTP 请求、超时、重试和同源跳转保护。平台模型目录或远程价卡中的 Gemini 模型触发官方抓取；官方结果替换第三方 Gemini 参考价，明确别名来自模型标题旁的 code ID，不推测日期版本或代理前缀。沿用现有缓存刷新入口，无数据库结构变更。

## 支持范围

- Standard 付费档文本输入、输出（含思考 Token）、缓存读取及输入上下文阶梯。金额保留 USD 原值，精确转换为每 Token 单价。
- 日期价格保留前后两档及官方日期，使用现有价格档位展示。官网未注明切换时区，日期条件仅用于展示，不生成计费表达式；通过 sync_error 和空倍率阻止自动同步成永久固定价。用户可核对日期后手动配置。
- 音频、Live、TTS、转写、图片生成、Omni、Embedding 不在本次文本计价范围内；缓存按小时存储费用不冒充缓存写入 Token 价格。范围说明随官方价卡展示。
- 页面格式异常、缺失标准表格、金额或档位边界不完整时拒绝部分结果，保留上次成功缓存；首次失败不使用第三方 Gemini 价格冒充官方结果。

## 验证

修复前用例复现 Gemini 来源未注册及 Fetch 不支持。修复后，本次抓取的完整官网页面成功解析 13 个标准文本模型及明确别名，包括 gemini-4-argon。自动测试使用本地 fixture、自定义 transport 和临时数据库。

在 backend 执行以下受影响检查，均通过：

```sh
bash scripts/check-go.sh test ./internal/officialpricing ./internal/officialpricing/__tests__ ./internal/newapimanagement ./internal/newapimanagement/__tests__
bash scripts/check-go.sh vet ./internal/officialpricing ./internal/officialpricing/__tests__ ./internal/newapimanagement ./internal/newapimanagement/__tests__
go build -o /tmp/sub2api-gemini-server-check ./cmd/server
```

三套测试本次执行通过：officialpricing/__tests__ 2.057 秒、newapimanagement 5.568 秒、newapimanagement/__tests__ 33.299 秒。officialpricing 生产包编译通过，无本包测试。

前端 gemini-pricing、official-tiers、partial-reference-prices 三个测试文件共 7 项通过；bun run typecheck、新增测试文件 ESLint 和 oxfmt 通过。受影响 Go 文件 gofmt 和 diff 空白检查通过。未执行全仓回归。

核对 PID、工作目录、8080 监听和 systemd cgroup 后重启现有后端开发服务；重启后 active/running，接口返回预期的未登录 401。未使用用户登录会话触发真实平台价卡刷新；用户点击价格刷新即可更新现有缓存。未提交、推送或发布镜像。
