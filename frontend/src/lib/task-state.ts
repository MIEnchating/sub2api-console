import type { Task } from "../api";

export function taskIsPending(taskId: string | null, task: { data?: Task; isError: boolean }) {
  return Boolean(taskId) && !taskIsTerminal(task.data);
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
  return taskIsTerminal(task);
}

export function taskPollInterval(
  query: { state: { status: string; data?: Task } },
  interval = 500,
) {
  if (taskIsTerminal(query.state.data)) return false;
  if (query.state.data?.status === "waiting_input") return Math.max(interval, 2_000);
  if (query.state.status === "error") return Math.max(interval, 2_000);
  return interval;
}
