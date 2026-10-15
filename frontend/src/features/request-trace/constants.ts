import type { SystemLogSearchQuery } from "@/api";
import type { StatusVariant } from "@/components/status-badge";

export const timeRangeOptions = [
  { value: "5m", label: "最近 5 分钟" },
  { value: "30m", label: "最近 30 分钟" },
  { value: "1h", label: "最近 1 小时" },
  { value: "6h", label: "最近 6 小时" },
  { value: "24h", label: "最近 24 小时" },
  { value: "7d", label: "最近 7 天" },
  { value: "30d", label: "最近 30 天" },
] satisfies { value: SystemLogSearchQuery["timeRange"]; label: string }[];

export const logLevelOptions = [
  { value: "", label: "全部级别", variant: "neutral" },
  { value: "debug", label: "调试（debug）", variant: "neutral" },
  { value: "info", label: "信息（info）", variant: "info" },
  { value: "warn", label: "警告（warn）", variant: "warning" },
  { value: "error", label: "错误（error）", variant: "danger" },
] satisfies { value: SystemLogSearchQuery["level"]; label: string; variant: StatusVariant }[];

export function logLevelDisplay(level: string): { label: string; variant: StatusVariant } {
  const option = logLevelOptions.find((item) => item.value !== "" && item.value === level);
  return option ?? { label: level || "级别未记录", variant: "neutral" };
}
