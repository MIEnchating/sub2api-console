import { groupStatusDictionary } from "./domain-dictionaries";

export function strategySourceLabel(value: string): string {
  return { group_override: "分组覆盖", global_default: "全局默认" }[value] ?? "配置错误";
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
  return groupStatusDictionary[value] ?? { label: "配置异常", tone: "danger" };
}

export function overviewStrategyLabel(strategyLabel: string, participationStatus: string): string {
  return participationStatus === "out_of_scope" ? "未参与" : strategyLabel;
}
