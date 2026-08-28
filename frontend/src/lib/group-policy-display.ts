export function strategySourceLabel(value: string): string {
  if (value === "group_override") return "分组覆盖";
  if (value === "global_default") return "全局默认";
  return "配置错误";
}

export function participationStatusLabel(value: string): string {
  if (value === "participating") return "已参与";
  if (value === "out_of_scope") return "未参与";
  return "配置错误";
}

export function participationReasonLabel(value: string | null): string {
  return value ?? "—";
}

export function groupStatusMeta(value: string): {
  label: string;
  tone: "success" | "warning" | "danger" | "info" | "neutral";
} {
  if (value === "healthy") return { label: "健康", tone: "success" };
  if (value === "rate_limited") return { label: "限流中", tone: "warning" };
  if (value === "partial_degraded") return { label: "部分异常", tone: "warning" };
  if (value === "survivor_only") return { label: "仅剩保底", tone: "danger" };
  if (value === "all_fused") return { label: "全部熔断", tone: "danger" };
  if (value === "all_unavailable") return { label: "全部不可调度", tone: "danger" };
  if (value === "excluded") return { label: "已排除", tone: "neutral" };
  if (value === "skipped") return { label: "未参与", tone: "neutral" };
  if (value === "empty") return { label: "无账号", tone: "neutral" };
  return { label: "配置异常", tone: "danger" };
}

export function overviewStrategyLabel(strategyLabel: string, participationStatus: string): string {
  return participationStatus === "out_of_scope" ? "未参与" : strategyLabel;
}
