import type { KumaMonitor } from "@/api";

export const kumaQueryKey = ["uptime-kuma"] as const;
export const kumaConfigKey = [...kumaQueryKey, "config"] as const;
export const kumaMonitorsKey = [...kumaQueryKey, "monitors"] as const;
export const kumaTemplatesKey = [...kumaQueryKey, "templates"] as const;
export const httpMethods = ["GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"] as const;
export const requestAuthLabels: Record<string, string> = {
  none: "无鉴权",
  basic: "Basic",
  bearer: "Bearer",
};
const statusLabels: Record<number, string> = { 0: "故障", 1: "正常", 2: "待确认", 3: "维护中" };
export function monitorStatus(monitor: KumaMonitor, management: boolean): string {
  if (management && !monitor.active) return "已暂停";
  return monitor.status === null ? "暂无状态" : (statusLabels[monitor.status] ?? "暂无状态");
}
export const actionLabels = { pause: "暂停", resume: "恢复", delete: "删除" } as const;

export const monitorTypeLabels: Record<string, string> = {
  http: "HTTP(S)",
  keyword: "HTTP(S) 关键字",
  port: "TCP 端口",
  ping: "Ping",
  dns: "DNS",
  push: "Push 推送",
  group: "分组",
};
export const monitorStatusVariants = {
  正常: "success",
  故障: "danger",
  待确认: "warning",
  维护中: "info",
  已暂停: "neutral",
  暂无状态: "neutral",
} as const;
export const monitorStatusOptions = Object.keys(monitorStatusVariants);

export const resourceTitles = {
  notifications: "通知渠道",
  maintenance: "维护计划",
  "status-pages": "状态页管理",
} as const;
export const notificationTypes: Record<string, string> = {
  webhook: "Webhook",
  telegram: "Telegram",
  smtp: "邮件（SMTP）",
  ntfy: "ntfy",
  discord: "Discord",
};
export const maintenanceStrategies: Record<string, string> = {
  manual: "手动维护",
  single: "单次维护",
  cron: "Cron 定时",
  "recurring-interval": "按天间隔",
  "recurring-weekday": "每周重复",
  "recurring-day-of-month": "每月重复",
};
export const resourceActionLabels = {
  create: "新增",
  edit: "编辑",
  delete: "删除",
  test: "发送测试通知",
  pause: "暂停",
  resume: "恢复",
} as const;
export const kumaResourcesKey = [...kumaQueryKey, "resources"] as const;
