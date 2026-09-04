import { useEffect, useMemo, useState } from "react";
import { CircleAlert, Eye, LoaderCircle } from "lucide-react";

import type { NotificationQueueDetails, NotificationQueueItem, NotificationStatus } from "@/api";
import { DataTablePagination } from "@/components/data-table/pagination";
import { SearchField } from "@/components/data-table/search-field";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { DataTablePanel } from "@/components/data-table/table-panel";
import { RefreshButton } from "@/components/refresh-button";
import { StatusBadge } from "@/components/status-badge";
import type { StatusVariant } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  operationDialogHeight,
  operationDialogWidth,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  alertCauseLabel,
  alertObjectLabel,
  alertStatusLabel,
  alertSubjectLabel,
  alertTypeLabel,
} from "@/features/alerts/lib/alert-display";
import { useClientPagination } from "@/hooks/use-client-pagination";

type QueueMetricProps = {
  label: string;
  value: number;
  danger?: boolean;
};

type QueueDisplayItem = {
  alert: NotificationQueueItem;
  queueStatus: string;
  queueReason?: string;
};

type NotificationPresentation = {
  label: string;
  detail?: string;
  variant: StatusVariant;
};

function QueueMetric(props: QueueMetricProps) {
  return (
    <div className="flex items-baseline gap-2 whitespace-nowrap">
      <span className="text-muted-foreground text-xs">{props.label}</span>
      <strong
        className={
          props.danger
            ? "text-destructive text-base font-semibold tabular-nums"
            : "text-base font-semibold tabular-nums"
        }
      >
        {props.value}
      </strong>
    </div>
  );
}

function queueItems(details: NotificationQueueDetails): QueueDisplayItem[] {
  return details.consumer_items.map((alert) => ({
    alert,
    queueStatus: alert.queue_status || "状态未知",
    queueReason: alert.queue_reason,
  }));
}

function notificationPresentation(item: QueueDisplayItem): NotificationPresentation {
  const detail = item.queueReason?.trim() || item.alert.last_error?.trim() || undefined;
  if (item.queueStatus === "发送失败，等待重试") {
    return { label: "发送失败，等待重试", detail, variant: "danger" };
  }
  if (item.queueStatus === "待发送") {
    return { label: "等待发送", detail, variant: "info" };
  }
  if (item.queueStatus === "状态观察中") {
    return { label: "观察中", detail, variant: "warning" };
  }
  if (item.queueStatus === "状态变化冷却中") {
    return { label: "等待冷却", detail, variant: "warning" };
  }
  if (item.queueStatus === "降级告警汇总冷却中") {
    return { label: "等待合并发送", detail, variant: "warning" };
  }
  if (item.queueStatus === "已抑制") {
    if (detail === "恢复通知开关已关闭") {
      return { label: "无需发送", detail: "恢复通知已关闭", variant: "neutral" };
    }
    if (detail === "告警通知开关已关闭" || detail === "通知规则开关已关闭") {
      return { label: "通知已关闭", detail, variant: "neutral" };
    }
    return { label: "暂不发送", detail, variant: "warning" };
  }
  if (item.queueStatus === "本轮不发送") {
    return { label: "本轮无需发送", detail, variant: "neutral" };
  }
  if (item.queueStatus === "无需发送") {
    return { label: "无需发送", detail, variant: "neutral" };
  }
  return { label: item.queueStatus, detail, variant: "neutral" };
}

function alertStatusVariant(status: string): StatusVariant {
  if (status === "firing") return "danger";
  if (status === "recovered") return "success";
  if (status === "suppressed") return "warning";
  return "neutral";
}

function formatQueueTime(value: string | null | undefined) {
  if (!value) return "未知时间";
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? value
    : date.toLocaleString("zh-CN", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
      });
}

function queueItemMatches(item: QueueDisplayItem, search: string) {
  const query = search.trim().toLocaleLowerCase();
  if (!query) return true;
  const alert = item.alert;
  const notification = notificationPresentation(item);
  return [
    alertSubjectLabel(alert.event_type),
    alertTypeLabel(alert.event_type, alert.status),
    alertObjectLabel(alert),
    alertCauseLabel(alert.cause_code),
    notification.label,
    notification.detail,
    alert.last_error,
    alert.incident_key,
  ].some((value) => value?.toLocaleLowerCase().includes(query));
}

function QueueDetailsTable(props: { items: QueueDisplayItem[] }) {
  if (!props.items.length) {
    return (
      <div className="text-muted-foreground flex min-h-40 items-center justify-center px-4 text-sm">
        没有匹配的队列内容
      </div>
    );
  }
  return (
    <>
      <div className="divide-border divide-y sm:hidden">
        {props.items.map((item) => {
          const alert = item.alert;
          const notification = notificationPresentation(item);
          return (
            <article className="space-y-3 p-3" key={alert.incident_key}>
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <strong className="block font-medium">
                    {alertSubjectLabel(alert.event_type)}
                  </strong>
                  <span className="text-muted-foreground mt-0.5 block text-xs">
                    {alertObjectLabel(alert)}
                  </span>
                </div>
                <StatusBadge
                  label={alertStatusLabel(alert.status)}
                  variant={alertStatusVariant(alert.status)}
                />
              </div>
              <p className="text-sm">
                <span className="text-muted-foreground">触发原因：</span>
                {alertCauseLabel(alert.cause_code)}
              </p>
              <div className="bg-muted/40 rounded-md px-2.5 py-2 text-xs">
                <StatusBadge label={notification.label} variant={notification.variant} />
                {notification.detail ? <p className="mt-1">{notification.detail}</p> : null}
                {item.queueStatus === "发送失败，等待重试" && alert.delivery_attempts > 0 ? (
                  <p className="text-muted-foreground mt-1">已尝试 {alert.delivery_attempts} 次</p>
                ) : null}
                {alert.delivered_at ? (
                  <p className="text-muted-foreground mt-1">
                    上次通知 {formatQueueTime(alert.delivered_at)}
                  </p>
                ) : null}
              </div>
              <div className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs">
                <span>首次发现 {formatQueueTime(alert.first_seen_at)}</span>
                <span>
                  {alert.status === "recovered" ? "恢复时间" : "最近更新"}{" "}
                  {formatQueueTime(alert.last_seen_at)}
                </span>
              </div>
            </article>
          );
        })}
      </div>
      <div className="hidden sm:block">
        <Table className="min-w-[840px]" containerClassName="overflow-x-auto">
          <TableHeader>
            <TableRow>
              <TableHead className="w-[38%]">告警内容</TableHead>
              <TableHead className="w-28">当前状态</TableHead>
              <TableHead className="w-[28%]">通知状态</TableHead>
              <TableHead className="w-40">时间</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.items.map((item) => {
              const alert = item.alert;
              const notification = notificationPresentation(item);
              return (
                <TableRow key={alert.incident_key}>
                  <TableCell className="whitespace-normal" overflowTooltip={false}>
                    <strong className="block font-medium">
                      {alertSubjectLabel(alert.event_type)}
                    </strong>
                    <span className="text-muted-foreground mt-0.5 block text-xs">
                      {alertObjectLabel(alert)}
                    </span>
                    <span className="mt-1.5 block text-xs">
                      <span className="text-muted-foreground">触发原因：</span>
                      {alertCauseLabel(alert.cause_code)}
                    </span>
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      label={alertStatusLabel(alert.status)}
                      variant={alertStatusVariant(alert.status)}
                    />
                  </TableCell>
                  <TableCell className="whitespace-normal" overflowTooltip={false}>
                    <StatusBadge label={notification.label} variant={notification.variant} />
                    {notification.detail ? (
                      <span className="mt-1 block text-xs">{notification.detail}</span>
                    ) : null}
                    {item.queueStatus === "发送失败，等待重试" && alert.delivery_attempts > 0 ? (
                      <span className="text-muted-foreground mt-1 block text-xs">
                        已尝试 {alert.delivery_attempts} 次
                      </span>
                    ) : null}
                    {alert.delivered_at ? (
                      <span className="text-muted-foreground mt-1 block text-xs">
                        上次通知 {formatQueueTime(alert.delivered_at)}
                      </span>
                    ) : null}
                  </TableCell>
                  <TableCell className="whitespace-normal" overflowTooltip={false}>
                    <span className="block text-xs">
                      <span className="text-muted-foreground">首次发现：</span>
                      {formatQueueTime(alert.first_seen_at)}
                    </span>
                    <span className="mt-1 block text-xs">
                      <span className="text-muted-foreground">
                        {alert.status === "recovered" ? "恢复时间：" : "最近更新："}
                      </span>
                      {formatQueueTime(alert.last_seen_at)}
                    </span>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </div>
    </>
  );
}

export function NotificationQueueDetailsList(props: { details: NotificationQueueDetails }) {
  const [search, setSearch] = useState("");
  const filteredItems = useMemo(
    () => queueItems(props.details).filter((item) => queueItemMatches(item, search)),
    [props.details, search],
  );
  const pagination = useClientPagination(filteredItems);

  useEffect(() => {
    pagination.setCurrentPage(1);
  }, [pagination.setCurrentPage, props.details]);

  return (
    <div className="flex h-full min-h-0 flex-col gap-3" data-testid="notification-queue-details">
      <TableFilterToolbar>
        <SearchField
          value={search}
          onChange={(value) => {
            setSearch(value);
            pagination.setCurrentPage(1);
          }}
          placeholder="搜索告警、对象、状态或原因"
        />
      </TableFilterToolbar>
      <DataTablePanel className="flex-1">
        <div className="min-h-0 flex-1 overflow-y-auto">
          <QueueDetailsTable items={pagination.visibleItems} />
        </div>
        {filteredItems.length > 0 ? (
          <DataTablePagination
            currentPage={pagination.currentPage}
            totalPages={pagination.totalPages}
            totalItems={filteredItems.length}
            pageSize={pagination.pageSize}
            pageSizes={[10, 20, 50, 100]}
            onPageChange={pagination.setCurrentPage}
            onPageSizeChange={pagination.setPageSize}
          />
        ) : null}
      </DataTablePanel>
    </div>
  );
}

export function NotificationQueueStatus(props: {
  queues: NotificationStatus["queues"];
  loadDetails: () => Promise<NotificationQueueDetails>;
}) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [details, setDetails] = useState<NotificationQueueDetails | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadQueue = async () => {
    setDialogOpen(true);
    setDetails(null);
    setError(null);
    setLoading(true);
    try {
      setDetails(await props.loadDetails());
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : "队列内容读取失败");
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <Card
        size="sm"
        className="shrink-0 overflow-hidden"
        data-testid="notification-queue-overview"
      >
        <CardHeader className="border-b px-3 py-2.5">
          <CardTitle>告警队列</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <section className="grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-3 py-3">
            <div className="flex min-w-0 flex-wrap items-center gap-x-5 gap-y-1">
              <QueueMetric label="告警中" value={props.queues.producer_firing} />
              <QueueMetric label="已恢复" value={props.queues.producer_recovered} />
              <QueueMetric label="待发送" value={props.queues.consumer_pending} />
              <QueueMetric
                label="发送失败"
                value={props.queues.consumer_failed}
                danger={props.queues.consumer_failed > 0}
              />
              {props.queues.consumer_failed > 0 ? (
                <CircleAlert className="text-destructive sr-only" aria-label="存在发送失败" />
              ) : null}
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="justify-self-end"
              onClick={() => void loadQueue()}
            >
              <Eye aria-hidden="true" />
              查看队列
            </Button>
          </section>
        </CardContent>
      </Card>

      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          if (!open) setDialogOpen(false);
        }}
      >
        <DialogContent
          width={operationDialogWidth(Boolean(details && !loading))}
          height={operationDialogHeight(Boolean(details && !loading))}
          className="grid grid-rows-[auto_minmax(0,1fr)] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle>队列明细</DialogTitle>
            <DialogDescription>
              告警中 {details?.producer_firing.length ?? props.queues.producer_firing} 条，已恢复{" "}
              {details?.producer_recovered.length ?? props.queues.producer_recovered} 条，待发送{" "}
              {details?.consumer_pending.length ?? props.queues.consumer_pending} 条，发送失败{" "}
              {details?.consumer_failed.length ?? props.queues.consumer_failed} 条
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="overflow-hidden pr-0">
            {loading ? (
              <DataTablePanel className="h-full overflow-auto">
                <div className="text-muted-foreground flex min-h-40 items-center justify-center gap-2 text-sm">
                  <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
                  正在读取队列内容
                </div>
              </DataTablePanel>
            ) : error ? (
              <DataTablePanel className="h-full overflow-auto">
                <div className="flex min-h-40 flex-col items-center justify-center gap-3 px-4 text-center text-sm">
                  <span className="text-destructive">{error}</span>
                  <RefreshButton ariaLabel="刷新队列内容" onClick={() => void loadQueue()} />
                </div>
              </DataTablePanel>
            ) : details ? (
              <NotificationQueueDetailsList details={details} />
            ) : null}
          </DialogBody>
        </DialogContent>
      </Dialog>
    </>
  );
}
