import type { ReactElement } from "react";
import type { AnimationResult, Task } from "@/api";
import { TaskStartupState } from "@/components/task-startup-state";
import { Badge } from "@/components/ui/badge";
import { detectionResultToneClasses, precheckVerdictLabels } from "../constants";
import type { AnimationActivity } from "../lib/animation-task-results";
import { PrecheckResultDetails } from "./precheck-result-details";
import { PrecheckResultMetrics } from "./precheck-result-metrics";

export function PrecheckAccountResult(props: {
  result?: AnimationResult;
  activity?: AnimationActivity;
  status?: Task["status"];
}): ReactElement {
  const running = props.activity?.mode === "precheck";
  const check = props.result?.precheck;
  let label = "未检测";
  let variant: "destructive" | "warning" | "outline" = "outline";
  if (running) {
    label = "检测中";
  } else if (check) {
    label = precheckVerdictLabels[check.verdict];
    variant = "warning";
    if (check.verdict === "passed") variant = "outline";
    if (check.verdict === "not_passed" || check.verdict === "error") variant = "destructive";
  } else if (props.status === "cancelled") {
    label = "已取消";
  } else if (props.result?.error) {
    label = "检测失败";
    variant = "destructive";
  }

  return (
    <section aria-label="前置检测结果" className="h-auto w-full min-w-0 shrink-0 py-2 text-xs">
      <div className="flex h-8 min-w-0 items-center gap-2">
        <span className="shrink-0 font-medium">前置检测</span>
        <Badge
          variant={variant}
          className={
            !running && check?.verdict === "passed" ? detectionResultToneClasses.success : undefined
          }
        >
          {label}
        </Badge>
        {props.result && !running ? <PrecheckResultDetails result={props.result} /> : null}
      </div>
      {running ? <TaskStartupState message="正在执行前置检测" className="min-h-6 text-xs" /> : null}
      {props.result && !running ? <PrecheckResultMetrics result={props.result} /> : null}
    </section>
  );
}
