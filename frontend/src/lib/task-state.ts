import type { Task } from "../api";

export function taskIsPending(taskId: string | null, task: { data?: Task; isError: boolean }) {
  return Boolean(taskId) && task.data?.status !== "waiting_input" && !taskIsTerminal(task.data);
}

export function taskIsTerminal(task?: Task) {
  return (
    task?.status === "succeeded" ||
    task?.status === "partial" ||
    task?.status === "failed" ||
    task?.status === "cancelled"
  );
}

export function taskStopsPolling(task?: Task) {
  return task?.status === "waiting_input" || taskIsTerminal(task);
}

export function taskPollInterval(
  query: { state: { status: string; data?: Task } },
  interval = 500,
) {
  if (taskStopsPolling(query.state.data)) return false;
  if (query.state.status === "error") return Math.max(interval, 2_000);
  return interval;
}
