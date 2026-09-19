export const accountConcurrencyLimitedReason = "上游可用并发不足，账号按调度策略等待额度";
export const accountConcurrencyLimitedHelp =
  "开启上游共享并发分配后，系统先为健康账号各分配至少 1 个并发，再按调度权重分配剩余额度；额度不足的账号等待，释放确认后自动恢复，智能扩容关闭时仅按上游额度独立执行；开启时遵守配置的扩容限制和自动执行开关。";
