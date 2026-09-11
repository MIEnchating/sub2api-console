import { toast } from "sonner";
import type { Task } from "@/api";

/** 仅发送终态摘要，进度与逐项结果仍由任务详情展示。 */
export function notifyTaskResult(
  task: Pick<Task, "id" | "status" | "message">,
  action: string,
  options?: { successMessage: string },
): void {
  const message = task.message.trim();
  const toastOptions = { id: `task-result:${task.id}` };
  if (task.status === "succeeded") {
    toast.success(options?.successMessage || message || `${action}完成`, toastOptions);
  } else if (task.status === "partial") {
    toast.warning(message || `${action}部分完成，请查看任务结果`, toastOptions);
  } else if (task.status === "cancelled") {
    toast.info(message || `${action}已取消`, toastOptions);
  } else if (task.status === "failed") {
    toast.error(message || `${action}失败`, toastOptions);
  }
}
