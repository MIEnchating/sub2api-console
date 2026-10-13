import { useId, useState, type ReactElement } from "react";
import { CheckCircle2, ChevronDown, Circle, LoaderCircle, XCircle } from "lucide-react";
import type { ProbeResult } from "@/api";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ProbeStep } from "../hooks/use-onboarding-probe-task";
import { ProbeTaskTimeline } from "./probe-task-timeline";
import { ProbeAnimationStage } from "./probe-animation-stage";

export function ProbeAnimationResult(props: {
  result: ProbeResult | null;
  pending: boolean;
  pendingMessage?: string;
  steps: ProbeStep[];
}): ReactElement {
  const [expanded, setExpanded] = useState(false);
  const detailId = useId();
  const result = props.pending ? null : props.result;
  let label = "准备就绪";
  let Icon = Circle;
  let tone = "text-muted-foreground";
  if (props.pendingMessage || props.pending) {
    label = props.pendingMessage || "动画生成中";
    Icon = LoaderCircle;
  } else if (result?.status === "passed") {
    label = "动画已生成";
    Icon = CheckCircle2;
    tone = "text-emerald-700 dark:text-emerald-400";
  } else if (result) {
    label = "测试未完成";
    Icon = XCircle;
    tone = "text-destructive";
  }
  return (
    <div className="grid min-w-0 gap-3">
      <div role="region" aria-label="探活进度" className="grid min-w-0 gap-2">
        <div className="flex min-h-8 min-w-0 items-center gap-2">
          <span
            className={cn("flex min-w-0 flex-1 items-center gap-2 text-sm", tone)}
            aria-live="polite"
          >
            <Icon
              aria-hidden="true"
              className={cn(
                "size-4 shrink-0",
                Icon === LoaderCircle && "animate-spin motion-reduce:animate-none",
              )}
            />
            <span className="min-w-0 break-words">{label}</span>
          </span>
          {result && result.latency_ms > 0 ? (
            <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
              {(result.latency_ms / 1000).toFixed(1)} 秒
            </span>
          ) : null}
          <Button
            variant="ghost"
            disabled={props.steps.length === 0 && !result}
            aria-label="查看测试过程"
            aria-expanded={expanded}
            aria-controls={detailId}
            onClick={() => setExpanded(!expanded)}
          >
            过程 <ChevronDown aria-hidden="true" className={cn(expanded && "rotate-180")} />
          </Button>
        </div>
        {result && result.status !== "passed" ? (
          <p className="break-words text-sm text-destructive [overflow-wrap:anywhere]">
            {result.message}
          </p>
        ) : null}
        {expanded ? (
          <div
            id={detailId}
            role="region"
            aria-label="测试过程详情"
            tabIndex={0}
            className="grid max-h-40 min-w-0 gap-3 overflow-y-auto overscroll-contain rounded-lg border bg-muted/30 p-3 text-xs"
          >
            {result ? (
              <div className="grid gap-1 text-muted-foreground [overflow-wrap:anywhere]">
                <p>请求模型：{result.request_model}</p>
                {result.actual_model && result.actual_model !== result.request_model ? (
                  <p>响应模型：{result.actual_model}</p>
                ) : null}
                {result.http_status > 0 ? <p>HTTP {result.http_status}</p> : null}
              </div>
            ) : null}
            <ProbeTaskTimeline steps={props.steps} />
          </div>
        ) : null}
      </div>
      <ProbeAnimationStage result={props.result} pending={props.pending} />
    </div>
  );
}
