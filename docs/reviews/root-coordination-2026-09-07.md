# 根审查协调备注

> 历史过程记录，分配与等待状态已经失效。全部分配范围完成情况和最新验证请看 `global-review-2026-09-07.md`。

给账号与上游审查：已读取你的中间报告。根侧已完成 configstore/sqliteutil/taskstore/redact/naming/runtimepolicy/taskrunner/taskcontext/mutationguard/targetguard 的生产文件人工审查，以及 business/mutation_lease.go 和 mutation_protection.go；正在审查全部 API。新修复共享数据库路径转义、完整凭据值脱敏、任务时间排序/清理、CI/E2E 门禁。

上游自定义 Header 返回浏览器候选需要处理敏感 Header 值保密与编辑保留契约。请先复现后告知根侧拟议响应契约，避免直接更改前端契约；已有前端 Header 表单需要支持未改值保留。根侧尚未修改该路径。

其余两个委派范围已写在 /tmp/review-ops-scope.txt 和 /tmp/review-frontend-scope.txt；当前仅账号与上游审查被确认启动。若可以调用协作工具，请启动两个独立代理并让根侧知道，避免范围闲置。所有代理共享工作区，勿覆盖现有改动。

更新：根侧本轮已读完 management/service.go、newapimanagement 全生产文件、pricing/service.go/revenue.go，以及 business/store.go、schema.go、catalog.go、readmodels.go、management_sync.go、pricing_catalog.go、revenue_catalog.go、pricing_backups.go、newapi_management.go、traffic_ranking.go、cleanup.go、billing_quota_unit.go。新增修复：New API 输入/精度/分组回滚、账号维护与价格分组执行时模式、Base URL 同步人工保护、外部十进制有界解析。未完成候选仍见主清单；跨前端审查未启动（第四槽被验证码子审查占用），请验证码子审查完成后立即接手 /tmp/review-frontend-scope.txt。

operations 请读取 /tmp/root-operations-coordination.txt；根侧需要与你统一 usage_records 时间窗口与证据时间规范。已新加 decimalutil.Parse 可供外部十进制输入校验使用（先写复现测试）；根侧拥有管理/定价和上述公共 business 文件。
