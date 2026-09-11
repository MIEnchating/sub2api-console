import { ContentRetry } from "@/components/content-retry";
import { ContentLoading } from "@/components/content-loading";
import { useQuery } from "@tanstack/react-query";
import { Cpu, Eye, HardDrive, MemoryStick, type LucideIcon } from "lucide-react";
import { useMemo, useState } from "react";

import { api, type TaskSummary } from "@/api";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { PageActions } from "@/components/page-actions";
import { PageHeading } from "@/components/page-heading";
import { PageLayout } from "@/components/page-layout";
import { QueryErrorToast } from "@/components/query-error-toast";
import { RefreshButton } from "@/components/refresh-button";
import { StatusBadge } from "@/components/status-badge";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  PlatformProbeResultTable,
  platformProbeResults,
} from "@/features/accounts/components/platform-probe-dialog";

import {
  activeTaskStatuses,
  taskOperationLabel,
  taskStatusLabel,
  taskStatusVariant,
} from "../constants";

type TaskListGroup = "active" | "history";

function formatTaskDate(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("zh-CN", { hour12: false });
}

function probeScopeLabel(platform: string, model: string): string {
  let displayPlatform = platform;
  if (platform.toLocaleLowerCase() === "openai") displayPlatform = "OpenAI";
  return [displayPlatform, model].filter(Boolean).join(" · ");
}

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
}) {
  const Icon = props.icon;
  return (
    <Card size="sm">
      <CardContent className="grid gap-2.5">
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2 font-medium">
            <Icon className="text-muted-foreground size-4 shrink-0" aria-hidden="true" />
            <span>{props.label}</span>
          </div>
          <span className="text-base font-semibold tabular-nums">
            {formatPercent(props.usagePercent)}
          </span>
        </div>
        <Progress
          value={props.usagePercent}
          aria-label={`${props.label} ${formatPercent(props.usagePercent)}`}
        />
        <p className="text-muted-foreground text-xs tabular-nums">{props.detail}</p>
      </CardContent>
    </Card>
  );
}

function TaskDetailsDialog(props: {
  taskId: string | null;
  accountNames: ReadonlyMap<string, string>;
  onClose: () => void;
}) {
  const detail = useQuery({
    queryKey: ["task", props.taskId],
    queryFn: () => api.task(props.taskId!),
    enabled: props.taskId !== null,
    refetchInterval: (query) =>
      query.state.data && activeTaskStatuses.has(query.state.data.status) ? 1_000 : false,
  });
  const task = detail.data;
  const probeResults = platformProbeResults(task, props.accountNames);
  const platform = typeof task?.result.platform === "string" ? task.result.platform : "";
  const model = typeof task?.result.model === "string" ? task.result.model : "";

  return (
    <Dialog open={props.taskId !== null} onOpenChange={(open) => !open && props.onClose()}>
      <DialogContent width={probeResults.length > 0 ? "table" : "wide"} height="adaptive">
        <DialogHeader>
          <DialogTitle>任务详情</DialogTitle>
          <DialogDescription>
            {task ? `${taskOperationLabel(task.operation)} · ${task.id}` : "查看任务状态与执行结果"}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="grid gap-4">
          {detail.isLoading ? <ContentLoading label="正在读取任务详情" /> : null}
          {detail.error ? (
            <>
              <QueryErrorToast error={detail.error} fallback="任务详情读取失败，请稍后重试" />
              {!task && (
                <ContentRetry onRetry={() => void detail.refetch()} pending={detail.isFetching} />
              )}
            </>
          ) : null}
          {task ? (
            <>
              <div className="grid gap-2 rounded-md border p-3 text-sm sm:grid-cols-2">
                <div>
                  <span className="text-muted-foreground">状态：</span>
                  <StatusBadge
                    label={taskStatusLabel(task.status)}
                    variant={taskStatusVariant(task.status)}
                  />
                </div>
                <div>
                  <span className="text-muted-foreground">进度：</span>
                  {task.progress}%
                </div>
                <div className="sm:col-span-2">{task.message}</div>
                <Progress
                  className="sm:col-span-2"
                  value={task.progress}
                  aria-label="任务详情进度"
                />
              </div>
              {platform || model ? (
                <p className="font-medium">{probeScopeLabel(platform, model)}</p>
              ) : null}
              {probeResults.length > 0 ? (
                <PlatformProbeResultTable results={probeResults} />
              ) : (
                <p className="text-muted-foreground text-sm">此任务没有可展示的账号探活明细。</p>
              )}
            </>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={props.onClose}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function SystemInfoPage() {
  const [group, setGroup] = useState<TaskListGroup>("active");
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
  const tasks = useQuery({
    queryKey: ["tasks"],
    queryFn: () => api.tasks(20),
    refetchInterval: (query) =>
      query.state.data?.some((task) => activeTaskStatuses.has(task.status)) ? 1_000 : 10_000,
  });
  const metrics = useQuery({
    queryKey: ["system-metrics"],
    queryFn: api.systemMetrics,
    refetchInterval: 5_000,
  });
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: api.accounts });
  const accountNames = useMemo(
    () => new Map((accounts.data ?? []).map((account) => [account.id, account.name])),
    [accounts.data],
  );
  const groupedTasks = useMemo(() => {
    const active: TaskSummary[] = [];
    const history: TaskSummary[] = [];
    for (const task of tasks.data ?? []) {
      if (task.system_info !== true) continue;
      if (active.length + history.length >= 20) break;
      if (activeTaskStatuses.has(task.status)) active.push(task);
      else history.push(task);
    }
    return { active, history };
  }, [tasks.data]);
  const visibleTasks = groupedTasks[group];

  return (
    <PageLayout fixedContent>
      {tasks.error ? <QueryErrorToast error={tasks.error} fallback="任务列表读取失败" /> : null}
      {metrics.error ? (
        <QueryErrorToast error={metrics.error} fallback="系统资源统计读取失败" />
      ) : null}
      <PageHeading
        eyebrow="SYSTEM / TASKS"
        title="系统信息"
        description="查看后台进行中的任务与历史执行结果。"
        action={
          <PageActions>
            <RefreshButton
              pending={tasks.isFetching || metrics.isFetching}
              ariaLabel="刷新系统信息"
              onClick={() => void Promise.all([tasks.refetch(), metrics.refetch()])}
            />
          </PageActions>
        }
      />
      <div className="flex h-full min-h-0 flex-col gap-3">
        <div className="grid shrink-0 gap-3 sm:grid-cols-3" aria-label="服务器资源占用">
          {metrics.data ? (
            <>
              <ResourceMetric
                label="CPU 占用"
                icon={Cpu}
                usagePercent={metrics.data.cpu.usage_percent}
                detail={`${metrics.data.cpu.logical_cores} 个逻辑核心`}
              />
              <ResourceMetric
                label="内存占用"
                icon={MemoryStick}
                usagePercent={metrics.data.memory.usage_percent}
                detail={`${formatBytes(metrics.data.memory.used_bytes)} / ${formatBytes(metrics.data.memory.total_bytes)}`}
              />
              <ResourceMetric
                label="硬盘占用"
                icon={HardDrive}
                usagePercent={metrics.data.disk.usage_percent}
                detail={`${formatBytes(metrics.data.disk.used_bytes)} / ${formatBytes(metrics.data.disk.total_bytes)}`}
              />
            </>
          ) : (
            Array.from({ length: 3 }, (_, index) => (
              <Skeleton key={index} className="h-24 w-full rounded-lg" />
            ))
          )}
        </div>
        <SegmentedControl role="tablist" aria-label="任务分类">
          <SegmentedControlItem
            type="button"
            role="tab"
            selected={group === "active"}
            onClick={() => setGroup("active")}
          >
            进行中任务 {groupedTasks.active.length}
          </SegmentedControlItem>
          <SegmentedControlItem
            type="button"
            role="tab"
            selected={group === "history"}
            onClick={() => setGroup("history")}
          >
            历史任务 {groupedTasks.history.length}
          </SegmentedControlItem>
        </SegmentedControl>
        <DataTablePanel className="flex-1">
          <Table containerClassName="h-full min-h-0 overflow-auto" className="min-w-[760px]">
            <TableHeader>
              <TableRow>
                <TableHead className="w-44">任务类型</TableHead>
                <TableHead className="w-24">状态</TableHead>
                <TableHead>进度与消息</TableHead>
                <TableHead className="w-44">更新时间</TableHead>
                <TableHead className="w-16 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tasks.isLoading
                ? Array.from({ length: 5 }, (_, index) => (
                    <TableRow key={index} aria-label="正在加载任务">
                      {Array.from({ length: 5 }, (_, column) => (
                        <TableCell key={column}>
                          <Skeleton className="h-4 w-4/5" />
                        </TableCell>
                      ))}
                    </TableRow>
                  ))
                : null}
              {!tasks.isLoading && visibleTasks.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="h-28 text-center text-muted-foreground">
                    {group === "active" ? "当前没有进行中的任务" : "当前没有历史任务"}
                  </TableCell>
                </TableRow>
              ) : null}
              {visibleTasks.map((task) => (
                <TableRow key={task.id}>
                  <TableCell>
                    <div className="font-medium">{taskOperationLabel(task.operation)}</div>
                    <Tooltip>
                      <TooltipTrigger
                        render={<div className="text-muted-foreground max-w-40 truncate text-xs" />}
                      >
                        {task.id}
                      </TooltipTrigger>
                      <TooltipContent>{task.id}</TooltipContent>
                    </Tooltip>
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      label={taskStatusLabel(task.status)}
                      variant={taskStatusVariant(task.status)}
                    />
                  </TableCell>
                  <TableCell>
                    <div className="mb-1 flex items-center justify-between gap-3 text-sm">
                      <span>{task.message}</span>
                      <span className="text-muted-foreground tabular-nums">{task.progress}%</span>
                    </div>
                    <Progress value={task.progress} aria-label={`${task.id} 任务进度`} />
                  </TableCell>
                  <TableCell className="text-xs">{formatTaskDate(task.updated_at)}</TableCell>
                  <TableCell className="text-right">
                    <TableActionButton label="查看任务" onClick={() => setSelectedTaskId(task.id)}>
                      <Eye aria-hidden="true" />
                    </TableActionButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </DataTablePanel>
      </div>
      <TaskDetailsDialog
        taskId={selectedTaskId}
        accountNames={accountNames}
        onClose={() => setSelectedTaskId(null)}
      />
    </PageLayout>
  );
}
