import type { ReactElement } from "react";
import type { AnimationResult } from "@/api";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { formatTokens, Metric, ResultTokenMetrics } from "./result-token-metrics";

const timeFormat = new Intl.DateTimeFormat("zh-CN", {
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

export function PrecheckResultMetrics(props: {
  result: AnimationResult;
  detailed?: boolean;
}): ReactElement {
  const result = props.result;
  const completedAt = new Date(result.completed_at);
  const validTime = !Number.isNaN(completedAt.getTime());
  return (
    <div
      aria-label="前置检测统计"
      className={cn(
        "w-full min-w-0 shrink-0 text-xs tabular-nums",
        props.detailed ? "space-y-3 border-t py-3" : "pt-1",
      )}
    >
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1.5">
        <span className="min-w-0 font-medium wrap-anywhere">{result.model}</span>
        {result.reasoning_effort || props.detailed ? (
          <Badge variant="secondary" aria-label="推理强度">
            {result.reasoning_effort || "未记录"}
          </Badge>
        ) : null}
        <span className="shrink-0 text-muted-foreground">
          耗时 {result.duration_ms > 0 ? `${(result.duration_ms / 1000).toFixed(1)} 秒` : "未记录"}
        </span>
        <Tooltip>
          <TooltipTrigger
            render={
              <time
                aria-label="完成时间"
                dateTime={validTime ? result.completed_at : undefined}
                className="ml-auto text-muted-foreground"
              />
            }
          >
            {validTime ? timeFormat.format(completedAt) : "完成时间未记录"}
          </TooltipTrigger>
          <TooltipContent>
            {validTime ? completedAt.toLocaleString("zh-CN") : "完成时间未记录"}
          </TooltipContent>
        </Tooltip>
      </div>
      {props.detailed ? <ResultTokenMetrics result={result} /> : null}
      {props.detailed ? (
        <dl className="grid grid-cols-3 gap-3">
          <Metric
            label="耗时"
            value={
              result.duration_ms > 0 ? `${(result.duration_ms / 1000).toFixed(1)} 秒` : "未记录"
            }
          />
          <Metric
            label="推理 Token"
            value={formatTokens(result.usage?.reasoning_tokens)}
            title="推理 Token 属于输出用量，不重复计入总计"
          />
          <Metric
            label="预估费用"
            value="无法估算"
            title="此检测记录没有对应的价格快照，无法可靠估算费用"
          />
        </dl>
      ) : null}
    </div>
  );
}
