import type { ReactElement } from "react";
import { CircleAlert, RotateCw } from "lucide-react";
import type { AnimationResult, AnimationTarget } from "@/api";
import { ContentLoading } from "@/components/content-loading";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { AnimationActivity } from "../lib/animation-task-results";
import { AnimationPreview } from "./animation-preview";
import { AnimationResultDetails } from "./animation-result-details";

const timeFormat = new Intl.DateTimeFormat("zh-CN", {
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

export function AnimationCardResult(props: {
  result: AnimationResult;
  activity?: AnimationActivity;
  retryDisabled: boolean;
  onRetry: (target: AnimationTarget) => void;
}): ReactElement {
  const result = props.result;
  const success = result.status === "succeeded" && Boolean(result.svg);
  const modelLabel =
    result.response_model && result.response_model !== result.model
      ? `${result.model} · 返回模型 ${result.response_model}`
      : result.model;
  let preview: ReactElement;
  if (props.activity) {
    const label = props.activity.status === "starting" ? "正在启动检测" : "生成中，等待动画结果";
    preview = (
      <ContentLoading compact label={label} ariaLabel={label} className="h-full justify-center" />
    );
  } else if (success) {
    preview = <AnimationPreview result={result} className="rounded-none ring-0" />;
  } else {
    preview = (
      <div className="flex h-full flex-col items-center justify-center gap-2 bg-muted/20 px-5 text-center">
        <CircleAlert className="size-6 text-destructive" aria-hidden="true" />
        <p className="text-sm font-medium">动画生成失败</p>
        <p className="line-clamp-2 text-xs leading-5 text-muted-foreground wrap-anywhere">
          {result.error || "未返回动画内容，请重试"}
        </p>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type="button"
                variant="outline"
                size="icon"
                aria-label={`重试 ${result.account_name}`}
                disabled={props.retryDisabled}
                onClick={() =>
                  props.onRetry({ account_id: result.account_id, model: result.model })
                }
              />
            }
          >
            <RotateCw aria-hidden="true" />
          </TooltipTrigger>
          <TooltipContent>重试动画检测</TooltipContent>
        </Tooltip>
      </div>
    );
  }
  return (
    <div aria-label="动画检测结果" className="h-56 min-w-0 shrink-0">
      <div
        role="group"
        aria-label="动画预览区域"
        className="h-[180px] w-full shrink-0 overflow-hidden border-y border-border/40"
      >
        {preview}
      </div>
      <div className="flex h-11 min-w-0 items-center gap-2 px-3">
        <div className="min-w-0 flex-1 space-y-0.5 text-xs">
          <Tooltip>
            <TooltipTrigger render={<p className="truncate font-medium" />}>
              {modelLabel}
            </TooltipTrigger>
            <TooltipContent>{modelLabel}</TooltipContent>
          </Tooltip>
          <div className="flex min-w-0 items-center gap-2 text-[11px] text-muted-foreground tabular-nums">
            <span
              className={cn(
                "shrink-0",
                success ? "text-emerald-700 dark:text-emerald-400" : "text-destructive",
              )}
            >
              {success ? "成功" : "失败"}
            </span>
            <time className="truncate" dateTime={result.completed_at}>
              {timeFormat.format(new Date(result.completed_at))}
            </time>
            <span className="ml-auto shrink-0">{(result.duration_ms / 1000).toFixed(1)} 秒</span>
          </div>
        </div>
        <AnimationResultDetails result={result} />
      </div>
    </div>
  );
}
