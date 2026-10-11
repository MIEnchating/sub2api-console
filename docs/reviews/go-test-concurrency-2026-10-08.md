# Go 竞态测试并行限制验证（2026-10-08）

## 范围与设置

工作区基于 `12a3d800e6d231a9db6fd6437e51c33074c2d431`，含既有未提交修改。本轮只调整 `backend/scripts/check-go.sh` 的测试调度，并将规则同步到 `AGENTS.md` 和 `README.md`；没有改动业务逻辑、测试断言或超时。

原脚本未指定并行上限，本机可用 CPU 数为 16。最终候选脚本使用 `go test -p=4 -parallel=4 -race -timeout=20m`：最多同时运行 4 个包，每个包内最多并行执行 4 个声明 `t.Parallel()` 的用例。普通包和自动发现的所有 `__tests__` 包均保留。CI 和发布共用该脚本，直接继承限制。

两轮均在 `backend/` 执行 `GOFLAGS='-count=1' /usr/bin/time -p bash scripts/check-go.sh test`，禁用测试结果缓存，保留编译缓存。各轮顺序执行，没有同时启动另一轮后端全量测试。已有开发及在线服务继续运行；不是空闲机器基准。

## 第一轮：仅限制包级并行

- 配置：`-p=4`，包内仍使用原默认值。
- 耗时：838.12 秒。
- `internal/routingwrite` 通过，39.682 秒；`internal/routingwrite/__tests__` 通过，170.570 秒。此前的重叠批次超时和写入授权复核失败均未复现。
- 全量检查未通过：`internal/modelcheck/__tests__/TestAnimationAllSelectedAccountsExceedTwenty` 等待任务完成超时，随后清理任务超时并关闭数据库。其余包通过。
- 模型检测包没有 `t.Parallel()`，但同时运行的业务库测试有大量并行用例，因此第二轮进一步限制包内并行。
- 日志：`/tmp/sub2api-go-p4-20261008.log`。

## 第二轮：同时限制包级与包内并行

- 配置：`-p=4 -parallel=4`，测试进程参数已核对包含 `-test.parallel=4 -test.count=1`。
- 完整回归通过：75 个有测试的包全部通过，命令退出码 0，没有测试结果缓存命中，耗时 852.08 秒（约 14 分 12 秒）。
- `internal/routingwrite` 通过，40.885 秒；`internal/routingwrite/__tests__` 通过，225.942 秒；`internal/modelcheck/__tests__` 通过，471.391 秒。前述三处失败本轮均未复现。
- 采用此配置。相较第一轮总耗时基本持平；本次结果支持降低并行争用的方向，不能据单轮通过认定所有时序波动已消除，也不能声称整体提速。
- 日志：`/tmp/sub2api-go-p4-parallel4-20261008.log`。

## 静态检查与验证边界

`bash -n backend/scripts/check-go.sh` 与涉及文件的 `git diff --check` 通过。使用临时 Go 命令替身校验了脚本的并行参数、竞态参数、原有超时、普通包与模块回归发现；`vet` 调用保持原有参数。临时替身目录已清理。

运行前保存 Go 源码、模块及脚本的 SHA256 清单，分别为 `/tmp/sub2api-go-p4-20261008.sha256` 与 `/tmp/sub2api-go-p4-parallel4-20261008.sha256`。校验发现其他工作在测试期间修改了 `backend/internal/onboarding/probe.go`，第二轮另有 `backend/internal/accountautomation/service.go` 变化；这些后续变更仍需各自验证。因此记录用于评估测试调度配置，不能冒充最终工作区或提交的完整 CI 成功。失败涉及的模型检测和调度写回源码在已核对清单中没有变化。

## 日常反馈时间调整

完整实跑的主要慢包为 `modelcheck/__tests__` 471.391 秒、`business/__tests__` 329.655 秒、`probe/__tests__` 254.917 秒、`routing/__tests__` 231.870 秒和 `routingwrite/__tests__` 225.942 秒。包间并行执行，不能直接相加当作总耗时；这些数字也不能证明具体函数或夹具的性能根因。

为避免每次开发迭代都等待完整实跑，统一检查脚本增加可选包参数，原无参数完整入口不变。例如 `bash scripts/check-go.sh test ./internal/accountautomation ./internal/accountautomation/__tests__`，仅验证显式列出的范围；调用者负责补充对应模块回归和受影响调用方。`vet` 支持同样的范围。带包参数时脚本输出范围提示，防止误记为完整检查。

按用户确认的规则，普通验证保留并复用已通过模块的有效结果，最终候选也不强制重跑全量：只核对所需范围是否齐全，补充新增、失效、失败或缺少可靠记录的范围。复用判断包括业务代码、测试及传递依赖、配置、fixture、相关环境和工具链；优先交给 Go 自带缓存维护，不以测试文件未变化作为唯一条件，不额外建立永久通过标记。完整脚本已覆盖普通包，不再重复执行 `go test -race ./...`。CI 和发布仍核验所需范围的结果，汇总时区分本次执行与复用。此次完整实跑禁用缓存是为了测量并行配置，不作为日常默认行为。

脚本语法与空白检查通过。临时 Go 命令替身验证了默认完整范围、显式包的 test/vet 范围、嵌套回归发现、无效命令及下游失败退出码传递；临时文件已清理。无参数 Go 调用与第二轮一致，未重复运行全量测试。真正缩短全量仍需对上述慢包采样定位，不能直接提高并行数或删除覆盖。
