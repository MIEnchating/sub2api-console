import type { ReactElement } from "react";
import type { AccountRecentResult } from "@/api";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { formatHealthScore } from "@/lib/health-score";

export type AccountRecentResultsProps = {
  results: readonly AccountRecentResult[];
  sampleCount?: number;
  limit?: number;
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
  const normalized = source.trim().toLowerCase().replaceAll("_", "-");
  if (["active-probe", "probe"].includes(normalized)) return "探针";
  if (["traffic", "ops", "logs"].includes(normalized)) return "真实流量";
  return source.trim() || "来源未记录";
}

function eventLabel(result: AccountRecentResult): string {
  switch (result.event_type) {
    case "healthy":
      if (sourceLabel(result.source) === "探针") return "探测通过";
      return "完美健康";
    case "slow":
      return "响应慢";
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
    result.score === null || result.score === undefined
      ? null
      : `${formatHealthScore(result.score)} 分`,
    result.latency_ms === null ? null : `首字 ${metric(result.latency_ms)}ms`,
    result.duration_ms === null || result.duration_ms === undefined
      ? null
      : `总耗时 ${metric(result.duration_ms)}ms`,
    sourceLabel(result.source),
    result.failure_reason,
  ]
    .filter(Boolean)
    .join(" · ");
}

export function AccountRecentResults(props: AccountRecentResultsProps): ReactElement {
  const visibleResults = props.results
    .filter(isResultSample)
    .slice(0, props.limit ?? 10)
    .reverse();
  const label = props.ariaLabel ?? "最近结果";

  return (
    <div data-slot="account-recent-results" className={cn("grid w-fit gap-2", props.className)}>
      <div className="grid gap-1.5" role="group" aria-label={label}>
        <ResultSourceRow
          label="真实流量"
          slots={props.limit ?? 10}
          results={visibleResults.filter((result) => sourceLabel(result.source) === "真实流量")}
        />
        <ResultSourceRow
          label="探针"
          slots={props.limit ?? 10}
          results={visibleResults.filter((result) => sourceLabel(result.source) === "探针")}
        />
        {visibleResults.some(
          (result) => !["真实流量", "探针"].includes(sourceLabel(result.source)),
        ) ? (
          <ResultSourceRow
            label="其他"
            slots={props.limit ?? 10}
            results={visibleResults.filter(
              (result) => !["真实流量", "探针"].includes(sourceLabel(result.source)),
            )}
          />
        ) : null}
      </div>
      {props.sampleCount === undefined ? null : (
        <Tooltip>
          <TooltipTrigger
            render={
              <span
                className="text-muted-foreground w-fit text-xs tabular-nums"
                tabIndex={0}
                aria-label={`有效样本 ${props.sampleCount}，用于本轮健康评分`}
              />
            }
          >
            有效样本 {props.sampleCount}
          </TooltipTrigger>
          <TooltipContent className="max-w-xs text-xs">
            用于本轮健康评分的有效结果数量，受评分历史范围和长期条数限制；有新鲜流量时采用流量历史及更新的探针，否则采用探针历史。须有新鲜有效证据；熔断恢复仅用新鲜恢复样本。探针慢响应参与扣分，真实性能统计仅使用真实请求首字。
          </TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

function ResultSourceRow(props: {
  label: string;
  slots: number;
  results: readonly AccountRecentResult[];
}): ReactElement {
  return (
    <div
      className="grid grid-cols-[2.75rem_auto] items-center gap-2"
      role="group"
      aria-label={`${props.label}结果`}
    >
      <span className="text-muted-foreground text-[11px]! leading-4">{props.label}</span>
      <div
        className="flex min-h-4 items-center gap-0.5"
        role={props.results.length === 0 ? "img" : undefined}
        aria-label={props.results.length === 0 ? `${props.label}无结果` : undefined}
      >
        {props.results.map((result, index) => {
          const detail = resultDetail(result);
          return (
            <Tooltip
              key={result.id ?? `${result.source}:${result.observed_at ?? "unknown"}:${index}`}
            >
              <TooltipTrigger
                render={
                  <span
                    className={cn(
                      "focus-visible:ring-ring h-4 w-2 shrink-0 rounded-[2px] outline-none hover:opacity-75 focus-visible:ring-2 focus-visible:ring-offset-2",
                      resultTone(result),
                    )}
                    tabIndex={0}
                    aria-label={detail}
                  />
                }
              />
              <TooltipContent role="tooltip" className="max-w-sm break-words">
                {detail}
              </TooltipContent>
            </Tooltip>
          );
        })}
        {Array.from({ length: Math.max(0, props.slots - props.results.length) }, (_, index) => (
          <span
            key={`empty:${index}`}
            className="bg-muted-foreground/20 h-4 w-2 shrink-0 rounded-[2px]"
            aria-hidden="true"
          />
        ))}
      </div>
    </div>
  );
}
