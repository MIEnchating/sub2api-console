import { useState } from "react";
import { BellRing, CircleAlert, Eye, LoaderCircle, Radio, Send } from "lucide-react";

import type { NotificationQueueDetails, NotificationQueueItem, NotificationStatus } from "@/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { StatusBadge } from "@/components/status-badge";
import {
  alertCauseLabel,
  alertDeliveryLabel,
  alertObjectLabel,
  alertTypeLabel,
} from "@/features/alerts/lib/alert-display";

type QueueKind = "producer" | "consumer";

type QueueMetricProps = {
  label: string;
  value: number;
  danger?: boolean;
};

type QueueDisplayItem = {
  alert: NotificationQueueItem;
  queueStatus: string;
  queueReason?: string;
  danger?: boolean;
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

function queueItems(details: NotificationQueueDetails, kind: QueueKind): QueueDisplayItem[] {
  if (kind === "producer") {
    return [
      ...details.producer_firing.map((alert) => ({ alert, queueStatus: "告警中", danger: true })),
      ...details.producer_recovered.map((alert) => ({ alert, queueStatus: "已恢复" })),
    ];
  }
  return details.consumer_items.map((alert) => ({
    alert,
    queueStatus: alert.queue_status || "状态未知",
    queueReason: alert.queue_reason,
    danger: alert.queue_status === "发送失败，等待重试",
  }));
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

function QueueDetailsTable(props: { items: QueueDisplayItem[]; kind: QueueKind }) {
  if (!props.items.length) {
    return (
      <div className="text-muted-foreground flex min-h-40 items-center justify-center px-4 text-sm">
        当前队列没有内容
      </div>
    );
  }
  return (
    <>
      <div className="divide-border divide-y sm:hidden">
        {props.items.map(({ alert, queueStatus, queueReason, danger }) => (
          <article className="space-y-2.5 p-3" key={`${queueStatus}:${alert.incident_key}`}>
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <strong className="block font-medium">{alertTypeLabel(alert.event_type)}</strong>
                <span className="text-muted-foreground mt-0.5 block text-xs">
                  {alertObjectLabel(alert)}
                </span>
              </div>
              <span
                className={
                  danger
                    ? "text-destructive max-w-32 shrink-0 text-right text-xs font-medium"
                    : "max-w-32 shrink-0 text-right text-xs font-medium"
                }
              >
                {queueStatus}
              </span>
            </div>
            {queueReason ? (
              <div className="bg-muted/50 rounded-md px-2.5 py-2 text-xs">
                <span className="text-muted-foreground">通知处理：</span>
                <span>{queueReason}</span>
              </div>
            ) : null}
            <div className="text-sm">
              <span>{alertCauseLabel(alert.cause_code)}</span>
              {alert.last_error ? (
                <span className="text-destructive mt-0.5 block text-xs">{alert.last_error}</span>
              ) : null}
            </div>
            <div className="text-muted-foreground flex flex-wrap items-center justify-between gap-2 text-xs">
              <span>最近更新 {formatQueueTime(alert.last_seen_at)}</span>
              {props.kind === "consumer" ? (
                <span>{alertDeliveryLabel(alert.delivery_status, alert.delivery_attempts)}</span>
              ) : null}
            </div>
          </article>
        ))}
      </div>
      <div className="hidden sm:block">
        <Table className="min-w-[760px]" containerClassName="overflow-x-auto">
          <TableHeader>
            <TableRow>
              <TableHead className="w-[28%]">告警与对象</TableHead>
              <TableHead className="w-32">队列状态</TableHead>
              <TableHead>原因</TableHead>
              <TableHead className="w-32">最近更新</TableHead>
              {props.kind === "consumer" ? <TableHead className="w-32">通知情况</TableHead> : null}
            </TableRow>
          </TableHeader>
          <TableBody>
            {props.items.map(({ alert, queueStatus, queueReason, danger }) => (
              <TableRow key={`${queueStatus}:${alert.incident_key}`}>
                <TableCell className="whitespace-normal" overflowTooltip={false}>
                  <strong className="block font-medium">{alertTypeLabel(alert.event_type)}</strong>
                  <span className="text-muted-foreground mt-0.5 block text-xs">
                    {alertObjectLabel(alert)}
                  </span>
                </TableCell>
                <TableCell className="whitespace-normal" overflowTooltip={false}>
                  <span className={danger ? "text-destructive font-medium" : "font-medium"}>
                    {queueStatus}
                  </span>
                  {queueReason ? (
                    <span className="text-muted-foreground mt-0.5 block text-xs">
                      {queueReason}
                    </span>
                  ) : null}
                </TableCell>
                <TableCell className="whitespace-normal" overflowTooltip={false}>
                  <span>{alertCauseLabel(alert.cause_code)}</span>
                  {alert.last_error ? (
                    <span className="text-destructive mt-0.5 block text-xs">
                      {alert.last_error}
                    </span>
                  ) : null}
                </TableCell>
                <TableCell>{formatQueueTime(alert.last_seen_at)}</TableCell>
                {props.kind === "consumer" ? (
                  <TableCell className="whitespace-normal" overflowTooltip={false}>
                    {alertDeliveryLabel(alert.delivery_status, alert.delivery_attempts)}
                    {alert.delivered_at ? (
                      <span className="text-muted-foreground mt-0.5 block text-xs">
                        {formatQueueTime(alert.delivered_at)}
                      </span>
                    ) : null}
                  </TableCell>
                ) : null}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </>
  );
}

export function NotificationQueueStatus(props: {
  queues: NotificationStatus["queues"];
  loadDetails: () => Promise<NotificationQueueDetails>;
}) {
  const [dialogKind, setDialogKind] = useState<QueueKind | null>(null);
  const [details, setDetails] = useState<NotificationQueueDetails | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadQueue = async (kind: QueueKind) => {
    setDialogKind(kind);
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

  const items = details && dialogKind ? queueItems(details, dialogKind) : [];
  return (
    <>
      <Card className="mb-3 overflow-hidden">
        <CardHeader className="flex-row items-center justify-between gap-3 border-b px-4 py-3">
          <div className="min-w-0">
            <CardTitle>告警队列</CardTitle>
            <CardDescription>查看检测结果与待处理通知</CardDescription>
          </div>
          <StatusBadge
            label={props.queues.consumer_active ? "消费中" : "等待任务"}
            icon={Radio}
            pulse={props.queues.consumer_active}
            variant={props.queues.consumer_active ? "success" : "neutral"}
          />
        </CardHeader>
        <CardContent className="p-0">
          <section className="grid gap-3 border-b px-4 py-3 sm:grid-cols-[minmax(12rem,1fr)_auto_auto] sm:items-center sm:gap-5">
            <div className="flex min-w-0 items-center gap-2 font-medium">
              <BellRing className="text-muted-foreground size-4 shrink-0" aria-hidden="true" />
              <span>告警生产队列</span>
            </div>
            <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
              <QueueMetric label="告警中" value={props.queues.producer_firing} />
              <QueueMetric label="已恢复" value={props.queues.producer_recovered} />
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="justify-self-start sm:justify-self-end"
              onClick={() => void loadQueue("producer")}
            >
              <Eye aria-hidden="true" />
              查看
            </Button>
          </section>
          <section className="grid gap-3 px-4 py-3 sm:grid-cols-[minmax(12rem,1fr)_auto_auto] sm:items-center sm:gap-5">
            <div className="flex min-w-0 items-center gap-2 font-medium">
              <Send className="text-muted-foreground size-4 shrink-0" aria-hidden="true" />
              <span>通知消费队列</span>
            </div>
            <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
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
              className="justify-self-start sm:justify-self-end"
              onClick={() => void loadQueue("consumer")}
            >
              <Eye aria-hidden="true" />
              查看
            </Button>
          </section>
        </CardContent>
      </Card>

      <Dialog
        open={dialogKind !== null}
        onOpenChange={(open) => {
          if (!open) setDialogKind(null);
        }}
      >
        <DialogContent className="grid max-h-[min(42rem,calc(100svh-2rem))] min-w-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden sm:max-w-5xl">
          <DialogHeader>
            <DialogTitle>{dialogKind === "consumer" ? "通知消费队列" : "告警生产队列"}</DialogTitle>
            <DialogDescription>
              {dialogKind === "consumer"
                ? `待发送 ${details?.consumer_pending.length ?? props.queues.consumer_pending} 条，发送失败 ${details?.consumer_failed.length ?? props.queues.consumer_failed} 条${details ? `，已抑制或本轮不发送 ${Math.max(0, details.consumer_items.length - details.consumer_pending.length)} 条` : ""}`
                : `告警中 ${details?.producer_firing.length ?? props.queues.producer_firing} 条，已恢复 ${details?.producer_recovered.length ?? props.queues.producer_recovered} 条`}
            </DialogDescription>
          </DialogHeader>
          <div className="border-border min-h-0 overflow-auto rounded-lg border">
            {loading ? (
              <div className="text-muted-foreground flex min-h-40 items-center justify-center gap-2 text-sm">
                <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
                正在读取队列内容
              </div>
            ) : error ? (
              <div className="flex min-h-40 flex-col items-center justify-center gap-3 px-4 text-center text-sm">
                <span className="text-destructive">{error}</span>
                {dialogKind ? (
                  <Button variant="outline" size="sm" onClick={() => void loadQueue(dialogKind)}>
                    重新读取
                  </Button>
                ) : null}
              </div>
            ) : (
              <QueueDetailsTable items={items} kind={dialogKind ?? "producer"} />
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
