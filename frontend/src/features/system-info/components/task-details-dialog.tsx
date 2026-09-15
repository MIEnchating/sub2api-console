import { useQuery } from "@tanstack/react-query";
import type { ReactElement } from "react";

import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { ContentRetry } from "@/components/content-retry";
import { QueryErrorToast } from "@/components/query-error-toast";
import { StatusBadge } from "@/components/status-badge";
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
import { Progress } from "@/components/ui/progress";
import {
  PlatformProbeResultTable,
  platformProbeResults,
} from "@/features/accounts/components/platform-probe-dialog";
import {
  activeTaskStatuses,
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
  accountNames: ReadonlyMap<string, string>;
  onClose: () => void;
}): ReactElement {
  const detail = useQuery({
    queryKey: ["task", props.taskId],
    queryFn: () => api.task(props.taskId!),
    enabled: props.taskId !== null,
    refetchInterval: (query) =>
      query.state.data && activeTaskStatuses.has(query.state.data.status) ? 1_000 : false,
  });
  const task = detail.data;
  const probeResults = platformProbeResults(task, props.accountNames);
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
              {!task && (
                <ContentRetry onRetry={() => void detail.refetch()} pending={detail.isFetching} />
              )}
            </>
          ) : null}
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
                    label={taskStatusLabel(task.status)}
                    variant={taskStatusVariant(task.status)}
                  />
                  <span className="text-muted-foreground text-xs tabular-nums">
                    进度 {task.progress}%
                  </span>
                </div>
                <p className="min-w-0 leading-6 wrap-anywhere whitespace-pre-wrap">
                  {task.message}
                </p>
                <Progress value={task.progress} aria-label="任务详情进度" />
              </div>
              {platform || model ? (
                <p className="wrap-anywhere font-medium">{probeScopeLabel(platform, model)}</p>
              ) : null}
              {probeResults.length > 0 ? (
                <PlatformProbeResultTable results={probeResults} />
              ) : (
                <p className="text-muted-foreground text-sm">此任务没有可展示的账号探活明细。</p>
              )}
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
