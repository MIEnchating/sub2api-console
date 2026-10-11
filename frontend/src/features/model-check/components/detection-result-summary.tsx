import type { ReactElement } from "react";
import type { AnimationResult } from "@/api";
import { Badge } from "@/components/ui/badge";
import { AnimationEndpointDetails } from "./animation-endpoint-details";
import { ResultTokenMetrics } from "./result-token-metrics";

export function DetectionResultSummary(props: {
  result: AnimationResult;
  label: string;
}): ReactElement {
  const result = props.result;
  const modelMismatch = result.response_model && result.response_model !== result.model;
  return (
    <section aria-label={props.label} className="min-w-0 space-y-3">
      <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-2">
        <span className="text-xs text-muted-foreground">检测模型</span>
        <span className="min-w-0 text-sm font-medium wrap-anywhere">{result.model}</span>
        <Badge variant="secondary" aria-label="推理强度">
          {result.reasoning_effort || "未记录"}
        </Badge>
      </div>
      {modelMismatch ? (
        <div className="flex min-w-0 flex-wrap items-center gap-2 text-xs">
          <span className="text-muted-foreground">返回模型</span>
          <span className="min-w-0 wrap-anywhere">{result.response_model}</span>
          <Badge variant="destructive">重点：模型不一致</Badge>
        </div>
      ) : null}
      <ResultTokenMetrics result={result} detailed />
    </section>
  );
}

export function DetectionResultMetadata(props: { result: AnimationResult }): ReactElement {
  const result = props.result;
  const completedAt = new Date(result.completed_at);
  const validTime = Number.isFinite(completedAt.getTime());
  return (
    <section aria-label="请求信息" className="min-w-0 space-y-3 border-t pt-4">
      <h3 className="text-xs font-medium text-muted-foreground">请求信息</h3>
      <dl className="grid min-w-0 grid-cols-[4.5rem_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs leading-5">
        <AnimationEndpointDetails source={result} />
        <dt className="text-muted-foreground">完成时间</dt>
        <dd className="min-w-0 wrap-anywhere">
          <time aria-label="完成时间" dateTime={validTime ? result.completed_at : undefined}>
            {validTime ? completedAt.toLocaleString("zh-CN") : "未记录"}
          </time>
        </dd>
        {result.retry_count ? (
          <>
            <dt className="text-muted-foreground">自动重试</dt>
            <dd>自动重试 {result.retry_count} 次</dd>
          </>
        ) : null}
        {result.request_id ? (
          <>
            <dt className="text-muted-foreground">请求 ID</dt>
            <dd className="min-w-0 font-mono wrap-anywhere">{result.request_id}</dd>
          </>
        ) : null}
      </dl>
    </section>
  );
}
