import type { ReactElement } from "react";

import type { TrafficRanking, TrafficRankingRow as RankingRow } from "@/api";
import { Badge } from "@/components/ui/badge";
import { TableCell, TableRow } from "@/components/ui/table";
import { cn } from "@/lib/utils";

import {
  formatTrafficCachePercent,
  formatTrafficCount,
  formatTrafficLatency,
  formatTrafficPercent,
  formatTrafficTokens,
  trafficStabilityLabel,
} from "../lib/traffic-ranking";

function MetricLine(props: { label: string; value: string; secondary?: boolean }): ReactElement {
  return (
    <div className={cn("flex items-baseline justify-between gap-2", props.secondary && "mt-1")}>
      <span className="text-muted-foreground shrink-0 text-xs">{props.label}</span>
      <span
        className={cn(
          "min-w-0 truncate",
          props.secondary ? "text-muted-foreground text-xs" : "font-medium",
        )}
        title={props.value}
      >
        {props.value}
      </span>
    </div>
  );
}

function LatestTraffic(props: { value: string | null }): ReactElement {
  if (props.value === null) return <span>-</span>;
  return (
    <time dateTime={props.value} title={new Date(props.value).toLocaleString("zh-CN")}>
      {new Date(props.value).toLocaleString("zh-CN", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      })}
    </time>
  );
}

export function TrafficRankingRow(props: {
  row: RankingRow;
  bucket: TrafficRanking["bucket"];
}): ReactElement {
  const row = props.row;
  const stability = trafficStabilityLabel(row.stability_score);
  const identity = `#${row.account_id} · ${row.groups.join("、") || "未分组"}`;
  const upstream = row.upstream_host || row.platform || "未标记上游";

  return (
    <TableRow>
      <TableCell className="sticky left-0 z-10 bg-card px-3 py-3 shadow-[1px_0_0_var(--border)] group-hover:[background-color:color-mix(in_oklch,var(--muted)_50%,var(--background))]">
        <div className="flex min-w-0 items-center gap-3">
          <span
            aria-label={`第 ${row.rank} 名`}
            className={cn(
              "flex size-7 shrink-0 items-center justify-center rounded-md text-xs font-semibold",
              row.rank <= 3
                ? "bg-secondary text-secondary-foreground"
                : "bg-muted text-muted-foreground",
            )}
          >
            {row.rank}
          </span>
          <div className="min-w-0 flex-1 space-y-0.5">
            <div className="truncate font-medium" title={row.account_name}>
              {row.account_name}
            </div>
            <div className="text-muted-foreground truncate text-xs" title={identity}>
              {identity}
            </div>
            <div className="text-muted-foreground truncate text-xs" title={upstream}>
              {upstream}
            </div>
          </div>
        </div>
      </TableCell>
      <TableCell className="px-3 text-right">
        <div className="font-semibold">{formatTrafficCount(row.requests)}</div>
        <div className="text-muted-foreground mt-1 flex items-center justify-end gap-2 text-xs">
          {row.traffic_share !== null && (
            <span
              aria-hidden="true"
              className="bg-primary/10 h-1.5 w-12 overflow-hidden rounded-full"
            >
              <span
                className="bg-primary block h-full rounded-full"
                style={{ width: `${Math.min(100, Math.max(0, row.traffic_share))}%` }}
              />
            </span>
          )}
          <span>{formatTrafficPercent(row.traffic_share)}</span>
        </div>
      </TableCell>
      <TableCell className="px-3 text-right">
        <div className="font-medium">{formatTrafficPercent(row.stability_score)}</div>
        <div className="mt-1 flex justify-end">
          <Badge variant={stability.variant}>{stability.label}</Badge>
        </div>
      </TableCell>
      <TableCell className="px-3 text-right">
        <div className="font-medium">{formatTrafficPercent(row.success_rate)}</div>
        <div
          className="text-muted-foreground mt-1 text-xs"
          title={`成功 ${formatTrafficCount(row.successful)} 次 / 失败 ${formatTrafficCount(row.failed)} 次`}
        >
          {formatTrafficCount(row.successful)} /{" "}
          <span className={cn(row.failed > 0 && "text-destructive")}>
            {formatTrafficCount(row.failed)}
          </span>
        </div>
      </TableCell>
      <TableCell className="px-3 text-right">
        <MetricLine label="平均" value={formatTrafficLatency(row.average_latency_ms)} />
        <MetricLine label="P95" value={formatTrafficLatency(row.p95_latency_ms)} secondary />
      </TableCell>
      <TableCell className="px-3 text-right">
        <div className="font-medium">
          {row.active_buckets} / {row.total_buckets} {props.bucket === "day" ? "天" : "小时"}
        </div>
        <div className="text-muted-foreground mt-1 text-xs">
          <LatestTraffic value={row.latest_at} />
        </div>
      </TableCell>
      <TableCell className="px-3 text-right">
        {row.usage_available ? (
          <>
            <MetricLine label="输入" value={formatTrafficTokens(row.input_tokens)} />
            <MetricLine label="输出" value={formatTrafficTokens(row.output_tokens)} secondary />
          </>
        ) : (
          <span className="text-muted-foreground text-xs">未提供</span>
        )}
      </TableCell>
      <TableCell className="px-3 text-right">
        {row.usage_available ? (
          <>
            <MetricLine label="读取" value={formatTrafficCachePercent(row, "cache_read_tokens")} />
            <MetricLine
              label="写入"
              value={formatTrafficCachePercent(row, "cache_write_tokens")}
              secondary
            />
          </>
        ) : (
          <span className="text-muted-foreground text-xs">未提供</span>
        )}
      </TableCell>
    </TableRow>
  );
}
