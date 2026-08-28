import type { AccountRecentResult } from "@/api";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

export type AccountRecentResultsProps = {
  results: readonly AccountRecentResult[];
  sampleCount?: number;
  limit?: number;
  showCount?: boolean;
  ariaLabel?: string;
  className?: string;
};

function metric(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}

function resultTone(result: AccountRecentResult): string {
  switch (result.event_type) {
    case "healthy":
      return "bg-success";
    case "slow":
      return "bg-lime-500";
    case "unknown_upstream_error":
      return "bg-amber-500";
    case "gateway_error":
    case "rate_limited_or_exhausted":
      return "bg-orange-500";
    case "probe_failed":
      return "bg-destructive";
    case "credential_invalid":
      return "bg-red-700";
  }
  const normalized = (result.result ?? "").trim().toLowerCase();
  if (
    ["通过", "成功", "正常", "passed", "pass", "success", "succeeded", "healthy", "ok"].includes(
      normalized,
    )
  ) {
    return "bg-success";
  }
  if (["超时", "timeout"].includes(normalized)) return "bg-warning";
  if (["失败", "异常", "failed", "fail", "error", "unhealthy"].includes(normalized)) {
    return "bg-destructive";
  }
  return "bg-muted-foreground/35";
}

function sourceLabel(source: string): string {
  const normalized = source.trim().toLowerCase();
  if (["active-probe", "probe"].includes(normalized)) return "主动探测";
  if (["traffic", "ops"].includes(normalized)) return "真实流量";
  return source.trim() || "来源未记录";
}

function eventLabel(result: AccountRecentResult): string {
  switch (result.event_type) {
    case "healthy":
      return "完美健康";
    case "slow":
      return "首字慢";
    case "unknown_upstream_error":
      return "上游未知异常";
    case "gateway_error":
      return "网关错误";
    case "rate_limited_or_exhausted":
      return "限流或额度不足";
    case "probe_failed":
      return "探测失败";
    case "credential_invalid":
      return "致命错误";
    default:
      return result.result ?? "未判定";
  }
}

function isResultSample(result: AccountRecentResult): boolean {
  const source = result.source.trim().toLowerCase().replaceAll("_", "-");
  return source !== "account-state";
}

function observedAtLabel(value: string | null): string {
  if (!value) return "时间未记录";
  const timestamp = new Date(value);
  if (!Number.isFinite(timestamp.getTime())) return "时间未记录";
  return timestamp.toLocaleString("zh-CN");
}

function resultDetail(result: AccountRecentResult): string {
  return [
    observedAtLabel(result.observed_at),
    eventLabel(result),
    result.score === null || result.score === undefined ? null : `${metric(result.score)} 分`,
    result.latency_ms === null ? null : `首字 ${metric(result.latency_ms)}ms`,
    sourceLabel(result.source),
    result.failure_reason,
  ]
    .filter(Boolean)
    .join(" · ");
}

export function AccountRecentResults(props: AccountRecentResultsProps) {
  const visibleResults = props.results
    .filter(isResultSample)
    .slice(0, props.limit ?? 10)
    .reverse();
  const label = props.ariaLabel ?? "最近结果";

  if (visibleResults.length === 0) {
    return (
      <span
        data-slot="account-recent-results"
        className={cn("text-muted-foreground text-xs", props.className)}
      >
        暂无样本
      </span>
    );
  }

  return (
    <div data-slot="account-recent-results" className={cn("grid gap-1.5", props.className)}>
      <div className="flex items-center gap-1" aria-label={label}>
        {visibleResults.map((result, index) => {
          const detail = resultDetail(result);
          return (
            <Tooltip key={`${result.observed_at ?? "unknown"}:${index}`}>
              <TooltipTrigger
                render={
                  <span
                    className={cn("h-4 w-2 rounded-sm", resultTone(result))}
                    tabIndex={0}
                    aria-label={detail}
                  />
                }
              />
              <TooltipContent className="max-w-sm">{detail}</TooltipContent>
            </Tooltip>
          );
        })}
      </div>
      {props.showCount && (
        <span className="text-muted-foreground text-xs tabular-nums">
          {props.sampleCount ?? props.results.length} 条样本
        </span>
      )}
    </div>
  );
}
