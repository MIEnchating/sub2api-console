import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Ban, RefreshCw } from "lucide-react";

import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { Button } from "@/components/ui/button";
import { notifyOperationError } from "@/lib/operation-feedback";

import { Progress } from "@/components/ui/progress";

type Props = {
  message: string;
};

type ProgressProps = Props & {
  progress: number;
  taskId?: string;
};

export const taskStartupStateLayout = {
  root: "grid min-h-12 gap-3 py-2 text-sm",
  heading: "flex min-w-0 items-center gap-2",
} as const;

export function TaskStartupState(props: Props) {
  return <ContentLoading label={props.message} compact className="min-h-12 text-sm" />;
}

export function TaskProgressState(props: ProgressProps) {
  return (
    <div
      className={taskStartupStateLayout.root}
      role="status"
      aria-live="polite"
      aria-label={props.message}
    >
      <div className={taskStartupStateLayout.heading}>
        <RefreshCw className="shrink-0 animate-spin text-primary" size={16} aria-hidden="true" />
        <span className="truncate">{props.message}</span>
        <span className="text-muted-foreground ml-auto shrink-0 tabular-nums">
          {props.progress}%
        </span>
        {props.taskId ? <TaskCancelButton taskId={props.taskId} /> : null}
      </div>
      <Progress value={props.progress} aria-label={`${props.message}进度`} />
    </div>
  );
}

export function TaskCancelButton(props: { taskId: string; className?: string; compact?: boolean }) {
  const queryClient = useQueryClient();
  const cancel = useMutation({
    mutationFn: () => api.cancelTask(props.taskId),
    onSuccess: () =>
      queryClient.invalidateQueries({
        predicate: (query) => query.queryKey.includes(props.taskId),
        refetchType: "all",
      }),
    onError: (error) => notifyOperationError(error, "任务取消失败"),
  });
  return (
    <Button
      type="button"
      size={props.compact ? "icon" : "default"}
      variant="outline"
      className={props.className}
      aria-label={props.compact ? cancelLabel(cancel.isPending, cancel.isSuccess) : undefined}
      disabled={cancel.isPending || cancel.isSuccess}
      onClick={() => cancel.mutate()}
    >
      <Ban aria-hidden="true" />
      {props.compact ? (
        <span className="sr-only">{cancelLabel(cancel.isPending, cancel.isSuccess)}</span>
      ) : (
        cancelLabel(cancel.isPending, cancel.isSuccess)
      )}
    </Button>
  );
}

function cancelLabel(pending: boolean, requested: boolean): string {
  if (pending) return "取消中";
  if (requested) return "已请求取消";
  return "取消任务";
}
