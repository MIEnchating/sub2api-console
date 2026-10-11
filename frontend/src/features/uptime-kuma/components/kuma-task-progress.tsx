import type { Task } from "@/api";
import { TaskProgressState, TaskStartupState } from "@/components/task-startup-state";

export type KumaProgressTask = Pick<Task, "id" | "message" | "progress">;

export function KumaTaskProgress(props: { task?: KumaProgressTask | null; starting?: boolean }) {
  if (!props.task) {
    return props.starting ? <TaskStartupState message="正在创建 Uptime Kuma 任务" /> : null;
  }
  return (
    <TaskProgressState
      message={props.task.message}
      progress={props.task.progress}
      taskId={props.task.id}
    />
  );
}
