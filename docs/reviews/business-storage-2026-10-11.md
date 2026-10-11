# 业务存储：批量读取与元数据事务（2026-10-11）

本轮范围是 `backend/internal/business`。日志保持冻结；调度与模型检测已收口的功能没有追加优化。本报告只归属下述三处修改，工作区其他已有改动继续保留。

## 修改与回归证据

1. `AccountModelCatalogs` 将每账号一次查询改为按请求 ID 驱动的单条有序 LEFT JOIN；只解析所选账号，保留去重、请求顺序和首个不存在或元数据损坏的错误。模型同步预览共用读取快照，屏蔽规则与探测模型只读一次控制面策略。
2. `DetectionTaskScope` 复用调用方快照，一次扫描取得所选分组名称，再读取去重的账号归属；保留重复远端分组 ID 的首个标签、请求顺序中的首个缺失错误和稳定账号排序。空范围仍返回已初始化的空集合。旧实现的嵌套快照用例先失败；中间实现遗漏空范围的情况已修复。
3. `ClearRoutingRuntimeBlocks` 在同一写事务中读取和更新元数据，避免从旧调用方快照读出过期值后覆盖新提交。共享清理函数使用严格对象解析并保留 JSON 数字精度。修改前回归实际出现新元数据丢失、9007199254740993 变为 9007199254740992、小数截短及接受 null；修改后四项回归通过，也覆盖无效对象不更新行、缺失目标和取消请求。

新增三个职责测试文件及一个基准文件。目录与快照七项回归、元数据四项回归均已通过。测试仅使用临时数据库，不访问生产服务、密码箱或上游。

## 隔离基准

同一 Go 1.26.6 工具链和临时 fixture，每项 20 次迭代、三次采样；修改前后顺序执行，未与竞态回归重叠。下表展示采样区间，KB/MB 为累计 Go 分配量的近似值。

| 场景                |     修改前耗时 |     修改后耗时 |     修改前分配 |     修改后分配 |
| ------------------- | -------------: | -------------: | -------------: | -------------: |
| 模型预览，1 账号    |   1.11–1.51 ms |   0.63–0.76 ms |     254–255 KB |     130–131 KB |
| 模型预览，100 账号  |   5.06–5.32 ms |   1.67–1.75 ms |     614–621 KB |     408–410 KB |
| 模型预览，1000 账号 | 38.87–43.64 ms | 10.69–12.59 ms | 3.994–4.006 MB | 3.071–3.078 MB |
| 检测范围，1 分组    | 0.205–0.241 ms | 0.345–0.378 ms |         3.3 KB |   4.35–4.60 KB |
| 检测范围，100 分组  |   2.71–4.53 ms |   0.98–1.08 ms |       106.5 KB |          40 KB |
| 检测范围，500 分组  | 43.07–48.99 ms |   3.96–6.21 ms |     557–570 KB |         232 KB |

千账号预览中位耗时约下降 73%，累计分配约下降 23%。单分组查询增加约 0.14 ms，批量处理的固定开销不可忽略；不宣称所有规模都更快。这些结果不是 SQLite 分配器、常驻内存或生产接口延迟测量。

复现入口：`go test ./internal/business/__tests__ -run '^$' -bench 'Benchmark(ModelCatalogBatch|DetectionScopeBatch)' -benchmem -benchtime=20x -count=3`。准确基准名称见 `catalog_read_benchmark_test.go`；原始前后结果保存于 `/tmp/sub2api-business-reads-20261011/{before-bench,after-bench,detection-final-bench}.log`。

## 验证范围

最终源码对应的受影响命令在 `backend/` 执行，显式包含外部测试包及直接调用方：

```sh
bash scripts/check-go.sh test ./internal/business ./internal/business/__tests__ ./internal/accountops ./internal/accountops/__tests__ ./internal/modelcheck ./internal/modelcheck/__tests__ ./internal/onboarding ./internal/onboarding/__tests__ ./internal/api ./internal/api/__tests__ ./cmd/server ./internal/routingwrite ./internal/routingwrite/__tests__
bash scripts/check-go.sh vet ./internal/business ./internal/business/__tests__ ./internal/accountops ./internal/accountops/__tests__ ./internal/modelcheck ./internal/modelcheck/__tests__ ./internal/onboarding ./internal/onboarding/__tests__ ./internal/api ./internal/api/__tests__ ./cmd/server ./internal/routingwrite ./internal/routingwrite/__tests__
```

最终十三包竞态回归与 vet 全部通过，退出码均为 0；竞态回归九包实跑、四包复用 Go 有效缓存（modelcheck、onboarding、onboarding/**tests**、api）。业务外部测试包实跑 639.526 秒，modelcheck 外部包实跑 521.147 秒；这也是等待较长的主要原因，不是重复启动检查。旧十一包通过结果早于最后的元数据修复，不能替代当前最终结果。最终三个源码文件与新增测试的 gofmt、差异空白检查通过。未修改前端，无需重复 TypeScript 检查。

原始输出目录为 `/tmp/sub2api-business-reads-20261011/`：`runtime-before.log` 和 `runtime-after.log` 记录失败转通过；`final-test.log` 记录目录和检测范围回归；`final-race.log` 与 `final-vet.log` 为最终十三包检查。没有并行启动另一轮后端竞态套件。

## 逐文件覆盖与保留项

本轮完整阅读原有 95 项缺口（26 个运行源码及 69 个测试）和四个新增文件，99 项记录见 [业务存储台账](full-file-audit/business-storage-2026-10-11.tsv)。此前 156 项记录逐一按当前内容哈希复用，共覆盖 255 个业务文件；已核对本轮 99 项与复用 156 项的当前哈希。审查记录表示文件已阅读，不表示每个潜在优化均已测量或修复。

以下保留为未测量或未复现的候选，不据此持续追加本模块的零散修改：

- 手动优先级读取先扫描目录再筛选；空范围表示全部，不能直接套用所选账号语义。
- 价格分组同步写路径可能按账号重复读取分组锁策略；此前价格目录读侧优化保持收口。
- 请求轨迹可能先读全部账号用量再选取少量错误；日志冻结，本轮未修改。
- 告警评估在写事务前读取策略与发现项，存在需进一步复现的陈旧策略风险。
- 一个动画证据迭代路径缺少显式 `rows.Err` 检查；历史读取范围及部分 readmodel 策略解析仍可测量。组合读取的快照一致性仍取决于调用方。
- 旧测试存在合并断言、真实时钟 fixture 及租约短超时等维护限制，本轮未整体重写。

本模块验证已收口，按顺序进入 API 适配。全局六项目标继续进行，本模块结果不代替全仓审查或最终发布门禁。
