import type { GroupAllocation, GroupAllocationChannel, GroupStatus } from "@/api";
import { AccountHealthScore } from "@/components/account-health-score";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { StatusBadge, type StatusVariant } from "@/components/status-badge";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { QueryErrorToast } from "@/components/query-error-toast";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { groupStatusMeta } from "@/lib/group-policy-display";
import { schedulingMetric } from "@/lib/scheduling-display";
import { schedulingStrategyLabel } from "@/lib/scheduling-strategy";
import { cn } from "@/lib/utils";

type Props = {
  group: GroupStatus | null;
  allocation?: GroupAllocation;
  loading: boolean;
  error: unknown;
  onClose: () => void;
};

export const groupAllocationLayout = {
  dialog: "grid min-w-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden",
  width: "table",
  height: "tall",
  content: "flex h-full min-h-0 flex-col gap-3 overflow-hidden",
  loading: "grid h-full min-h-0 grid-rows-[4rem_minmax(0,1fr)] gap-4",
  policy:
    "bg-muted/40 grid min-w-0 grid-cols-1 divide-y overflow-hidden rounded-lg border sm:grid-cols-3 sm:divide-x sm:divide-y-0",
  policyItem: "grid min-w-0 content-center gap-1.5 px-3 py-2.5",
  metrics: "bg-border grid grid-cols-2 gap-px overflow-hidden rounded-lg border lg:grid-cols-4",
  metric: "bg-popover grid min-w-0 content-start gap-1 px-3 py-3",
  table: "min-w-[1060px] table-fixed",
  tableContainer: "h-full min-h-0 overflow-auto overscroll-contain",
} as const;

const stateLabels: Record<string, string> = {
  healthy: "健康",
  active: "健康",
  available: "可用",
  degraded: "降级",
  survivor: "保底",
  fused: "熔断",
  cost_blocked: "成本墙拦截",
  paused: "暂停",
  disabled: "不可用",
  excluded: "已排除",
  unknown: "待探测",
};

function integer(value: number): string {
  return Number.isFinite(value) ? Math.max(0, Math.trunc(value)).toLocaleString("zh-CN") : "—";
}

function latency(value: number | null): string {
  if (value === null || !Number.isFinite(value)) return "—";
  if (value >= 1_000) return `${(value / 1_000).toFixed(value >= 10_000 ? 1 : 2)}s`;
  return `${Math.round(value)}ms`;
}

function interval(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return "探测周期未记录";
  if (value % 3600 === 0) return `每 ${value / 3600} 小时测试`;
  if (value % 60 === 0) return `每 ${value / 60} 分钟测试`;
  return `每 ${Math.trunc(value)} 秒测试`;
}

function GroupHeader(props: { allocation: GroupAllocation }) {
  const status = groupStatusMeta(props.allocation.status);
  return (
    <div className={groupAllocationLayout.policy} data-slot="inspection-policy-summary">
      <div className={groupAllocationLayout.policyItem}>
        <span className="text-muted-foreground text-xs">当前状态</span>
        <StatusBadge label={status.label} variant={status.tone} size="lg" />
      </div>
      <div className={groupAllocationLayout.policyItem}>
        <span className="text-muted-foreground text-xs">巡检策略</span>
        <strong className="text-base font-semibold">
          {schedulingStrategyLabel(props.allocation.strategy)}策略
        </strong>
      </div>
      <div className={groupAllocationLayout.policyItem}>
        <span className="text-muted-foreground text-xs">测试周期</span>
        <strong className="text-base font-semibold">
          {interval(props.allocation.probe_interval_seconds)}
        </strong>
      </div>
    </div>
  );
}

function stateVariant(state: string): StatusVariant {
  if (["healthy", "active", "available"].includes(state)) return "success";
  if (["degraded", "survivor", "cost_blocked"].includes(state)) return "warning";
  if (["fused", "disabled"].includes(state)) return "danger";
  return "neutral";
}

const metricToneClasses: Record<StatusVariant, string> = {
  success: "text-success",
  warning: "text-warning",
  danger: "text-destructive",
  info: "text-info",
  purple: "text-purple-600 dark:text-purple-400",
  neutral: "text-foreground",
};

function SummaryMetric(props: {
  label: string;
  value: string;
  detail: string;
  tone?: StatusVariant;
}) {
  return (
    <div className={groupAllocationLayout.metric}>
      <span className="text-muted-foreground truncate text-xs">{props.label}</span>
      <strong
        className={cn(
          "truncate text-lg font-semibold tabular-nums",
          metricToneClasses[props.tone ?? "neutral"],
        )}
      >
        {props.value}
      </strong>
      <span className="text-muted-foreground min-h-4 text-xs leading-4">{props.detail}</span>
    </div>
  );
}

function availabilityTone(allocation: GroupAllocation): StatusVariant {
  if (allocation.account_count > 0 && allocation.available_accounts === allocation.account_count) {
    return "success";
  }
  if (allocation.available_accounts === 0) return "danger";
  return "warning";
}

function attentionSummary(allocation: GroupAllocation): {
  statusCount: number;
  detail: string;
  tone: StatusVariant;
} {
  const items = [
    { label: "熔断", count: allocation.fused_accounts },
    { label: "暂停", count: allocation.paused_accounts },
    { label: "不可用", count: allocation.unavailable_accounts },
    { label: "限流", count: allocation.rate_limited_accounts },
    { label: "待探测", count: allocation.pending_accounts },
  ].filter((item) => item.count > 0);
  if (items.length === 0) return { statusCount: 0, detail: "当前无异常", tone: "success" };
  const hasUnavailable = allocation.fused_accounts > 0 || allocation.unavailable_accounts > 0;
  return {
    statusCount: items.length,
    detail: items.map((item) => `${item.label} ${integer(item.count)}`).join(" · "),
    tone: hasUnavailable ? "danger" : "warning",
  };
}

function WeightCell(props: { channel: GroupAllocationChannel; maximum: number }) {
  const weight = props.channel.weight;
  const share =
    weight === null || props.maximum <= 0 ? 0 : Math.min(100, (weight / props.maximum) * 100);
  return (
    <div className="grid min-w-0 gap-1.5">
      <div className="bg-muted h-1.5 overflow-hidden rounded-full" aria-hidden="true">
        <span className="bg-primary block h-full" style={{ width: `${share}%` }} />
      </div>
      <span className="text-muted-foreground text-right text-xs tabular-nums">
        {weight === null ? "未生成" : `最终权重 ${schedulingMetric(weight)}`}
      </span>
    </div>
  );
}

export function GroupAllocationContent(props: { allocation: GroupAllocation }) {
  const maximumWeight = Math.max(
    0,
    ...props.allocation.channels.map((channel) => channel.weight ?? 0),
  );
  const attention = attentionSummary(props.allocation);
  const weightValue = props.allocation.has_allocation
    ? `${schedulingMetric(props.allocation.total_weight)} / ${integer(props.allocation.weight_budget)}`
    : "未生成";
  const weightDetail = props.allocation.has_allocation
    ? "已生成最终权重"
    : `预算 ${integer(props.allocation.weight_budget)}`;
  return (
    <div className={groupAllocationLayout.content}>
      <GroupHeader allocation={props.allocation} />
      {!props.allocation.has_allocation && props.allocation.channels.length > 0 && (
        <div className="border-info/30 bg-info/10 text-info rounded-lg border px-3 py-2 text-sm">
          尚未生成账号最终调度状态。监控模式不会保存调度结果；切换至完全模式后，下一轮巡检会按分组权重预算{" "}
          {integer(props.allocation.weight_budget)} 计算。
        </div>
      )}
      <div className={groupAllocationLayout.metrics}>
        <SummaryMetric
          label="可用账号"
          value={`${integer(props.allocation.available_accounts)} / ${integer(props.allocation.account_count)}`}
          detail={`其中健康 ${integer(props.allocation.healthy_accounts)}`}
          tone={availabilityTone(props.allocation)}
        />
        <SummaryMetric
          label="需关注"
          value={`${integer(attention.statusCount)} 项`}
          detail={attention.detail}
          tone={attention.tone}
        />
        <SummaryMetric
          label="权重分配"
          value={weightValue}
          detail={weightDetail}
          tone={props.allocation.has_allocation ? "info" : "neutral"}
        />
        <SummaryMetric
          label="分配并发"
          value={integer(props.allocation.assigned_concurrency)}
          detail="当前分配总计"
        />
      </div>
      {props.allocation.channels.length ? (
        <DataTablePanel className="flex-1">
          <Table
            className={groupAllocationLayout.table}
            containerClassName={groupAllocationLayout.tableContainer}
          >
            <TableHeader>
              <TableRow>
                <TableHead className="w-[13%]">健康分</TableHead>
                <TableHead className="w-[24%]">账号</TableHead>
                <TableHead className="w-[10%]">状态</TableHead>
                <TableHead className="w-[10%]">综合 P95</TableHead>
                <TableHead className="w-[9%]">账号成本</TableHead>
                <TableHead className="w-[9%]">优先级</TableHead>
                <TableHead className="w-[15%]">最终权重</TableHead>
                <TableHead className="w-[10%] text-right">分配并发</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {props.allocation.channels.map((channel) => (
                <TableRow key={channel.account_id}>
                  <TableCell>
                    <AccountHealthScore
                      score={channel.health_score}
                      shortScore={channel.short_score}
                      longScore={channel.long_score}
                      sampleCount={channel.sample_count}
                    />
                  </TableCell>
                  <TableCell>
                    <div className="grid min-w-0 gap-0.5">
                      <span className="truncate font-medium">{channel.account_name}</span>
                      <span className="text-muted-foreground truncate font-mono text-xs">
                        ID {channel.account_id}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      label={stateLabels[channel.health] ?? channel.health}
                      variant={stateVariant(channel.health)}
                    />
                  </TableCell>
                  <TableCell>{latency(channel.ttfb_p95_ms)}</TableCell>
                  <TableCell>{channel.rate ?? "—"}</TableCell>
                  <TableCell>{channel.priority ?? "—"}</TableCell>
                  <TableCell overflowTooltip={false}>
                    <WeightCell channel={channel} maximum={maximumWeight} />
                  </TableCell>
                  <TableCell className="text-right font-medium">
                    {channel.assigned_concurrency === null
                      ? "—"
                      : integer(channel.assigned_concurrency)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DataTablePanel>
      ) : (
        <div className="text-muted-foreground flex min-h-32 items-center justify-center rounded-lg border border-dashed px-4 text-sm">
          该分组暂无账号，尚未产生账号调度状态。
        </div>
      )}
    </div>
  );
}

export function GroupAllocationDialog(props: Props) {
  return (
    <Dialog open={props.group !== null} onOpenChange={(open) => !open && props.onClose()}>
      <DialogContent
        width={groupAllocationLayout.width}
        height={groupAllocationLayout.height}
        className={groupAllocationLayout.dialog}
      >
        <DialogHeader>
          <DialogTitle>
            {props.group ? `${props.group.name} 调度状态` : "分组账号调度状态"}
          </DialogTitle>
          <DialogDescription>
            {props.group
              ? `${props.group.account_count} 个账号 · 权重与并发分配`
              : "账号权重与并发分配"}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="overflow-hidden pr-0">
          {props.loading ? (
            <div className={groupAllocationLayout.loading} aria-label="正在读取分组账号调度状态">
              <Skeleton className="h-full w-full" />
              <Skeleton className="h-full min-h-0 w-full" />
            </div>
          ) : props.error ? (
            <QueryErrorToast error={props.error} fallback="分组账号调度状态读取失败" />
          ) : props.allocation ? (
            <GroupAllocationContent allocation={props.allocation} />
          ) : null}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
