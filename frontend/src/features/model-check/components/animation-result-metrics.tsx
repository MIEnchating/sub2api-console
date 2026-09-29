import type { ReactElement } from "react";
import type { AnimationResult } from "@/api";
import { Badge } from "@/components/ui/badge";
import { ResultTokenMetrics } from "./result-token-metrics";

export function AnimationResultMetrics(props: {
  result: AnimationResult;
  showIdentity?: boolean;
  showUsage?: boolean;
}): ReactElement {
  const result = props.result;
  return (
    <div
      aria-label={result.mode === "precheck" ? "前置检测统计" : "动画生成统计"}
      className="w-full min-w-0 shrink-0 space-y-2 text-xs tabular-nums"
    >
      {props.showIdentity !== false ? (
        <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5">
          <span className="min-w-0 font-medium wrap-anywhere">{result.model}</span>
          {result.reasoning_effort ? (
            <Badge variant="secondary">{result.reasoning_effort}</Badge>
          ) : null}
          <span className="ml-auto text-muted-foreground">
            耗时{" "}
            {result.duration_ms > 0 ? `${(result.duration_ms / 1000).toFixed(1)} 秒` : "未记录"}
          </span>
        </div>
      ) : null}
      {props.showUsage !== false ? <ResultTokenMetrics result={result} /> : null}
    </div>
  );
}
