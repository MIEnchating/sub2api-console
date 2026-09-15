# 前端补审修复记录

范围为 pricing、policy、traffic-ranking 的原 38 个文件及本轮新增的 4 个文件；另接续审查账号维护表单与新增回归文件。逐文件内容哈希分别记录在 `frontend-final.tsv`（42 项）和 `frontend-maintenance.tsv`（2 项）。

| 问题                         | 触发和影响                                                                                             | 修复                                                                                             | 回归证据                                                                                                          |
| ---------------------------- | ------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------- |
| 价格草稿被后台响应覆盖       | 修改盈利目标后后台返回其他配置，或保存期间继续输入，草稿被旧响应覆盖                                   | 未编辑表单直接读取最新缓存；只有提交内容仍等于当前草稿时才清除草稿                               | `draft-retention.test.tsx` 两个失败用例转为通过，并覆盖未编辑表单继续同步                                         |
| 调价确认预览丢失十进制精度   | 成本 `0.1`、售价 `0.11`、盈利目标 10% 时前端错误跳过最低售价；高精度成本和售价也可能被视为相等         | 复用原有有界十进制解析，使用 BigInt 定点乘法和比较；盈利判断、售价排序和计算依据一致             | `selection.test.tsx` 三种边界均先失败后通过，原最低迁入倍率与保本回退测试保持通过                                 |
| 成本档位误合并且重复扫描账号 | `0.1` 与 `0.10000000000000001` 被合并为同档，明细顺序错误；每个可见分组在渲染时独立扫描所有账号        | 按精确十进制比较、合并等值表示；按账号数据一次建立分组成本索引并缓存，筛选和翻页不再重复全量扫描 | `cost-tiers.test.tsx` 两个失败用例转为通过；性能改进是移除重复扫描，未宣称未测量的耗时倍数                        |
| 维护配置版本更新重建交互状态 | 后台轮询返回新 revision 后 React key 变化，草稿、进行中的任务和保存 pending 状态一并丢失；可能再次提交 | 表单实例保持稳定，脏草稿与确认操作保留原 revision，干净表单同步新版本，显式重置可采用最新配置    | `maintenance-refresh.test.tsx` 先复现三个失败行为；覆盖原版本提交、干净表单更新、任务保留、pending 禁用及键盘重置 |

## 修复位置

- 价格页面：[pricing-page.tsx](../../../frontend/src/features/pricing/components/pricing-page.tsx)
- 精确数值比较：[pricing-decimal.ts](../../../frontend/src/features/pricing/lib/pricing-decimal.ts)
- 分组成本索引：[account-costs.ts](../../../frontend/src/features/pricing/lib/account-costs.ts)
- 维护表单：[workbench-maintenance.tsx](../../../frontend/src/features/account-workbench/components/workbench-maintenance.tsx)

## 已完成验证

- `bun run test src/features/pricing src/features/policy src/features/traffic-ranking --maxWorkers=2 --minWorkers=1`：25 个测试文件、140 条测试通过。
- 价格模块修改后的 `bun run typecheck`、7 个修改文件的 ESLint、oxfmt 和 `git diff --check` 通过。
- `bun run test src/features/account-workbench/__tests__/maintenance-refresh.test.tsx src/features/account-workbench/__tests__/maintenance-uploads.test.tsx src/features/account-workbench/__tests__/maintenance-authorization.test.tsx --maxWorkers=2 --minWorkers=1`：3 个测试文件、17 条测试通过。
- 维护修改后的最新 `bun run typecheck`、2 个修改文件的 ESLint、oxfmt 和 `git diff --check` 通过。

本文件仅汇报本轮补审的修复。此前完成的后端、Uptime Kuma 与工作台队列问题见 `backend-domain-findings.md`，全项目检查由主审查统一汇总。
