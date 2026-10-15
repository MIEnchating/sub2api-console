import type { ReactElement } from "react";
import type { AnimationResult } from "@/api";
import { AnimationQualityConfirm } from "./animation-quality-confirm";
import { AnimationQualityRecheck } from "./animation-quality-recheck";

export function AnimationCardQualityActions(props: {
  result: AnimationResult;
}): ReactElement | null {
  const result = props.result;
  const batchActive =
    result.source_task_status === "running" ||
    result.source_task_status === "queued" ||
    result.source_task_status === "waiting_input";
  if (
    (batchActive && result.phase !== "completed") ||
    !result.source_task_id ||
    !result.request_id ||
    !result.source ||
    !result.prompt ||
    !result.html ||
    result.source_truncated ||
    result.status !== "succeeded" ||
    result.mode === "precheck" ||
    result.precheck
  )
    return null;
  return (
    <div className="flex shrink-0 items-center gap-1" role="group" aria-label="质量标注操作">
      <AnimationQualityConfirm compact result={result} taskID={result.source_task_id} />
      <AnimationQualityRecheck
        key={`${result.source_task_id}:${result.account_id}:${result.request_id}`}
        compact
        result={result}
        taskID={result.source_task_id}
      />
    </div>
  );
}
