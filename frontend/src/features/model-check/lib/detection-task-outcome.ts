import type { Task } from "@/api";
import { detectionMissingResultStates, type DetectionResultTone } from "../constants";

export type DetectionMissingResult = { label: string; reason?: string; tone: DetectionResultTone };

function taskTimedOut(task: Task | undefined): boolean {
  if (task?.status !== "failed") return false;
  return (
    task.result.error === "context deadline exceeded" ||
    /^(?:任务执行失败[:：]\s*)?context deadline exceeded$/.test(task.message.trim())
  );
}

export function detectionTaskMessage(task: Task): string {
  if (taskTimedOut(task))
    return "检测任务已超时结束，已保留完成的结果；其余阶段未取得结果，可重新检测。";
  return task.message;
}

export function detectionMissingResult(
  task: Task | undefined,
  active: boolean,
): DetectionMissingResult {
  if (active) return detectionMissingResultStates.waiting;
  if (taskTimedOut(task)) return detectionMissingResultStates.timeout;
  if (task?.status === "cancelled") return detectionMissingResultStates.cancelled;
  if (task?.status === "failed") {
    const reason =
      typeof task.result.error === "string" && task.result.error.trim()
        ? task.result.error.trim()
        : task.message.trim();
    if (reason)
      return {
        ...detectionMissingResultStates.failed,
        reason: `任务失败：${reason.replace(/^任务执行失败[:：]\s*/, "")}。没有结果的阶段无法确认是否执行，请重新检测。`,
      };
    return detectionMissingResultStates.failed;
  }
  return detectionMissingResultStates.missing;
}
