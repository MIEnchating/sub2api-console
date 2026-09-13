import { memo, type ReactElement } from "react";
import type { AnimationResult, AnimationTarget } from "@/api";
import type { AnimationActivity } from "../lib/animation-task-results";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

import { AnimationPreview } from "./animation-preview";

const resultStatusLabels = { succeeded: "成功", failed: "失败" } as const;
const compactTimeFormat = new Intl.DateTimeFormat("zh-CN", {
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

export const AnimationAccountResult = memo(function AnimationAccountResult(props: {
  result: AnimationResult;
  activity?: AnimationActivity;
  retryDisabled: boolean;
  onRetry: (target: AnimationTarget) => void;
}): ReactElement {
  const result = props.result;
  if (props.activity)
    return (
      <div
        role="status"
        aria-label={props.activity.status === "starting" ? "正在启动检测" : "生成中，等待动画结果"}
        className="flex h-full items-center justify-center text-sm text-muted-foreground"
      >
        {props.activity.status === "starting" ? "正在启动检测" : "生成中，等待动画结果"}
      </div>
    );
  const modelLabel =
    result.model +
    (result.response_model && result.response_model !== result.model
      ? ` · 返回模型 ${result.response_model}`
      : "");
  const completedAt = new Date(result.completed_at);
  return (
    <div aria-label="动画检测结果" className="flex h-full min-h-0 flex-col gap-1.5">
      <div className="min-h-0 flex-1">
        {result.status === "succeeded" && result.svg ? (
          <AnimationPreview result={result} />
        ) : (
          <div className="flex h-full min-h-0 flex-col items-start gap-2 rounded-md bg-destructive/5 p-3">
            <p className="min-h-0 w-full flex-1 overflow-y-auto break-words text-sm text-destructive">
              {result.error || "生成失败，请重试"}
            </p>
            <Button
              className="max-w-full"
              type="button"
              variant="outline"
              disabled={props.retryDisabled}
              onClick={() => props.onRetry({ account_id: result.account_id, model: result.model })}
            >
              <span className="truncate">重试 {result.account_name}</span>
            </Button>
          </div>
        )}
      </div>
      <div className="flex min-w-0 shrink-0 items-center justify-between gap-2 text-xs">
        <p className="min-w-0 truncate font-medium" title={modelLabel}>
          {modelLabel}
        </p>
        <span
          className={cn(
            "shrink-0",
            result.status === "succeeded"
              ? "text-emerald-700 dark:text-emerald-400"
              : "text-destructive",
          )}
        >
          {resultStatusLabels[result.status]}
        </span>
      </div>
      <div className="flex min-w-0 shrink-0 items-center justify-between gap-2 text-xs text-muted-foreground tabular-nums">
        <time
          className="min-w-0 truncate"
          dateTime={result.completed_at}
          title={`完成于 ${completedAt.toLocaleString("zh-CN")}`}
        >
          {compactTimeFormat.format(completedAt)}
        </time>
        <span className="shrink-0">耗时 {(result.duration_ms / 1000).toFixed(1)} 秒</span>
      </div>
    </div>
  );
});
