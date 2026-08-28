import { ArrowRight, Clock3, ShieldCheck } from "lucide-react";

import type { RunEvent } from "@/api";
import { AccountHealthScore } from "@/components/account-health-score";
import { AccountRecentResults } from "@/components/account-recent-results";
import { AccountIdentityMeta } from "@/features/accounts/components/account-pool-cells";
import { StatusBadge, type StatusVariant } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { logStatusLabel, logTitleLabel } from "@/features/logs/lib/log-display";
import { cn } from "@/lib/utils";
import type { AttentionAccount, AttentionState } from "../lib/overview-health";

type OverviewActivityProps = {
  attention: AttentionAccount[];
  events: RunEvent[];
  attentionLoading: boolean;
  attentionError: unknown;
  eventsLoading: boolean;
  eventsError: unknown;
  onOpenAccounts: () => void;
  onOpenEvents: () => void;
};

const attentionLabels: Record<AttentionState, string> = {
  apply_pending: "待执行",
  fused: "已熔断",
  cost_blocked: "成本墙拦截",
  survivor: "保底中",
  degraded: "已降级",
  paused: "已暂停",
  disabled: "已停用",
};

const attentionVariants: Record<AttentionState, StatusVariant> = {
  apply_pending: "warning",
  fused: "danger",
  cost_blocked: "warning",
  survivor: "warning",
  degraded: "info",
  paused: "warning",
  disabled: "neutral",
};

function eventTone(status: string): string {
  const normalized = status.trim().toLowerCase();
  if (["failed", "error", "cancelled"].includes(normalized)) return "bg-destructive";
  if (["warning", "partial", "degraded"].includes(normalized)) return "bg-warning";
  if (["succeeded", "success", "ok"].includes(normalized)) return "bg-success";
  return "bg-info";
}

function relativeTime(value: string): string {
  const timestamp = new Date(value).getTime();
  if (!Number.isFinite(timestamp)) return "时间未记录";
  const seconds = Math.max(0, Math.floor((Date.now() - timestamp) / 1000));
  if (seconds < 60) return "刚刚";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} 分钟前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时前`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days} 天前`;
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
  }).format(timestamp);
}

function ActivitySkeleton(props: { rows: number }) {
  return (
    <div className="space-y-2 p-4">
      {Array.from({ length: props.rows }, (_, index) => (
        <div className="flex h-14 items-center gap-3" key={index}>
          <Skeleton className="size-9 shrink-0 rounded-lg" />
          <div className="min-w-0 flex-1 space-y-2">
            <Skeleton className="h-4 w-2/5" />
            <Skeleton className="h-3 w-4/5" />
          </div>
        </div>
      ))}
    </div>
  );
}

function EmptyActivity(props: { icon: React.ReactNode; title: string; detail?: string }) {
  return (
    <div className="text-muted-foreground flex min-h-48 flex-col items-center justify-center px-4 text-center">
      <span
        className="bg-muted mb-2 flex size-9 items-center justify-center rounded-lg"
        aria-hidden="true"
      >
        {props.icon}
      </span>
      <p className="text-foreground text-sm font-medium">{props.title}</p>
      {props.detail && <p className="mt-1 text-xs">{props.detail}</p>}
    </div>
  );
}

export function OverviewActivity(props: OverviewActivityProps) {
  return (
    <Card className="mt-3 grid grid-cols-1 gap-0 xl:grid-cols-5" aria-label="运营动态">
      <div className="min-w-0 xl:col-span-3 xl:border-r">
        <CardHeader>
          <CardTitle>需要关注的渠道</CardTitle>
          <CardDescription className="truncate">
            自动执行失败、熔断、停用、暂停、保底与降级渠道按风险排序。
          </CardDescription>
          <CardAction>
            <Button variant="outline" size="sm" onClick={props.onOpenAccounts}>
              全部渠道 <ArrowRight />
            </Button>
          </CardAction>
        </CardHeader>

        <CardContent className="p-0">
          {props.attentionLoading && <ActivitySkeleton rows={4} />}
          {!props.attentionLoading && !props.attentionError && props.attention.length === 0 && (
            <EmptyActivity
              icon={<ShieldCheck size={18} />}
              title="所有受管渠道都健康"
              detail="当前没有需要人工关注的渠道。"
            />
          )}
          {!props.attentionLoading && !props.attentionError && props.attention.length > 0 && (
            <ul className="divide-border divide-y px-4">
              {props.attention.slice(0, 8).map((item) => (
                <li
                  className="flex min-h-16 min-w-0 items-center gap-3 py-2.5"
                  key={item.account.id}
                >
                  <AccountHealthScore
                    score={item.account.health_score}
                    shortScore={item.account.short_score}
                    longScore={item.account.long_score}
                    sampleCount={item.account.sample_count}
                  />
                  <div className="min-w-0 flex-1">
                    <div className="flex min-w-0 items-center gap-2">
                      <p className="truncate text-sm font-medium">{item.account.name}</p>
                      <StatusBadge
                        className="shrink-0"
                        label={attentionLabels[item.state]}
                        variant={attentionVariants[item.state]}
                      />
                    </div>
                    <AccountIdentityMeta account={item.account} className="mt-0.5 block" />
                    <Tooltip>
                      <TooltipTrigger
                        render={<p className="text-muted-foreground mt-1 truncate text-xs" />}
                      >
                        {item.reason}
                      </TooltipTrigger>
                      <TooltipContent className="max-w-sm">{item.reason}</TooltipContent>
                    </Tooltip>
                  </div>
                  <AccountRecentResults
                    className="hidden shrink-0 sm:grid"
                    results={item.account.recent_results}
                    limit={8}
                    ariaLabel={`${item.account.name} 最近结果`}
                  />
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </div>

      <div className="min-w-0 border-t xl:col-span-2 xl:border-t-0">
        <CardHeader>
          <CardTitle>最近事件</CardTitle>
          <CardDescription className="truncate">最近的运行、自动执行与策略变化。</CardDescription>
          <CardAction>
            <Button variant="outline" size="sm" onClick={props.onOpenEvents}>
              全部 <ArrowRight />
            </Button>
          </CardAction>
        </CardHeader>

        <CardContent className="p-0">
          {props.eventsLoading && <ActivitySkeleton rows={4} />}
          {!props.eventsLoading && !props.eventsError && props.events.length === 0 && (
            <EmptyActivity icon={<Clock3 size={18} />} title="暂无最近事件" />
          )}
          {!props.eventsLoading && !props.eventsError && props.events.length > 0 && (
            <ul className="divide-border divide-y px-4">
              {props.events.slice(0, 8).map((event) => (
                <li className="flex min-h-16 min-w-0 items-start gap-3 py-3" key={event.id}>
                  <span
                    className={cn("mt-1.5 size-2 shrink-0 rounded-full", eventTone(event.status))}
                    aria-hidden="true"
                  />
                  <div className="min-w-0 flex-1">
                    <Tooltip>
                      <TooltipTrigger render={<p className="truncate text-sm" />}>
                        {event.summary}
                      </TooltipTrigger>
                      <TooltipContent className="max-w-sm">{event.summary}</TooltipContent>
                    </Tooltip>
                    <p className="text-muted-foreground mt-1 flex min-w-0 items-center gap-1.5 text-xs">
                      <span className="truncate">{logTitleLabel(event.event_type)}</span>
                      <span aria-hidden="true">·</span>
                      <span className="shrink-0">{logStatusLabel(event.status)}</span>
                      <span aria-hidden="true">·</span>
                      <time className="shrink-0" dateTime={event.created_at}>
                        {relativeTime(event.created_at)}
                      </time>
                    </p>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </div>
    </Card>
  );
}
