import { Cpu, HardDrive, MemoryStick, type LucideIcon } from "lucide-react";
import type { ReactElement } from "react";

import type { SystemMetrics } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { Card, CardContent } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";

type SystemResourcesProps = {
  metrics?: SystemMetrics;
  loading: boolean;
  refreshing: boolean;
  failed: boolean;
  onRetry: () => void;
};

const resourceSkeletonKeys = ["cpu", "memory", "disk"];

function formatBytes(value: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let amount = Math.max(0, value);
  let unitIndex = 0;
  while (amount >= 1024 && unitIndex < units.length - 1) {
    amount /= 1024;
    unitIndex += 1;
  }
  const digits = amount >= 10 || Number.isInteger(amount) ? 0 : 1;
  return `${amount.toFixed(digits)} ${units[unitIndex]}`;
}

function formatPercent(value: number): string {
  return `${Number(value.toFixed(1))}%`;
}

function ResourceMetric(props: {
  label: string;
  icon: LucideIcon;
  usagePercent: number;
  detail: string;
}): ReactElement {
  const Icon = props.icon;
  const percent = formatPercent(props.usagePercent);

  return (
    <Card size="sm">
      <CardContent className="grid gap-2 sm:gap-2.5">
        <div className="flex min-w-0 items-center gap-2 text-xs font-medium sm:text-sm">
          <Icon
            className="text-muted-foreground hidden size-4 shrink-0 sm:block"
            aria-hidden="true"
          />
          <span className="min-w-0 wrap-anywhere">{props.label}</span>
        </div>
        <p className="text-2xl leading-8 font-semibold tracking-tight whitespace-nowrap tabular-nums">
          {percent}
        </p>
        <Progress value={props.usagePercent} aria-label={`${props.label} ${percent}`} />
        <p className="text-muted-foreground min-w-0 text-xs leading-5 wrap-anywhere tabular-nums">
          {props.detail}
        </p>
      </CardContent>
    </Card>
  );
}

export function SystemResources(props: SystemResourcesProps): ReactElement {
  return (
    <section
      className="grid min-w-0 shrink-0 grid-cols-3 gap-2 sm:gap-3"
      aria-label="服务器资源占用"
    >
      {props.metrics ? (
        <>
          <ResourceMetric
            label="CPU 占用"
            icon={Cpu}
            usagePercent={props.metrics.cpu.usage_percent}
            detail={`${props.metrics.cpu.logical_cores} 个逻辑核心`}
          />
          <ResourceMetric
            label="内存占用"
            icon={MemoryStick}
            usagePercent={props.metrics.memory.usage_percent}
            detail={`${formatBytes(props.metrics.memory.used_bytes)} / ${formatBytes(props.metrics.memory.total_bytes)}`}
          />
          <ResourceMetric
            label="硬盘占用"
            icon={HardDrive}
            usagePercent={props.metrics.disk.usage_percent}
            detail={`${formatBytes(props.metrics.disk.used_bytes)} / ${formatBytes(props.metrics.disk.total_bytes)}`}
          />
        </>
      ) : null}
      {!props.metrics && props.loading
        ? resourceSkeletonKeys.map((key) => (
            <Card key={key} size="sm" role="status" aria-label="正在读取资源占用" aria-busy="true">
              <CardContent className="grid gap-2 sm:gap-2.5">
                <Skeleton className="h-4 w-20 max-w-full sm:h-5" />
                <Skeleton className="h-8 w-18 max-w-full" />
                <Skeleton className="h-1 w-full" />
                <Skeleton className="h-5 w-full max-w-32" />
              </CardContent>
            </Card>
          ))
        : null}
      {!props.metrics && !props.loading && props.failed ? (
        <div className="col-span-full">
          <ContentRetry pending={props.refreshing} onRetry={props.onRetry} />
        </div>
      ) : null}
    </section>
  );
}
