import { useState, type ReactElement } from "react";
import { Tabs } from "@base-ui/react/tabs";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/api";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
} from "@/components/ui/dialog";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import {
  TaskCancelButton,
  TaskProgressState,
  TaskStartupState,
} from "@/components/task-startup-state";
import { taskPollInterval } from "@/lib/task-state";
import { collectAnimationTasks } from "../lib/animation-task-results";
import { terminalContinuityResults } from "../lib/terminal-continuity";
import { detectionAccountRows, detectionResultGroups } from "../lib/detection-task-results";
import { DetectionResultCard } from "./detection-result-card";

export function DetectionTaskDetails(props: { id: string; onClose: () => void }): ReactElement {
  const query = useQuery({
    queryKey: ["model-detection-tasks", "run", props.id],
    queryFn: () => api.task(props.id),
    refetchInterval: taskPollInterval,
  });
  const groups = useQuery({ queryKey: ["groups"], queryFn: api.groups });
  const task = query.data;
  const state = collectAnimationTasks(task ? [task] : [], new Set<string>());
  const terminals = terminalContinuityResults(task ? [task] : []);
  const rows = detectionAccountRows(task, state, terminals);
  const grouped = detectionResultGroups(task, groups.data, rows);
  const [selectedGroupId, setSelectedGroupId] = useState<string>();
  const activeGroupId = grouped.some((group) => group.id === selectedGroupId)
    ? selectedGroupId
    : grouped[0]?.id;
  const active = task !== undefined && ["queued", "running", "waiting_input"].includes(task.status);
  const configuration = task?.result.configuration;
  const config =
    configuration && typeof configuration === "object"
      ? (configuration as Record<string, unknown>)
      : {};
  const completed = typeof task?.result.completed === "number" ? task.result.completed : 0;

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) props.onClose();
      }}
    >
      <DialogContent
        width="table"
        height="adaptive"
        className="h-[min(52rem,calc(100svh-2rem))] overflow-hidden"
      >
        <DialogHeader>
          <DialogTitle>检测任务运行详情</DialogTitle>
          <DialogDescription>
            {typeof task?.result.detection_task_name === "string"
              ? task.result.detection_task_name
              : "按所选分组查看本次检测结果"}
          </DialogDescription>
          {active && (
            <div className="rounded-lg border bg-muted/20 p-3">
              {completed > 0 ? (
                <TaskProgressState
                  message={task.message}
                  progress={task.progress}
                  taskId={task.id}
                />
              ) : (
                <>
                  <TaskStartupState message={task.message} />
                  <TaskCancelButton taskId={task.id} />
                </>
              )}
            </div>
          )}
          {task && !active && (
            <p className="text-sm text-muted-foreground wrap-anywhere">{task.message}</p>
          )}
        </DialogHeader>
        <DialogBody
          aria-label="分组检测结果"
          className="flex min-h-0 min-w-0 flex-col overflow-hidden"
        >
          {query.isPending && <ContentLoading label="正在读取检测任务" />}
          {query.isError && (
            <ContentRetry onRetry={() => void query.refetch()} pending={query.isFetching} />
          )}
          {task?.result.grouping_source === "current" && (
            <p className="mb-3 text-xs text-muted-foreground">
              此记录未保存分组快照，按当前分组归属展示。
            </p>
          )}
          {task?.result.grouping_source === "unavailable" && (
            <p className="mb-3 text-xs text-muted-foreground">
              此记录未保存分组快照，当前目录无法还原归属，保留原结果。
            </p>
          )}
          {grouped.length > 0 && activeGroupId ? (
            <Tabs.Root
              value={activeGroupId}
              onValueChange={(value) => setSelectedGroupId(String(value))}
              className="flex min-h-0 flex-1 flex-col gap-3"
            >
              <Tabs.List
                aria-label="检测分组"
                className="flex shrink-0 gap-1 overflow-x-auto border-b"
              >
                {grouped.map((group) => (
                  <Tabs.Tab
                    key={group.id}
                    value={group.id}
                    className="flex shrink-0 items-center gap-1.5 border-b-2 border-transparent px-3 py-2 text-sm whitespace-nowrap data-[active]:border-primary data-[active]:text-primary focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <span className="max-w-56 truncate">{group.name}</span>
                    <span className="text-xs text-muted-foreground">{group.rows.length}</span>
                  </Tabs.Tab>
                ))}
              </Tabs.List>
              <Tabs.Panel
                value={activeGroupId}
                className="min-h-0 flex-1 overflow-y-auto overscroll-contain pr-2"
              >
                {grouped
                  .filter((group) => group.id === activeGroupId)
                  .map((group) => (
                    <section key={group.id} aria-label={`分组 ${group.name}`} className="space-y-3">
                      <header className="flex min-w-0 items-start justify-between gap-3 border-b pb-2">
                        <h3 className="min-w-0 font-medium wrap-anywhere">{group.name}</h3>
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {group.rows.length} 个账号
                        </span>
                      </header>
                      {group.id === "__ungrouped" && (
                        <p className="text-xs text-muted-foreground">
                          旧记录未保存执行时的分组归属，保留原结果供查看。
                        </p>
                      )}
                      {group.rows.length === 0 && (
                        <p className="text-sm text-muted-foreground">暂无账号结果</p>
                      )}
                      <div className="grid min-w-0 items-start gap-3 md:grid-cols-2 lg:grid-cols-3">
                        {group.rows.map((row) => (
                          <DetectionResultCard
                            key={row.id}
                            row={row}
                            active={active}
                            precheck={config.precheck === true}
                            terminal={config.terminal === true}
                          />
                        ))}
                      </div>
                    </section>
                  ))}
              </Tabs.Panel>
            </Tabs.Root>
          ) : null}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
