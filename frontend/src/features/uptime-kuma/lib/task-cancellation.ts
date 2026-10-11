import { ApiError, type Task } from "@/api";

export function kumaTaskCancellationError(task: Task): ApiError {
  const message =
    task.result.remote_write === false
      ? "Uptime Kuma 任务已取消，未执行远端操作"
      : "Uptime Kuma 任务已取消；已发送的操作不会自动撤销，请刷新核对远端状态后再操作";
  return new ApiError(message, "kuma_task_cancelled", 422);
}

export function isKumaTaskCancellation(error: unknown): error is ApiError {
  return error instanceof ApiError && error.code === "kuma_task_cancelled";
}
