import type { TaskSummary } from "@/api";
import type { StatusVariant } from "@/components/status-badge";

export const activeTaskStatuses = new Set<TaskSummary["status"]>([
  "queued",
  "running",
  "waiting_input",
]);

const taskStatusLabels: Record<TaskSummary["status"], string> = {
  queued: "排队中",
  running: "进行中",
  waiting_input: "等待输入",
  succeeded: "已成功",
  partial: "部分完成",
  failed: "已失败",
  cancelled: "已取消",
};

const taskStatusVariants: Record<TaskSummary["status"], StatusVariant> = {
  queued: "neutral",
  running: "info",
  waiting_input: "warning",
  succeeded: "success",
  partial: "warning",
  failed: "danger",
  cancelled: "neutral",
};

const taskOperationLabels: Record<string, string> = {
  "active-probe": "主动探活",
  "automatic-inspection": "自动巡检",
  "account-model-behavior-check": "模型检测",
  "upstream-sync": "上游同步",
  "alert-evaluation": "告警检测",
  onboarding: "账号开户",
};

export function taskStatusLabel(status: TaskSummary["status"]): string {
  return taskStatusLabels[status];
}

export function taskStatusVariant(status: TaskSummary["status"]): StatusVariant {
  return taskStatusVariants[status];
}

export function taskOperationLabel(operation: string): string {
  return taskOperationLabels[operation] ?? operation;
}
