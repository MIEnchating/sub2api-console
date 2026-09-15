import type { Task } from "@/api";

export function filterHistory(
  tasks: Task[],
  search: string,
  status: string,
  operation: string,
): Task[] {
  const needle = search.trim().toLocaleLowerCase();
  return tasks.filter((task) => {
    if (status && task.status !== status) return false;
    if (operation && task.operation !== operation) return false;
    if (!needle) return true;
    return [task.id, task.message, JSON.stringify(task.result)].some((value) =>
      value.toLocaleLowerCase().includes(needle),
    );
  });
}
