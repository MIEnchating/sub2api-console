import { lazy, Suspense } from "react";
import type { Task } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import type { ProbeStep } from "../hooks/use-onboarding-probe-task";
import { taskIsTerminal } from "@/lib/task-state";
import { ProbeTaskTimeline, ProbeProgressSummary } from "./probe-task-timeline";

const ModelCheckResult = lazy(() =>
  import("@/features/model-check/components/model-check-result").then((module) => ({
    default: module.ModelCheckResult,
  })),
);

export function ProbeVerificationResult(props: {
  task?: Task;
  steps: ProbeStep[];
  pendingMessage?: string;
}) {
  const task = props.task;
  const finished = taskIsTerminal(task) && !props.pendingMessage;
  const hasReport =
    task?.result.active ||
    Array.isArray(task?.result.tests) ||
    task?.status === "failed" ||
    task?.status === "cancelled";
  return (
    <div role="region" aria-label="模型检测过程与结果" className="grid min-w-0 gap-3">
      {finished && props.steps.length > 0 ? (
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer">执行过程</summary>
          <div className="pt-3">
            <ProbeTaskTimeline steps={props.steps} />
          </div>
        </details>
      ) : null}
      {!finished ? (
        <ProbeProgressSummary steps={props.steps} pendingMessage={props.pendingMessage} />
      ) : null}
      {task && hasReport ? (
        <div role="region" aria-label="模型检测报告" className="min-w-0">
          <Suspense fallback={<ContentLoading label="正在读取模型检测报告" />}>
            <ModelCheckResult task={task} embedded />
          </Suspense>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">
          选择模型后点击“开始测试”，查看模型检测过程与报告。
        </p>
      )}
    </div>
  );
}
