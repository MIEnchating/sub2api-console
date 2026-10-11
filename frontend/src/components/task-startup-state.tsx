import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Ban, RefreshCw } from "lucide-react";
import { useEffect, useRef } from "react";

import { api } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { Button } from "@/components/ui/button";
import { notifyOperationError } from "@/lib/operation-feedback";
import { cn } from "@/lib/utils";

import { Progress } from "@/components/ui/progress";

type Props = {
  message: string;
};

type ProgressProps = Props & {
  progress: number;
  taskId?: string;
};

export const taskStartupStateLayout = {
  root: "grid min-h-12 min-w-0 gap-3 py-2 text-sm",
  heading: "flex min-w-0 items-center gap-2",
} as const;

export function TaskStartupState(props: Props & { className?: string }) {
  return (
    <ContentLoading
      label={props.message}
      compact
      className={cn("min-h-12 text-sm", props.className)}
    />
  );
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
        <RefreshCw
          className="shrink-0 motion-safe:animate-spin text-primary"
          size={16}
          aria-hidden="true"
        />
        <span className="min-w-0 flex-1 wrap-anywhere">{props.message}</span>
        <span className="text-muted-foreground ml-auto shrink-0 tabular-nums">
          {props.progress}%
        </span>
        {props.taskId ? <TaskCancelButton taskId={props.taskId} /> : null}
      </div>
      <Progress value={props.progress} aria-label={`${props.message}进度`} />
    </div>
  );
}

type CancelProps = { taskId: string; className?: string; compact?: boolean };

export function TaskCancelButton(props: CancelProps) {
  return <TaskCancelAction key={props.taskId} {...props} />;
}

function TaskCancelAction(props: CancelProps) {
  const queryClient = useQueryClient();
  const submitted = useRef(false);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const cancel = useMutation({
    retry: false,
    networkMode: "always",
    mutationFn: async (taskId: string) => {
      const result = await api.cancelTask(taskId);
      if (result.cancelled !== true) {
        throw new Error("服务未确认取消，请刷新任务状态后重试");
      }
      return result;
    },
    onError: (error) => {
      submitted.current = false;
      if (mounted.current) notifyOperationError(error, "任务取消失败");
    },
    onSettled: (_data, _error, taskId) => {
      // Feature pages use several task query aliases. Mark all matching caches
      // stale, but only fetch observed queries; a closed detail can be large.
      // Refresh on failure too: the task may have finished before cancellation.
      void queryClient.invalidateQueries({
        predicate: (query) =>
          query.queryKey.includes(taskId) ||
          (query.queryKey.length === 1 && query.queryKey[0] === "tasks"),
        refetchType: "active",
      });
    },
  });
  return (
    <Button
      type="button"
      size={props.compact ? "icon" : "default"}
      variant="outline"
      className={props.className}
      aria-label={props.compact ? cancelLabel(cancel.isPending, cancel.isSuccess) : undefined}
      disabled={cancel.isPending || cancel.isSuccess}
      onClick={() => {
        if (submitted.current) return;
        submitted.current = true;
        cancel.mutate(props.taskId);
      }}
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
