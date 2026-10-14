# Qwen 官方价格刷新失败修复

本次因用户报告的“官方价格表超出边界”重新打开官方价格模块，范围限定为 Qwen 表格筛选及价格缓存回归。

## 原因与修复

2026-10-14 抓取的阿里云官方价格页面包含 MiniMax-M2.5 表格，其最后一行免费额度单元格声明 `rowspan=2`，但实际只剩一行。旧 Qwen 解析器先对北京区域所有匹配价格表头的表格执行严格网格校验，再过滤模型，导致无关表格阻断 Qwen 刷新。

现在在展开网格前检查表格是否包含已支持的 Qwen 文本计价模型，复用原有模型排除规则；无关表格不参与解析。相关表格仍保留跨度、边界及价格校验，失败不返回部分结果。未改变通用表格边界、价格计算、数据库或前端。官网故障表格原文片段保存在 `backend/internal/officialpricing/__tests__/testdata/qwen-unrelated-table.html`。

## 验证

- 修复前新增用例稳定复现无关表格放在有效表格前后均报边界错误，以及不支持的音频表格阻断刷新。
- 修复后官方价格模块及价格管理模块的竞态回归通过，显式包含各自 `__tests__` 包；验证恢复清除 stale、无关表格不阻断、相关表格异常保留上次完整价格。
- 同范围 `go vet`、所改 Go 文件 `gofmt`、`git diff --check` 通过。
- 在 backend 目录执行 `go build -o /tmp/sub2api-qwen-server-check ./cmd/server` 通过。
- 独立读取本次抓取的完整官网页面，修复后的真实解析器成功解析 54 个模型。自动化测试仅使用本地 fixture 和隔离测试依赖，不访问生产数据库或真实上游。

执行命令（backend 目录）：

```sh
bash scripts/check-go.sh test ./internal/officialpricing ./internal/officialpricing/__tests__ ./internal/newapimanagement ./internal/newapimanagement/__tests__
bash scripts/check-go.sh vet ./internal/officialpricing ./internal/officialpricing/__tests__ ./internal/newapimanagement ./internal/newapimanagement/__tests__
```

三套测试包本次执行通过：officialpricing/__tests__ 1.962 秒、newapimanagement 5.466 秒、newapimanagement/__tests__ 30.410 秒；生产 officialpricing 包编译通过且无本包测试。未执行全仓回归，未修改 TypeScript。
