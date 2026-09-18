export const configTabs = [
  { value: "connection", label: "连接设置" },
  { value: "newapi", label: "New API 平台" },
  { value: "monitoring", label: "监控平台" },
  { value: "tasks", label: "任务并发" },
  { value: "accounts", label: "账号设置" },
  { value: "notifications", label: "通知设置" },
  { value: "interface", label: "界面与日志" },
  { value: "dictionaries", label: "字典管理" },
] as const;

export type ConfigTab = (typeof configTabs)[number]["value"];

export function normalizeConfigTab(value: unknown): ConfigTab {
  if (typeof value !== "string") return "connection";
  const matchedTab = configTabs.find((tab) => tab.value === value);
  return matchedTab?.value ?? "connection";
}

export const taskPoolLabels: Record<string, string> = {
  account: "账号操作",
  probe: "账号探活",
  model_check: "模型检测",
  model_animation_scheduler: "模型自动检测调度",
  upstream_sync: "上游同步",
  upstream_delete: "上游删除",
  account_delete: "账号删除",
  onboarding: "添加账号",
  auth_recovery: "鉴权恢复",
  inspection: "巡检",
  management: "管理数据同步",
  pricing: "计费与价格",
  alert: "告警任务",
  notification_target: "通知目标获取",
  logs: "日志任务",
  uptime_kuma: "Uptime Kuma",
  newapi_channel: "New API 渠道",
  live: "实时采集",
  workbench: "账号工作台",
  housekeeping: "后台维护",
};
