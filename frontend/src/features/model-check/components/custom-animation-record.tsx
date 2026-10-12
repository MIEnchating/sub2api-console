import { Clapperboard, ShieldCheck } from "lucide-react";
import type { ReactElement } from "react";
import { TaskCancelButton, TaskStartupState } from "@/components/task-startup-state";
import type { useAnimationTasks } from "../hooks/use-animation-tasks";
import { detectionStageOptions } from "../constants";
import { animationActivityLabel } from "../lib/animation-task-results";
import { AnimationAccountResult } from "./animation-account-result";
import { AnimationEndpointDetails } from "./animation-endpoint-details";
import { PrecheckAccountResult } from "./precheck-account-result";

export function CustomAnimationRecord(props: {
  id: string;
  tasks: ReturnType<typeof useAnimationTasks>;
  retryDisabled: boolean;
  onRetry: (model: string) => void;
}): ReactElement {
  const activity = props.tasks.activities.get(props.id);
  const result = props.tasks.results.get(props.id);
  const precheckResult = props.tasks.precheckResults.get(props.id);
  const isPrecheck = props.tasks.precheckStatuses.has(props.id);
  const source = result ?? precheckResult ?? props.tasks.targets.get(props.id);
  const stage = detectionStageOptions[isPrecheck ? 1 : 0];
  const Icon = isPrecheck ? ShieldCheck : Clapperboard;
  return (
    <article
      aria-label={`自定义检测 ${source?.model ?? props.id}`}
      className="flex h-auto min-w-0 flex-col overflow-hidden rounded-xl border border-border/70 bg-card shadow-sm"
    >
      <header className="flex min-w-0 items-center gap-2.5 border-b border-border/50 bg-muted/20 px-4 py-3">
        <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border bg-background text-muted-foreground">
          <Icon className="size-4" aria-hidden="true" />
        </span>
        <div className="min-w-0 space-y-0.5">
          <h4 className="text-xs text-muted-foreground">{stage.label}</h4>
          <p className="text-sm font-medium wrap-anywhere">{source?.model || "模型未记录"}</p>
        </div>
      </header>
      <dl className="grid min-w-0 grid-cols-[4rem_minmax(0,1fr)] gap-x-3 gap-y-1.5 px-4 py-3 text-xs leading-5">
        <AnimationEndpointDetails source={source ?? { account_id: props.id }} />
      </dl>
      <div className="min-w-0 border-t border-border/50 px-4 py-3">
        {activity ? (
          <div className="flex min-w-0 flex-wrap items-center justify-between gap-x-3 gap-y-2">
            <TaskStartupState
              className="min-h-8 flex-1 text-xs"
              message={
                activity.mode === "precheck"
                  ? "正在执行前置检测"
                  : animationActivityLabel(activity.status)
              }
            />
            {activity.taskID ? (
              <TaskCancelButton taskId={activity.taskID} className="shrink-0" />
            ) : null}
          </div>
        ) : null}
        {!activity && isPrecheck ? (
          <PrecheckAccountResult
            result={precheckResult}
            status={props.tasks.precheckStatuses.get(props.id)}
          />
        ) : null}
        {!activity && result ? (
          <AnimationAccountResult
            layout="record"
            result={result}
            retryDisabled={props.retryDisabled}
            onRetry={() => props.onRetry(result.model)}
          />
        ) : null}
        {!activity && !result && !isPrecheck ? (
          <p className="text-sm text-muted-foreground">检测已结束，未返回动画</p>
        ) : null}
      </div>
    </article>
  );
}
