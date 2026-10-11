# 调度写回准备并发与模块收口（2026-10-11）

本次完成 routing / routingwrite 约定范围：补完剩余逐文件审查，限制账号准备、写入和结果确认的工作协程数量。此前批量确认与评分配置复用的改动保留。日志模块保持冻结，下一模块为 modelcheck 后端；全局六项目标仍未完成。

## 原因与改动

原实现为每个账号启动一个 goroutine，准备完成后仍阻塞在批量协调器中，直到所有账号到齐。批量请求的取消监听又按账号启动 goroutine；HTTP 并发虽有限制，等待中的协程数量仍随账号数增长。

现在分两阶段执行：有界工作线程准备账号并提交写入意图，全部准备完成后再由有界线程处理结果。期间保存结果处理闭包，保留原有整批汇合、相同字段合并写入、授权复核、容量预留、基线捕获和读回确认。没有写入的账号同样通知协调器；整批完成后才释放租约。批量分组、旧接口回退和不明确响应的读回也使用有界工作线程；空闲线程动态领取下一项，避免排在慢请求后的任务闲置。

取消监听按不同的 Done 信号去重，使用 context.AfterFunc；共有父上下文不再重复注册。组内外工作线程可能合计达到配置并发数的平方，但不随账号数量无限增长，HTTP 请求仍共享同一并发闸门（配置上限 16）。保存的结果和闭包仍是 O(账号数)，这不是将整个操作的空间复杂度降为常数。

## 回归与发现

新增行为测试覆盖 17 个账号超过工作线程上限时：混合无效目标、无需变更和正常目标仍完整汇合成一个批次；交还控制权恢复全部基线；准备期间取消后所有账号均返回结果且不写入。基准复用隔离 SQLite 和 HTTP fixture，未访问真实管理端。

首次竞态回归发现固定步长分派会让空闲工作线程提前退出，而第三项仍等在慢的第一项后面。先补充不同写入字段、首项阻塞、第三项释放首项的事件控制用例，确认失败，再改为原子索引动态领取，确认通过。超时仅作为死锁失败边界，不以 sleep 制造通过条件。旧的并发上限回归也恢复通过。

失败记录在 `/tmp/sub2api-routing-preparation-20261011/queue-before.log`，修复后的定向竞态记录在 `queue-after.log`。最初整包验证的停滞诊断保留在 `test.log`，不把该次中断执行算作通过。

## 对照基准

工具链、fixture 和并发数 2 一致，使用修改前 production 文件的 Go overlay 与最终实现依次运行；基准期间没有同时运行本任务的 vet 或竞态回归。每种规模三组，每组 3 次：

```bash
go test -overlay /tmp/sub2api-routing-preparation-20261011/baseline-overlay.json ./internal/routingwrite/__tests__ -run '^$' -bench '^BenchmarkRoutingPreparation$' -benchmem -benchtime=3x -count=3
go test ./internal/routingwrite/__tests__ -run '^$' -bench '^BenchmarkRoutingPreparation$' -benchmem -benchtime=3x -count=3
```

| 账号数 | 写入时新增 goroutine（前 → 后） | 耗时中位数（前 → 后） | 累计分配字节中位数（前 → 后） |
| ------ | ------------------------------- | --------------------- | ----------------------------- |
| 32     | 70.7 → 7.7                      | 182.9 → 152.6 ms/op   | 6,722,546 → 6,626,808 B/op    |
| 256    | 519 → 9                         | 1176.9 → 1122.0 ms/op | 46,500,693 → 47,565,912 B/op  |

这确认了写入时协程数量增长被消除，指标是相对调用前的瞬时差值，不是整个流程的峰值。耗时区间仍重叠，样本量小，不宣称稳定生产提速。256 账号累计分配字节增加约 2.3%，32 账号减少约 1.4%；不能宣称总体内存下降，更不能外推 SQLite 内存或 RSS。初次并行编译期间的小规模耗时曾变差（约 183–217 ms/op），对应 `before.log` / `after.log` 保留；最终上述控制对照分别在 `before-controlled.log` / `after-controlled.log`。

## 最终验证

从 backend 目录执行：

```bash
bash scripts/check-go.sh test ./internal/routingwrite ./internal/routingwrite/__tests__ ./internal/inspection ./internal/inspection/__tests__ ./internal/api ./internal/api/__tests__ ./cmd/server
bash scripts/check-go.sh vet ./internal/routingwrite ./internal/routingwrite/__tests__ ./internal/inspection ./internal/inspection/__tests__ ./internal/api ./internal/api/__tests__ ./cmd/server
```

两项均退出 0。最终竞态命令 7 包通过：routingwrite、其 **tests**、cmd/server 共 3 包实跑，inspection 和 api 的普通/外部测试共 4 包由 Go 判定缓存有效并复用。检查日志为同一临时目录的 `test-final.log`、`vet-final.log`。其余 routing、probe、onboarding、upstreamsync 的依赖未受本次改动影响，沿用上一轮记录，没有重新执行全后端或前端门禁。涉及文件 gofmt 和差异空白检查通过。

## 审查结论与边界

本次补完原有 18 个审查缺口，并全文复审因内部辅助调用变化而失效的 routingwrite/service_test.go，覆盖稳定账号范围、跨分组保底、容量预算、人工保护、写入授权、失败读回和基线恢复。生产改动在此前全文审查基础上核对完整差异及相关函数体；新增准备与测试文件全文核对。具体哈希与结论见 [本次台账](full-file-audit/routing-preparation-2026-10-11.tsv)。

既有大测试文件仍包含较宽的职责和少数依赖短等待窗口的否定断言；本次未重写这些历史测试，不把哈希覆盖等同于全部测试风格符合规范或所有潜在风险消失。新增回归使用明确事件同步。当前模块的集中性能改动、生产边界审查和受影响验证完成，后续仅因明确新缺陷或依赖回归重新打开。

未提交、推送、部署、启动或重启服务，未操作生产数据。
