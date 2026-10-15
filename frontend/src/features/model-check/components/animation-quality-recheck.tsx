import { useEffect, useRef, type ReactElement } from "react";
import { useIsMutating, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw, LoaderCircle } from "lucide-react";
import { toast } from "sonner";
import { api, type AnimationQualityReview, type AnimationResult } from "@/api";
import { Button } from "@/components/ui/button";
import { AnimationQualityActionButton } from "./animation-quality-action-button";
import { TaskStartupState } from "@/components/task-startup-state";
import { notifyOperationError } from "@/lib/operation-feedback";

export function qualityRecheckActive(status?: string): boolean {
  return status === "queued" || status === "running";
}

export function AnimationQualityRecheck(props: {
  result: AnimationResult;
  taskID: string;
  review?: AnimationQualityReview;
  compact?: boolean;
}): ReactElement {
  const client = useQueryClient();
  const seen = useRef<string | undefined>(undefined);
  const key = ["animation-quality", props.taskID, props.result.account_id, props.result.request_id];
  const query = useQuery({
    queryKey: key,
    queryFn: ({ signal }) =>
      api.animationQuality(props.taskID, props.result.account_id, props.result.request_id, signal),
    enabled: (query) =>
      Boolean(props.compact) && qualityRecheckActive(query.state.data?.recheck?.status),
    refetchInterval: (query) =>
      props.compact && qualityRecheckActive(query.state.data?.recheck?.status) ? 1000 : false,
    retry: false,
  });
  const task = (props.review ?? query.data)?.recheck;
  const active = qualityRecheckActive(task?.status);
  const mutationKey = [...key, "recheck"];
  const submitting = useIsMutating({ mutationKey }) > 0;
  const recheck = useMutation({
    mutationKey,
    retry: false,
    mutationFn: async () => {
      let review = props.review;
      if (!review) {
        const latest = await query.refetch();
        // Query failures are already reported by the global query handler.
        if (latest.isError) return null;
        review = latest.data;
      }
      if (!review || qualityRecheckActive(review.recheck?.status)) return null;
      return api.recheckAnimationQuality(props.taskID, {
        account_id: props.result.account_id,
        request_id: props.result.request_id,
        version: review.recheck_version ?? 0,
      });
    },
    onSuccess: (started) => {
      if (!started) return;
      const key = [
        "animation-quality",
        props.taskID,
        props.result.account_id,
        props.result.request_id,
      ];
      seen.current = started.id;
      client.setQueryData<AnimationQualityReview>(key, (current) =>
        current
          ? {
              ...current,
              recheck: { task_id: started.id, status: started.status, message: started.message },
            }
          : current,
      );
    },
    onError: (error) => notifyOperationError(error, "质量复核启动失败"),
    onSettled: () =>
      void client.invalidateQueries({
        queryKey: [
          "animation-quality",
          props.taskID,
          props.result.account_id,
          props.result.request_id,
        ],
      }),
  });
  useEffect(() => {
    if (!task) return;
    if (task.status === "succeeded")
      void client.invalidateQueries({ queryKey: ["model-check-account-statuses"] });
    if (props.compact) return;
    if (qualityRecheckActive(task.status)) {
      seen.current = task.task_id;
    } else if (seen.current === task.task_id) {
      seen.current = undefined;
      if (task.status === "succeeded") toast.success("质量复核完成");
    }
    if (task.status === "succeeded") {
      void client.invalidateQueries({ queryKey: ["model-animation", "task", props.taskID] });
    }
  }, [task, client, props.taskID, props.compact]);
  if (props.compact) {
    return (
      <AnimationQualityActionButton
        label={submitting || active ? "正在复核" : "复核"}
        description={task?.message ? `复核：${task.message}` : "复核原动画质量"}
        disabled={submitting || active}
        busy={submitting || active}
        onClick={() => recheck.mutate()}
      >
        {submitting || active ? (
          <LoaderCircle className="animate-spin" aria-hidden="true" />
        ) : (
          <RefreshCw aria-hidden="true" />
        )}
      </AnimationQualityActionButton>
    );
  }
  return (
    <div className="space-y-2">
      <Button variant="outline" disabled={submitting || active} onClick={() => recheck.mutate()}>
        <RefreshCw aria-hidden="true" />
        {active ? "正在复核" : "重新评审"}
      </Button>
      {recheck.isPending || active ? (
        <TaskStartupState message={active && task ? task.message : "正在创建质量复核任务"} />
      ) : null}
      {!recheck.isPending && task && (task.status === "failed" || task.status === "cancelled") ? (
        <p className="text-xs text-muted-foreground" role="status">
          {task.message}
        </p>
      ) : null}
    </div>
  );
}
