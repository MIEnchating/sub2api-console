import { ResultModelSelector } from "./result-model-selector";
import { useState, type ReactElement } from "react";
import { TaskCancelButton, TaskStartupState } from "@/components/task-startup-state";
import type { useAnimationTasks } from "../hooks/use-animation-tasks";
import { animationPlatformLabels } from "../constants";
import { animationActivityLabel } from "../lib/animation-task-results";
import { AnimationAccountResult } from "./animation-account-result";
import { PrecheckAccountResult } from "./precheck-account-result";

export function CustomAnimationRecord(props: {
  id: string;
  tasks: ReturnType<typeof useAnimationTasks>;
  retryDisabled: boolean;
  onRetry: (model: string) => void;
}): ReactElement {
  const activity = props.tasks.activities.get(props.id);
  const [selectedModel, setSelectedModel] = useState<string>();
  const animations = props.tasks.modelResults?.get(props.id);
  const prechecks = props.tasks.modelPrecheckResults?.get(props.id);
  const choices = animations ?? prechecks;
  const current = choices?.find((item) => item.model === selectedModel) ?? choices?.[0];
  const result =
    animations?.find((item) => item.model === current?.model) ?? props.tasks.results.get(props.id);
  const precheckResult =
    prechecks?.find((item) => item.model === current?.model) ??
    props.tasks.precheckResults.get(props.id);
  const isPrecheck = props.tasks.precheckStatuses.has(props.id);
  const source = result ?? precheckResult ?? props.tasks.targets.get(props.id);
  return (
    <article
      aria-label={`自定义检测 ${source?.model ?? props.id}`}
      className="flex h-auto min-w-0 flex-col overflow-hidden rounded-xl border border-border/70 bg-card shadow-sm"
    >
      <header
        role="group"
        aria-label="检测接口信息"
        className="min-w-0 space-y-1 border-b border-border/50 bg-muted/20 px-4 py-3"
      >
        <h4 className="text-sm font-medium wrap-anywhere">{source?.model || "模型未记录"}</h4>
        <dl className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1 text-xs leading-5">
          <dt className="sr-only">接口协议</dt>
          <dd className="shrink-0 text-muted-foreground">
            {source?.platform ? animationPlatformLabels[source.platform] : "协议未记录"}
          </dd>
          <dt className="sr-only">接口地址</dt>
          <dd className="min-w-0 wrap-anywhere">{source?.endpoint || "接口地址未记录"}</dd>
        </dl>
      </header>
      <ResultModelSelector
        models={choices?.map((item) => item.model)}
        value={current?.model}
        onChange={setSelectedModel}
      />
      <div className="min-w-0 px-4 py-3">
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
        {isPrecheck ? (
          <PrecheckAccountResult
            result={precheckResult}
            status={props.tasks.precheckStatuses.get(props.id)}
          />
        ) : null}
        {result ? (
          <AnimationAccountResult
            layout="record"
            showQualityActions={false}
            showRequestID={false}
            result={result}
            retryDisabled={props.retryDisabled || props.tasks.busyIDs.has(props.id)}
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
