export const policyCleanupActionOptions = [
  { value: "none", label: "仅停止调度，不额外处置" },
  { value: "pause", label: "暂停调度" },
  { value: "disable", label: "停用账号" },
  { value: "delete", label: "删除账号" },
] as const;

export const costWallPolicyLabels = {
  title: "成本墙",
  toggle: "启用成本墙拦截",
  description:
    "倍率等于或超过所有受管分组的有效成本墙时停止调度。自动写入遵守运行模式与调度状态自动执行开关。",
  fallback: "无可用账号时允许保底",
  fallbackDescription:
    "分组没有可用账号时，临时启用一个符合安全条件的成本墙账号；正常账号恢复后关闭。",
  stopAutoProbe: "拦截期间停止自动探活",
  stopAutoProbeDescription: "成本墙拦截的账号停止自动探活，保底启用确认后恢复；手动探活始终保留。",
} as const;

export function policyCleanupActionLabel(value: string): string {
  return policyCleanupActionOptions.find((option) => option.value === value)?.label ?? value;
}

export const policyScalingDescription =
  "已知 Sub2API 用户并发上限时，同一上游的账号按调度策略权重共享可用并发；不足时低优先级账号等待并发额度，额度恢复后自动评估。分配仍受全局上限、单账号上下限、步长和冷却约束。其余账号在已配置并发占全局并发上限的比例达到阈值时小步扩容，健康状态变差时按步长缩容。该比例表示配置容量，不代表实时请求利用率。完全模式下同时开启「并发上限自动执行」与「调度状态自动执行」后才会自动调整并发、暂停或恢复账号。";

export const upstreamConcurrencyPolicyLabels = {
  title: "上游超额自动下调",
  toggle: "启用上游超额自动下调",
  description:
    "完全模式下，关联账号总并发超过已确认的 Sub2API 用户额度时，按调度策略下调并发，必要时暂停低优先级账号。监控模式仅预览。",
  scope:
    "独立控制超额下调，不需要开启智能扩容或通用自动执行开关，不受全局并发预算、单账号扩容上下限、步长和冷却限制。手动及守护范围外的账号保留现有容量。",
  recovery:
    "此开关不自动扩容或恢复账号。恢复「等待并发额度」账号仍需开启智能扩容、并发上限自动执行与调度状态自动执行，并通过额度及健康核对。",
} as const;
