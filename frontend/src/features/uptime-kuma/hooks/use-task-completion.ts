import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, ApiError, type Task } from "@/api";
import { operationErrorMessage } from "@/lib/operation-feedback";
import { isSessionExpiredError } from "@/lib/session-auth";
import { kumaTaskCancellationError } from "../lib/task-cancellation";

const terminalStatuses = new Set(["succeeded", "failed", "cancelled", "partial"]);
const pageClosedMessage = "页面已关闭，后台任务仍可在日志中心查看";

async function readTaskProgress(taskID: string, signal: AbortSignal): Promise<Task> {
  try {
    return await api.task(taskID, signal);
  } catch (error) {
    if (isSessionExpiredError(error)) throw error;
    throw new Error(
      `任务 ${taskID} 的进度读取失败，请在日志中心查看结果后再决定是否重试：${operationErrorMessage(error, "请求未完成")}`,
      { cause: error },
    );
  }
}

// The mutation remains pending while React Query follows the persisted backend
// task. Neither a failed poll nor a disconnected page resubmits the operation.
export function useTaskCompletion() {
  const [result, setResult] = useState<Task | null>(null);
  const [task, setTask] = useState<Task | null>(null);
  const [starting, setStarting] = useState(false);
  const mounted = useRef(true);
  const completion = useRef<{
    resolve: (task: Task) => void;
    reject: (error: Error) => void;
  } | null>(null);
  const query = useQuery({
    queryKey: ["tasks", task?.id],
    queryFn: ({ signal }) => readTaskProgress(task!.id, signal),
    enabled: !!task,
    retry: false,
    refetchInterval: (query) =>
      terminalStatuses.has(query.state.data?.status ?? "") ? false : 1000,
  });
  useEffect(() => {
    if (!task || !completion.current) return;
    if (query.error) {
      completion.current.reject(query.error);
    } else if (query.data && terminalStatuses.has(query.data.status)) {
      setResult(query.data);
      if (query.data.status === "succeeded") completion.current.resolve(query.data);
      else if (query.data.status === "cancelled")
        completion.current.reject(kumaTaskCancellationError(query.data));
      else
        completion.current.reject(
          new ApiError(
            query.data.message,
            String(query.data.result.error_code ?? "kuma_task_failed"),
            422,
          ),
        );
    } else return;
    completion.current = null;
    setTask(null);
  }, [task, query.data, query.error]);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      completion.current?.reject(new Error(pageClosedMessage));
      completion.current = null;
    };
  }, []);
  const wait = (task: Task): Promise<Task> =>
    new Promise((resolve, reject) => {
      if (!mounted.current) {
        reject(new Error(pageClosedMessage));
        return;
      }
      completion.current = { resolve, reject };
      setStarting(false);
      setTask(task);
    });
  const prepare = (): void => {
    setResult(null);
    setStarting(true);
  };
  return { wait, prepare, starting, result, task: query.data ?? task };
}
