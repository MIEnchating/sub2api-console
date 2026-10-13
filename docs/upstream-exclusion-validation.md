# 上游排除修复验证记录

验证日期：2026-10-12。范围为当前未提交工作区的上游排除修复，不代表完整 CI、生产发布或其他并行修改已验收。

## 行为验证

- 范围过滤：模型自动化、账号模型发现/应用、分组检测、上游同步、鉴权恢复、维护任务、分组账号数量。
- 执行边界：排队后排除、混合账号、全部排除不发请求、不使任务失败。
- 预览安全：排除导致范围缩小可继续；模型目录真实变化仍拒绝旧预览。
- 恢复：排除账号不重新生成，已完成历史结果保留，进度不超过 100%。
- 其他入口：工作台维护、开户来源账号、流量与实时补采、通知发送前复核。
- 页面：维护结果显示“已跳过”及排除原因。

## 已通过检查

Go 均使用 `backend/scripts/check-go.sh test`（race、包并行 4、用例并行 4），显式包含模块 `__tests__` 包。

| 范围 | 结果与证据 |
| --- | --- |
| 业务存储、管理维护、上游同步、巡检、routing、routingwrite、probe、accountdelete、upstreamdelete、upstreamconfig、upstreamexclusion | 模块回归通过，日志 `/tmp/console-exclusion-callers.log` |
| accountautomation、accountops、authrecovery、accountworkbench、evidence、notification、onboarding 测试包 | 首轮模块结果见 `/tmp/console-exclusion-task-tests.log`；后续修改按下列范围补验 |
| accountops、modelcheck、evidence、onboarding、management、upstreamsync 最新受影响场景 | 定向回归通过，日志 `/tmp/console-exclusion-final-targeted.log` |
| accountautomation 数量、accountworkbench 排除、modelcheck 排除与恢复 | 最后补验通过，日志 `/tmp/console-exclusion-last.log` |
| API 排除、目录范围与相关请求，server 编译 | 补验通过，日志 `/tmp/console-exclusion-api.log` |
| Go vet | 涉及模块及调用方通过，日志 `/tmp/console-exclusion-final-vet.log`、`/tmp/console-exclusion-last-vet.log` |
| 前端 | `bun run typecheck`、维护 UI 6 个测试、constants 与 maintenance-ui 测试文件 ESLint 均通过；oxfmt 已执行 |
| 差异格式 | `git diff --check` 通过 |

首轮 modelcheck 存在空范围旧断言失败，现已更新为正常结束并通过定向回归。并行修改期间 onboarding、accountworkbench 曾出现缺失定义，文件齐备后相关测试、API/server 编译和 vet 已补验通过。未将首轮失败命令描述为整体成功；未重复执行无变化且已有通过记录的模块。

未访问生产数据库、上游或 QQ；未部署、未提交或推送。日志位于本地临时目录，不作为永久 CI 凭据。
