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

export const policyScalingSummary =
  "按配置的容量阈值和健康状态调整并发，遵守全局上限、单账号上下限、步长与冷却。";

export const policyScalingDescription =
  "已知 Sub2API 用户并发上限时，同一上游的账号按调度策略权重共享可用并发；不足时低优先级账号等待并发额度，额度恢复后自动评估。分配仍受全局上限、单账号上下限、步长和冷却约束。其余账号在已配置并发占全局并发上限的比例达到阈值时小步扩容，健康状态变差时按步长缩容。该比例表示配置容量，不代表实时请求利用率。完全模式下同时开启「并发上限自动执行」与「调度状态自动执行」后才会自动调整并发、暂停或恢复账号。";

export const upstreamConcurrencyAccountModes = [
  { value: "all", label: "全部符合条件的账号" },
  { value: "selected", label: "指定账号" },
  { value: "upstreams", label: "指定上游" },
] as const;

export const upstreamConcurrencyPolicyLabels = {
  help: "查看共享并发分配说明",
  accountScope: "共享并发分配范围",
  accounts: "参与共享并发分配的账号",
  upstreams: "参与共享并发分配的上游",
  upstreamScope:
    "基础范围包括所选上游下现有及新添加的符合条件的 Sub2API 账号；账号和上游的单独设置优先。智能扩容仍按其设置执行。",
  emptyUpstreamScope: "尚未选择上游，基础范围为空；已有单独设置仍按其配置生效。",
  allScope: "基础范围包括现有及新添加的符合条件的 Sub2API 账号；单独关闭的账号或上游除外。",
  selectedScope:
    "基础范围仅包括所选账号，新添加账号默认不在此列表内；账号和上游的单独设置优先。范围外账号保留容量，智能扩容仍按其设置执行。",
  emptyScope: "尚未选择账号，基础范围为空；已有单独设置仍按其配置生效。",
  title: "上游共享并发分配",
  toggle: "启用上游共享并发分配",
  description:
    "完全模式下，先为每个健康账号分配至少 1 个并发，再按调度策略权重分配剩余额度。可用额度少于健康账号数时才暂停低优先级账号。监控模式仅预览。",
  scope:
    "独立分配 Sub2API 共享额度，不需要开启智能扩容或通用自动执行开关。智能扩容关闭时不应用全局上限，按各上游额度分配；开启时遵守配置的全局上限、单账号上下限、步长、冷却和自动执行开关。手动及守护范围外的账号保留现有容量，New API 不受上游共享额度限制。",
  recovery:
    "先下调占用过多的账号，读回确认容量释放后，自动恢复符合健康条件的等待账号。缓存额度仅允许下调，人工暂停和熔断账号不会因此恢复。",
} as const;

export type UpstreamConcurrencyMode = (typeof upstreamConcurrencyAccountModes)[number]["value"];

export function upstreamConcurrencyMode(value: unknown): UpstreamConcurrencyMode {
  if (value === "selected" || value === "upstreams") return value;
  return "all";
}

export const allocationSettingLabels = {
  title: "上游共享并发分配",
  loading: "正在读取共享并发设置",
  refresh: "请读取共享并发设置后重试",
  masterOff: "请先在调度策略中启用上游共享并发分配。",
  accountHelp: "默认采用上游及调度策略设置，修改后点击保存生效。健康、手动控制和容量保护继续生效。",
  upstreamHelp:
    "修改后点击保存生效，影响此上游现有及新添加的账号；账号单独设置优先。开启后按上游共享额度分配，智能扩容开启时同时遵守其配置。",
} as const;
