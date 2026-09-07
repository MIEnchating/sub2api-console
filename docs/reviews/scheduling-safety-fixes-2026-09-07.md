# 调度安全专项修复

本记录闭环 `scheduling-policy-review-2026-09-07.md` 中第 1、2、3、10 项。根已把原始隔离复现转为正式 `backend/internal/routing/scheduling_safety_test.go`，重新验证四项失败后修改生产实现。

## 修复

- 自动处置复用健康样本分类器，错误码过滤与“仅凭据失效”同时满足；中性客户端错误、限流、成功样本不计入处置。避免将过滤中性样本后的 Events 下标误对应原始 rows。
- 已满足恢复条件并处于健康状态时撤销自动处置观察，保留明确事件，取消旧失败样本产生的删除目标。
- 已熔断账号以新于最后流量与已知熔断起点的有效探测作为恢复证据，真实流量继续独立用于性能评估；新流量失败优先于旧成功探测。
- 绑定重新出现先恢复绑定默认状态，再继续人工暂停、排除、致命错误与健康检查；不再直接提前回池。

## 回归

原四个失败复现已全部通过；另覆盖恢复清理观察记录、更新流量优先、熔断前探测不计连续恢复、配置 500 错误码同时启用/关闭凭据限制，以及绑定恢复后健康/人工暂停边界。

- 最初失败：`/tmp/sub2api-scheduling-safety-before.log`。
- 完整 routing 包通过：`/tmp/sub2api-scheduling-safety-after.log`。
- 随后的 race 期间检测到并行新增的慢探测健康与较新探测选择回归，相关并行生产改动完成后重新执行 routing race 全包通过（2.462s），日志 `/tmp/sub2api-scheduling-safety-race-latest.log`。
- `go vet ./internal/routing` 通过；测试 gofmt 完成；预算/评分/策略版本修复完成后的最终全仓 race 已通过，见全局审查记录。

所有测试使用现有隔离 Repository fixture，未访问真实上游或生产存储。本报告记录其中四项安全修复；全部 10 项的闭环见 `scheduling-policy-fixes-2026-09-07.md`。
