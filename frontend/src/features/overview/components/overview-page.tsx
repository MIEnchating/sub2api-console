import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, Bolt, RefreshCw, ServerCog, ShieldCheck, TriangleAlert } from "lucide-react";
import { toast } from "sonner";

import { api } from "@/api";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { StatusBadge, type StatusVariant } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { terminalRefreshKeys } from "@/lib/task-refresh";
import { taskPollInterval, taskStopsPolling } from "@/lib/task-state";
import { cn } from "@/lib/utils";
import { OverviewActivity } from "./overview-activity";
import {
  buildAttentionAccounts,
  buildGroupHealth,
  buildOverviewMetrics,
  overviewAccounts,
  strategyLabel,
  visibleOverviewGroups,
  type GroupHealth,
  type HealthTone,
} from "../lib/overview-health";

type OverviewPageProps = {
  onOpenGroups: () => void;
  onOpenAccounts: () => void;
  onOpenEvents: () => void;
};

const toneStyles: Record<HealthTone, { variant: StatusVariant; progress: string; detail: string }> =
  {
    healthy: {
      variant: "success",
      progress: "bg-success",
      detail: "text-success",
    },
    warning: {
      variant: "warning",
      progress: "bg-warning",
      detail: "text-warning",
    },
    critical: {
      variant: "danger",
      progress: "bg-destructive",
      detail: "text-destructive",
    },
  };

function formatNumber(value: number): string {
  return new Intl.NumberFormat("zh-CN").format(value);
}

function groupHealthDetail(health: GroupHealth): string {
  if (health.group.participation_reason) return health.group.participation_reason;
  if (health.needsAttention > 0) {
    const rateLimited =
      health.rateLimitedCount > 0 ? ` · ${health.rateLimitedCount} 个限流中（会自愈）` : "";
    return `${health.needsAttention} 个渠道需要处理${rateLimited}`;
  }
  if (health.rateLimitedCount > 0) {
    return `${health.rateLimitedCount} 个渠道限流中，窗口重置后自动恢复`;
  }
  if (health.totalCount === 0) return "分组内暂无渠道";
  return "所有渠道均可参与调度";
}

function MetricCard(props: {
  label: string;
  value: string | number | null;
  detail: string;
  icon: React.ReactNode;
  tone: "teal" | "red";
  loading?: boolean;
}) {
  return (
    <Card className="min-h-24 gap-0 p-4">
      <div className="flex min-w-0 items-center gap-3">
        <span
          className={cn(
            "flex size-9 shrink-0 items-center justify-center rounded-lg",
            props.tone === "teal"
              ? "bg-success/12 text-success dark:bg-success/18"
              : "bg-destructive/10 text-destructive dark:bg-destructive/16",
          )}
          aria-hidden="true"
        >
          {props.icon}
        </span>
        <div className="min-w-0 flex-1">
          <p className="text-muted-foreground truncate text-sm">{props.label}</p>
          {props.loading ? (
            <Skeleton className="mt-1.5 h-6 w-20" />
          ) : (
            <strong className="mt-0.5 block truncate text-xl leading-6 font-semibold tabular-nums">
              {props.value ?? "—"}
            </strong>
          )}
          <p className="text-muted-foreground mt-1 truncate text-xs">{props.detail}</p>
        </div>
      </div>
    </Card>
  );
}

function GroupHealthCard(props: { health: GroupHealth; onOpen: () => void }) {
  const styles = toneStyles[props.health.tone];
  return (
    <button
      type="button"
      className="border-border bg-muted/15 hover:bg-muted/35 focus-visible:border-ring focus-visible:ring-ring group flex min-h-48 w-full min-w-0 flex-col rounded-[6px] border p-4 text-left transition-colors outline-none focus-visible:ring-2"
      onClick={props.onOpen}
      aria-label="打开分组管理"
    >
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold">{props.health.group.name}</h3>
          <p className="text-muted-foreground mt-1 truncate text-xs">
            {props.health.group.id ? `#${props.health.group.id}` : "无分组 ID"} ·{" "}
            {props.health.platformSummary}
            {props.health.averageMultiplier === null
              ? ""
              : ` · 倍率 ${props.health.averageMultiplier}`}
          </p>
        </div>
        <StatusBadge label={props.health.statusLabel} variant={styles.variant} />
      </div>

      <div className="mt-4 flex items-end justify-between gap-3">
        <StatusBadge label={strategyLabel(props.health.group.strategy)} variant="info" />
        <div className="text-right">
          <strong className="block text-lg leading-5 font-semibold tabular-nums">
            {props.health.liveCount}/{props.health.totalCount}
          </strong>
          <span className="text-muted-foreground mt-1 block text-xs">存活 / 总数</span>
        </div>
      </div>

      <div className="mt-4 flex items-center gap-3">
        <span className="bg-muted h-2 min-w-0 flex-1 overflow-hidden rounded-full">
          <span
            className={cn(
              "block h-full rounded-full transition-[width] duration-300",
              styles.progress,
            )}
            style={{ width: `${props.health.livePercent}%` }}
          />
        </span>
        <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
          {props.health.livePercent}% · 均分 {props.health.healthScore ?? "—"} ·{" "}
          {props.health.scoredCount}/{props.health.totalCount} 有评分
        </span>
      </div>

      <p className={cn("mt-auto line-clamp-2 pt-4 text-xs", styles.detail)}>
        {groupHealthDetail(props.health)}
      </p>
    </button>
  );
}

function MatrixSkeleton() {
  return Array.from({ length: 6 }, (_, index) => (
    <div className="border-border min-h-48 rounded-[6px] border p-4" key={index}>
      <div className="flex justify-between gap-4">
        <Skeleton className="h-6 w-28" />
        <Skeleton className="h-6 w-20 rounded-full" />
      </div>
      <Skeleton className="mt-2 h-4 w-40" />
      <div className="mt-8 flex items-end justify-between">
        <Skeleton className="h-5 w-20 rounded-full" />
        <Skeleton className="h-9 w-14" />
      </div>
      <Skeleton className="mt-5 h-2 w-full" />
      <Skeleton className="mt-5 h-4 w-40" />
    </div>
  ));
}

export function OverviewPage(props: OverviewPageProps) {
  const queryClient = useQueryClient();
  const [syncTaskId, setSyncTaskId] = useState<string | null>(null);
  const accounts = useQuery({
    queryKey: ["accounts"],
    queryFn: api.accounts,
    refetchInterval: 30_000,
  });
  const groups = useQuery({
    queryKey: ["groups"],
    queryFn: api.groups,
    refetchInterval: 30_000,
    refetchOnMount: "always",
  });
  const events = useQuery({
    queryKey: ["overview-events"],
    queryFn: api.recentEvents,
    refetchInterval: 15_000,
  });
  const sync = useMutation({
    mutationFn: api.syncManagement,
    onSuccess: (task) => setSyncTaskId(task.id),
    onError: (error) => toast.error(error instanceof Error ? error.message : "同步启动失败"),
  });
  const syncTask = useQuery({
    queryKey: ["overview-sync", syncTaskId],
    queryFn: () => api.task(syncTaskId!),
    enabled: Boolean(syncTaskId),
    refetchInterval: taskPollInterval,
  });

  useEffect(() => {
    if (!taskStopsPolling(syncTask.data)) return;
    for (const queryKey of terminalRefreshKeys("management-sync", syncTask.data)) {
      void queryClient.invalidateQueries({ queryKey });
    }
    if (syncTask.data?.status === "succeeded") {
      toast.success("运营数据已同步");
    } else if (syncTask.data?.status === "failed") {
      toast.error(syncTask.data.message || "运营数据同步失败");
    }
    setSyncTaskId(null);
  }, [queryClient, syncTask.data]);

  const accountRows = accounts.data ?? [];
  const groupRows = groups.data ?? [];
  const visibleGroups = useMemo(() => visibleOverviewGroups(groupRows), [groupRows]);
  const managedAccounts = useMemo(
    () => overviewAccounts(accountRows, visibleGroups),
    [accountRows, visibleGroups],
  );
  const metrics = useMemo(
    () => buildOverviewMetrics(managedAccounts, visibleGroups),
    [managedAccounts, visibleGroups],
  );
  const healthRows = useMemo(
    () => visibleGroups.map((group) => buildGroupHealth(group, managedAccounts)),
    [managedAccounts, visibleGroups],
  );
  const attention = useMemo(
    () => buildAttentionAccounts(accountRows, visibleGroups),
    [accountRows, visibleGroups],
  );
  const loading = accounts.isLoading || groups.isLoading;
  const syncing = sync.isPending || Boolean(syncTaskId);
  const error = accounts.error ?? groups.error;

  const accountDetail = `${metrics.healthyAccounts} 健康 · ${metrics.degradedAccounts} 降级 · ${metrics.fusedAccounts} 熔断${metrics.costBlockedAccounts ? ` · ${metrics.costBlockedAccounts} 成本拦截` : ""}${metrics.pausedAccounts ? ` · ${metrics.pausedAccounts} 暂停` : ""}${metrics.disabledAccounts ? ` · ${metrics.disabledAccounts} 停用` : ""}${metrics.survivorAccounts ? ` · ${metrics.survivorAccounts} 保底` : ""}${metrics.unknownAccounts ? ` · ${metrics.unknownAccounts} 待观察` : ""}`;
  const healthDetail =
    metrics.managedAccounts === 0
      ? "暂无渠道健康样本"
      : `${metrics.managedAccounts} 个渠道，${metrics.managedAccounts - metrics.unknownAccounts} 个已有状态`;

  return (
    <PageLayout>
      <PageHeading
        eyebrow="OPERATIONS / OVERVIEW"
        title="运营总览"
        description="汇总受管渠道、分组健康与近期运行状态。"
        action={
          <PageActions>
            <Button variant="outline" onClick={() => sync.mutate()} disabled={syncing}>
              <RefreshCw className={cn(syncing && "animate-spin")} />
              {syncing ? "同步中" : "立即同步"}
            </Button>
          </PageActions>
        }
      />
      {error && <QueryErrorToast error={error} fallback="运营数据读取失败" />}
      {events.error && <QueryErrorToast error={events.error} fallback="最近事件读取失败" />}
      <section
        className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4"
        aria-label="核心运营指标"
      >
        <MetricCard
          label="受管渠道"
          value={formatNumber(metrics.managedAccounts)}
          detail={accountDetail}
          icon={<ServerCog size={20} />}
          tone="teal"
          loading={loading}
        />
        <MetricCard
          label="平均健康分"
          value={metrics.averageHealthScore}
          detail={healthDetail}
          icon={<ShieldCheck size={20} />}
          tone="red"
          loading={loading}
        />
        <MetricCard
          label="已分配并发"
          value={formatNumber(metrics.assignedConcurrency)}
          detail={`${metrics.accountsWithConcurrency} 个渠道已设置并发上限`}
          icon={<Bolt size={20} />}
          tone="teal"
          loading={loading}
        />
        <MetricCard
          label="风险分组"
          value={metrics.riskGroups}
          detail={`${metrics.criticalGroups} 个分组仅剩保底或无可用账号`}
          icon={<TriangleAlert size={20} />}
          tone="red"
          loading={loading}
        />
      </section>

      <Card className="mt-3 gap-0">
        <CardHeader>
          <CardTitle>分组健康矩阵</CardTitle>
          <CardAction>
            <Button variant="outline" onClick={props.onOpenGroups}>
              分组管理 <ArrowRight />
            </Button>
          </CardAction>
        </CardHeader>

        <CardContent
          data-testid="group-health-grid"
          className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3"
        >
          {loading && <MatrixSkeleton />}
          {!loading && !error && healthRows.length === 0 && (
            <div className="text-muted-foreground col-span-full flex min-h-48 items-center justify-center text-sm">
              暂无分组数据
            </div>
          )}
          {!loading &&
            healthRows.map((health) => (
              <GroupHealthCard
                key={health.group.id ?? health.group.name}
                health={health}
                onOpen={props.onOpenGroups}
              />
            ))}
        </CardContent>
      </Card>

      <OverviewActivity
        attention={attention}
        events={events.data ?? []}
        attentionLoading={loading}
        attentionError={error}
        eventsLoading={events.isLoading}
        eventsError={events.error}
        onOpenAccounts={props.onOpenAccounts}
        onOpenEvents={props.onOpenEvents}
      />
    </PageLayout>
  );
}
