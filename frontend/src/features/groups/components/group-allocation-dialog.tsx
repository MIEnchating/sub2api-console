import type { GroupAllocation, GroupAllocationChannel, GroupStatus } from "@/api";
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
import {
  schedulingStrategyDescription,
  schedulingStrategyLabel,
  schedulingWeightFormula,
} from "@/lib/scheduling-strategy";

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
  content: "flex min-h-0 flex-col gap-4 overflow-hidden",
  loading: "grid h-full min-h-0 grid-rows-[4rem_minmax(0,1fr)] gap-4",
  metrics: "grid grid-cols-2 divide-x divide-y rounded-lg border sm:grid-cols-3 xl:grid-cols-6",
  table: "min-w-[1060px] table-fixed",
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
  const weightSummary = props.allocation.has_allocation
    ? `最终权重合计 ${schedulingMetric(props.allocation.total_weight)} · 分组预算 ${integer(props.allocation.weight_budget)}`
    : `分组权重预算 ${integer(props.allocation.weight_budget)}`;
  const metadata = [
    `#${props.allocation.group_id}`,
    props.allocation.platform ?? "平台未记录",
    `分组计费倍率 ${props.allocation.rate_multiplier ?? "—"}`,
    `渠道 ${integer(props.allocation.account_count)}`,
    weightSummary,
  ];
  return (
    <div className="grid min-w-0 gap-1.5">
      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
        <strong className="min-w-0 truncate text-lg font-semibold">
          {props.allocation.group_name}
        </strong>
        <StatusBadge label={status.label} variant={status.tone} />
        <StatusBadge label={interval(props.allocation.probe_interval_seconds)} variant="info" />
      </div>
      <p className="text-muted-foreground min-w-0 text-sm leading-5">{metadata.join(" · ")}</p>
      <p className="text-muted-foreground min-w-0 text-xs leading-5">
        {schedulingStrategyLabel(props.allocation.strategy)}：
        {schedulingStrategyDescription(props.allocation.strategy)}；{schedulingWeightFormula}
      </p>
    </div>
  );
}

function stateVariant(state: string): StatusVariant {
  if (["healthy", "active", "available"].includes(state)) return "success";
  if (["degraded", "survivor", "cost_blocked"].includes(state)) return "warning";
  if (["fused", "disabled"].includes(state)) return "danger";
  return "neutral";
}

function SummaryMetric(props: { label: string; value: string }) {
  return (
    <div className="grid min-w-0 gap-1 px-3 py-2.5">
      <span className="text-muted-foreground truncate text-xs">{props.label}</span>
      <strong className="truncate text-base font-semibold tabular-nums">{props.value}</strong>
    </div>
  );
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
          label="健康 / 可用"
          value={`${integer(props.allocation.healthy_accounts)} / ${integer(props.allocation.available_accounts)}`}
        />
        <SummaryMetric
          label="最高分"
          value={schedulingMetric(props.allocation.highest_health_score)}
        />
        <SummaryMetric
          label="平均分"
          value={schedulingMetric(props.allocation.average_health_score)}
        />
        <SummaryMetric
          label="熔断 / 暂停 / 不可用"
          value={`${integer(props.allocation.fused_accounts)} / ${integer(props.allocation.paused_accounts)} / ${integer(props.allocation.unavailable_accounts)}`}
        />
        <SummaryMetric
          label="限流 / 待探测"
          value={`${integer(props.allocation.rate_limited_accounts)} / ${integer(props.allocation.pending_accounts)}`}
        />
        <SummaryMetric label="分配并发" value={integer(props.allocation.assigned_concurrency)} />
      </div>
      {props.allocation.channels.length ? (
        <div className="min-h-0 flex-1 overflow-hidden rounded-lg border">
          <Table
            className={groupAllocationLayout.table}
            containerClassName="h-full min-h-0 overflow-auto"
          >
            <TableHeader>
              <TableRow>
                <TableHead className="w-[8%]">健康分</TableHead>
                <TableHead className="w-[24%]">账号</TableHead>
                <TableHead className="w-[10%]">状态</TableHead>
                <TableHead className="w-[10%]">综合 P95</TableHead>
                <TableHead className="w-[9%]">调度倍率</TableHead>
                <TableHead className="w-[9%]">优先级</TableHead>
                <TableHead className="w-[20%]">最终权重</TableHead>
                <TableHead className="w-[10%] text-right">分配并发</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {props.allocation.channels.map((channel) => (
                <TableRow key={channel.account_id}>
                  <TableCell className="font-semibold">
                    {schedulingMetric(channel.health_score)}
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
        </div>
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
          <DialogTitle>分组账号调度状态</DialogTitle>
          <DialogDescription>
            {props.group
              ? `${props.group.name} · 共 ${props.group.account_count} 个账号`
              : "查看分组内账号的唯一最终调度状态"}
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
