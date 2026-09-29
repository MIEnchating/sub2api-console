import type { QueryClient } from "@tanstack/react-query";
import type { Task, TaskSummary } from "@/api";

export function registerBackgroundTask(client: QueryClient, task: Task): void {
  const summary: TaskSummary = {
    id: task.id,
    skill: task.skill,
    operation: task.operation,
    status: task.status,
    progress: task.progress,
    message: task.message,
    created_at: task.created_at,
    updated_at: task.updated_at,
    system_info: true,
  };
  client.setQueryData(["task", task.id], task);
  client.setQueryData<TaskSummary[]>(["tasks"], (current) =>
    [summary, ...(current ?? []).filter((item) => item.id !== task.id)].slice(0, 20),
  );
  void client.invalidateQueries({ queryKey: ["tasks"] });
}
