import { useEffect, type ReactElement } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Task } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import {
  TaskCancelButton,
  TaskProgressState,
  TaskStartupState,
} from "@/components/task-startup-state";
import { Badge } from "@/components/ui/badge";
import { taskIsTerminal, taskPollInterval } from "@/lib/task-state";
import { taskStatusLabels, workbenchKeys } from "../constants";
import { workbenchResultItems } from "../lib/task-results";
import { WorkbenchTaskResults } from "./workbench-task-results";
import { WorkbenchTaskFiles } from "./workbench-task-files";

export function WorkbenchTask(props: {
  task: Task;
  onRetry?: (task: Task, indexes: number[]) => void;
}): ReactElement {
  const client = useQueryClient();
  const query = useQuery({
    queryKey: workbenchKeys.task(props.task.id),
    queryFn: () => api.task(props.task.id),
    initialData: props.task,
    refetchInterval: taskPollInterval,
  });
  const task = query.data;
  const terminal = taskIsTerminal(task);
  useEffect(() => {
    if (!terminal) return;
    void client.invalidateQueries({ queryKey: workbenchKeys.history });
    void client.invalidateQueries({ queryKey: ["accounts"] });
    void client.invalidateQueries({ queryKey: workbenchKeys.maintenance });
    void client.invalidateQueries({ queryKey: workbenchKeys.exports });
    void client.invalidateQueries({ queryKey: workbenchKeys.localExports });
    void client.invalidateQueries({ queryKey: workbenchKeys.profiles });
  }, [client, terminal]);
  const items = workbenchResultItems(task.result);
  const canRetry =
    terminal &&
    (task.operation === "account-workbench-import" || task.operation === "account-workbench-retry");
  return (
    <article
      className="min-w-0 space-y-3 rounded-lg border bg-card p-4"
      aria-label={`处理任务 ${task.id}`}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="min-w-0 text-sm font-medium wrap-anywhere">任务 {task.id}</h2>
        <Badge variant="secondary">{taskStatusLabels[task.status]}</Badge>
      </div>
      {!terminal && (task.status === "queued" || task.progress <= 0) && (
        <>
          <TaskStartupState message={task.message || "正在等待账号处理进度"} />
          <TaskCancelButton taskId={task.id} />
        </>
      )}
      {!terminal && task.status !== "queued" && task.progress > 0 && (
        <TaskProgressState
          taskId={task.id}
          progress={task.progress}
          message={task.message || "正在处理账号"}
        />
      )}
      {terminal && (
        <p role="status" className="text-sm wrap-anywhere">
          {task.message || taskStatusLabels[task.status]}
        </p>
      )}
      {query.isError && (
        <ContentRetry pending={query.isFetching} onRetry={() => void query.refetch()} />
      )}
      {items.length > 0 && (
        <WorkbenchTaskResults
          items={items}
          onRetry={
            canRetry && props.onRetry ? (indexes) => props.onRetry?.(task, indexes) : undefined
          }
        />
      )}
      {terminal && <WorkbenchTaskFiles task={task} />}
    </article>
  );
}
