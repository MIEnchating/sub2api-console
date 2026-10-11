import { useQuery } from "@tanstack/react-query";
import { useMemo, type ReactElement } from "react";

import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { QueryErrorToast } from "@/components/query-error-toast";
import { StatusBadge } from "@/components/status-badge";
import { TaskStartupState } from "@/components/task-startup-state";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  AccountModelAutomationGroupResults,
  AccountModelAutomationResults,
} from "@/features/newapi-management/components/account-model-automation-results";
import { Progress } from "@/components/ui/progress";
import {
  PlatformProbeResultTable,
  platformProbeResults,
} from "@/features/accounts/components/platform-probe-dialog";
import {
  activeTaskStatuses,
  isTaskStarting,
  taskOperationLabel,
  taskStatusLabel,
  taskStatusVariant,
} from "../constants";
import { formatTaskDate } from "../lib/format-task-date";

function probeScopeLabel(platform: string, model: string): string {
  let displayPlatform = platform;
  if (platform.toLocaleLowerCase() === "openai") displayPlatform = "OpenAI";
  return [displayPlatform, model].filter(Boolean).join(" · ");
}

export function TaskDetailsDialog(props: {
  taskId: string | null;
  onClose: () => void;
}): ReactElement {
  const detail = useQuery({
    queryKey: ["task", props.taskId],
    queryFn: (context) => api.task(props.taskId!, context.signal),
    enabled: props.taskId !== null,
    refetchInterval: (query) => {
      const task = query.state.data;
      if (!task) return false;
      const waitingForExecution =
        task.operation === "account-model-automation" &&
        typeof task.result.schedule_id === "string" &&
        task.status === "succeeded" &&
        !task.result.execution_task_id &&
        !task.result.execution_error;
      return activeTaskStatuses.has(task.status) || waitingForExecution ? 1_000 : false;
    },
  });
  const preview = detail.data;
  const executionId =
    preview?.operation === "account-model-automation" &&
    typeof preview.result.execution_task_id === "string"
      ? preview.result.execution_task_id
      : null;
  const execution = useQuery({
    queryKey: ["task", executionId],
    queryFn: (context) => api.task(executionId!, context.signal),
    enabled: props.taskId !== null && executionId !== null,
    refetchInterval: (query) =>
      query.state.data && activeTaskStatuses.has(query.state.data.status) ? 1_000 : false,
  });
  const task = executionId ? execution.data : preview;
  const startupError =
    typeof preview?.result.execution_error === "string" ? preview.result.execution_error : null;
  const starting = task !== undefined && isTaskStarting(task) && !startupError;
  const accountNames = useMemo(
    () => new Map(Object.entries(task?.display_account_names ?? {})),
    [task?.display_account_names],
  );
  const probeResults = useMemo(
    () => platformProbeResults(task, accountNames),
    [task, accountNames],
  );
  const platform = typeof task?.result.platform === "string" ? task.result.platform : "";
  const model = typeof task?.result.model === "string" ? task.result.model : "";

  return (
    <Dialog open={props.taskId !== null} onOpenChange={(open) => !open && props.onClose()}>
      <DialogContent width={probeResults.length > 0 ? "table" : "wide"} height="adaptive">
        <DialogHeader>
          <DialogTitle>任务详情</DialogTitle>
          <DialogDescription>查看任务状态与执行结果</DialogDescription>
        </DialogHeader>
        <DialogBody className="grid gap-4">
          {detail.isLoading ? <ContentLoading label="正在读取任务详情" /> : null}
          {detail.error ? (
            <>
              <QueryErrorToast error={detail.error} fallback="任务详情读取失败，请稍后重试" />
              <ContentRetry onRetry={() => void detail.refetch()} pending={detail.isFetching} />
            </>
          ) : null}
          {executionId && execution.isPending && <ContentLoading label="正在读取正式执行任务" />}
          {execution.error && (
            <>
              <QueryErrorToast error={execution.error} fallback="正式执行任务读取失败" />
              <ContentRetry
                onRetry={() => void execution.refetch()}
                pending={execution.isFetching}
              />
            </>
          )}
          {task ? (
            <>
              <dl className="grid min-w-0 gap-3 rounded-lg border p-3 text-sm sm:grid-cols-2">
                <div className="min-w-0 sm:col-span-2">
                  <dt className="text-muted-foreground text-xs">任务类型</dt>
                  <dd className="mt-1 leading-5 wrap-anywhere">
                    {taskOperationLabel(task.operation)}
                  </dd>
                </div>
                <div className="min-w-0">
                  <dt className="text-muted-foreground text-xs">任务 ID</dt>
                  <dd className="mt-1 font-mono text-xs leading-5 wrap-anywhere">{task.id}</dd>
                </div>
                <div className="min-w-0">
                  <dt className="text-muted-foreground text-xs">更新时间</dt>
                  <dd className="mt-1 text-xs leading-5 tabular-nums">
                    <time dateTime={task.updated_at}>{formatTaskDate(task.updated_at)}</time>
                  </dd>
                </div>
              </dl>
              <div className="bg-muted/20 grid min-w-0 gap-3 rounded-lg border p-3 text-sm">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <StatusBadge
                    label={startupError ? "执行未启动" : taskStatusLabel(task.status)}
                    variant={
                      startupError ? taskStatusVariant("failed") : taskStatusVariant(task.status)
                    }
                  />
                  {!starting && (
                    <span className="text-muted-foreground text-xs tabular-nums">
                      进度 {task.progress}%
                    </span>
                  )}
                </div>
                {starting ? (
                  <TaskStartupState message={task.message || "等待任务进度"} />
                ) : (
                  <>
                    <p className="min-w-0 leading-6 wrap-anywhere whitespace-pre-wrap">
                      {startupError ? `正式执行未启动：${startupError}` : task.message}
                    </p>
                    <Progress value={task.progress} aria-label="任务详情进度" />
                  </>
                )}
              </div>
              {platform || model ? (
                <p className="wrap-anywhere font-medium">{probeScopeLabel(platform, model)}</p>
              ) : null}
              {task.operation === "account-model-automation-group" && (
                <AccountModelAutomationGroupResults task={task} />
              )}
              {task.operation === "account-model-automation" && (
                <>
                  {task.result.dry_run === true &&
                    typeof task.result.schedule_id === "string" &&
                    (activeTaskStatuses.has(task.status) || task.status === "succeeded") &&
                    !startupError && (
                      <p className="text-muted-foreground text-sm">
                        当前正在检查账号模型和上架范围，预览通过后将自动执行；关闭详情不会停止运行。
                      </p>
                    )}
                  <AccountModelAutomationResults task={task} />
                </>
              )}
              {task.operation !== "account-model-automation" &&
                task.operation !== "account-model-automation-group" &&
                (probeResults.length > 0 ? (
                  <PlatformProbeResultTable results={probeResults} />
                ) : (
                  <p className="text-muted-foreground text-sm">此任务没有可展示的账号探活明细。</p>
                ))}
            </>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={props.onClose}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
