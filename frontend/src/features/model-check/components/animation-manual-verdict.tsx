import { useIsMutating, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CircleCheck, CircleX, RotateCcw } from "lucide-react";
import { useCallback, type ReactElement } from "react";
import { api, type AnimationManualVerdict, type AnimationResult, type Task } from "@/api";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { notifyOperationError } from "@/lib/operation-feedback";
import { animationManualVerdictLabels, detectionResultToneClasses } from "../constants";
import {
  selectAnimationTaskResult,
  type AnimationTaskSelection,
} from "../lib/animation-task-results";

export function AnimationManualVerdictControls(props: {
  result: AnimationResult;
  disabled?: boolean;
  compact?: boolean;
}): ReactElement | null {
  if (!props.result.source_task_id || props.result.mode === "precheck") return null;
  return <ManualVerdictEditor {...props} taskID={props.result.source_task_id} />;
}

function ManualVerdictEditor(props: {
  result: AnimationResult;
  taskID: string;
  disabled?: boolean;
  compact?: boolean;
}): ReactElement {
  const client = useQueryClient();
  const saving =
    useIsMutating({
      mutationKey: ["animation-manual-verdict", props.taskID, props.result.request_id],
    }) > 0;
  const selectResult = useCallback(
    (task: Task): AnimationTaskSelection =>
      selectAnimationTaskResult(task, props.result.account_id, props.result.request_id),
    [props.result.account_id, props.result.request_id],
  );
  const cachedTask = useQuery({
    queryKey: ["model-animation", "task", props.taskID],
    queryFn: ({ signal }) => api.task(props.taskID, signal),
    enabled: false,
    select: selectResult,
  });
  const result = cachedTask.data?.result ?? props.result;
  const save = useMutation({
    mutationKey: ["animation-manual-verdict", props.taskID, props.result.request_id],
    mutationFn: (verdict: AnimationManualVerdict["verdict"]) =>
      api.setAnimationManualVerdict(props.taskID, {
        account_id: props.result.account_id,
        request_id: props.result.request_id,
        verdict,
      }),
    onSuccess: async (task) => {
      await Promise.all([
        client.cancelQueries({ queryKey: ["model-animation", "task", task.id], exact: true }),
        client.cancelQueries({ queryKey: ["task", task.id], exact: true }),
      ]);
      client.setQueryData(["model-animation", "task", task.id], task);
      client.setQueryData(["task", task.id], task);
      client.setQueryData<Task[]>(["model-animation", "history"], (history) =>
        history?.map((item) => (item.id === task.id ? task : item)),
      );
      void client.invalidateQueries({ queryKey: ["model-animation", "history"] });
      void client.invalidateQueries({ queryKey: ["model-check-account-statuses"] });
    },
    onError: (error) => notifyOperationError(error, "手动标记保存失败"),
  });
  const status = cachedTask.data?.status ?? result.source_task_status;
  const active = status === "queued" || status === "running" || status === "waiting_input";
  const disabled = props.disabled || active || saving;
  const verdict = result.manual_verdict?.verdict ?? "unmarked";
  return (
    <section
      aria-label="动画手动标记"
      aria-description="仅标记本次动画结果，重测后需重新判断"
      className={cn("min-w-0", !props.compact && "space-y-1.5")}
    >
      <div
        className={cn("flex items-center", props.compact ? "gap-1" : "flex-wrap gap-2")}
        role="group"
        aria-label="手动判断动画结果"
      >
        {!props.compact && (
          <Tooltip>
            <TooltipTrigger
              render={<span tabIndex={0} className="text-xs text-muted-foreground" />}
            >
              手动标记
            </TooltipTrigger>
            <TooltipContent>仅标记本次动画结果，重测后需重新判断</TooltipContent>
          </Tooltip>
        )}
        {(["not_passed", "passed"] as const).map((value) => {
          const Icon = value === "passed" ? CircleCheck : CircleX;
          const button = (
            <Button
              key={value}
              type="button"
              variant={props.compact ? "ghost" : "outline"}
              size={props.compact ? "icon" : "default"}
              aria-label={animationManualVerdictLabels[value]}
              aria-pressed={verdict === value}
              disabled={disabled}
              className={cn(
                verdict === value &&
                  detectionResultToneClasses[value === "passed" ? "success" : "danger"],
              )}
              onClick={() => save.mutate(value)}
            >
              <Icon aria-hidden="true" />
              {!props.compact && animationManualVerdictLabels[value]}
            </Button>
          );
          if (!props.compact) return button;
          return (
            <Tooltip key={value}>
              <TooltipTrigger render={button} />
              <TooltipContent>{animationManualVerdictLabels[value]}</TooltipContent>
            </Tooltip>
          );
        })}
        {verdict !== "unmarked" ? (
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="清除标记"
                  disabled={disabled}
                  onClick={() => save.mutate("unmarked")}
                >
                  <RotateCcw aria-hidden="true" />
                </Button>
              }
            />
            <TooltipContent>清除标记</TooltipContent>
          </Tooltip>
        ) : null}
      </div>
      {save.isPending ? (
        <p role="status" className={props.compact ? "sr-only" : "text-xs text-muted-foreground"}>
          正在保存标记
        </p>
      ) : null}
      {active ? (
        <p className={props.compact ? "sr-only" : "text-xs text-muted-foreground"}>
          本批检测完成后可标记
        </p>
      ) : null}
    </section>
  );
}
