import { ScrollText, SearchX } from "lucide-react";

import type { UnifiedLogEntry, UnifiedLogKind } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { TableEmptyState } from "@/components/data-table/empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { LogEntryRow } from "./log-entry-row";

export function LogsTable(props: {
  items: UnifiedLogEntry[];
  kind: UnifiedLogKind;
  loading: boolean;
  unavailable: boolean;
  refreshing: boolean;
  filtered: boolean;
  onRetry: () => void;
  onSelect: (entry: UnifiedLogEntry) => void;
}) {
  const EmptyIcon = props.filtered ? SearchX : ScrollText;

  return (
    <div className="min-h-0 flex-1 overflow-hidden" data-testid="logs-table-scroll-region">
      <Table
        actionColumn
        aria-label="日志记录"
        uniformTextSize={false}
        containerClassName="h-full min-h-0 overflow-auto overscroll-contain"
        className="min-w-[920px] [&_td]:px-4 [&_td]:py-3 [&_th]:px-4"
      >
        <TableHeader>
          <TableRow>
            <TableHead className="w-28">时间</TableHead>
            <TableHead className="w-28">类型</TableHead>
            <TableHead>记录</TableHead>
            <TableHead className="w-44 xl:w-52">对象 / 执行人</TableHead>
            <TableHead className="w-32">{props.kind === "event" ? "级别" : "状态"}</TableHead>
            <TableHead className="w-16 text-right">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.loading && Array.from({ length: 6 }, (_, index) => <LogSkeletonRow key={index} />)}
          {props.unavailable && (
            <TableEmptyState columns={6}>
              <ContentRetry pending={props.refreshing} onRetry={props.onRetry} />
            </TableEmptyState>
          )}
          {!props.loading && !props.unavailable && props.items.length === 0 && (
            <TableEmptyState columns={6}>
              <div className="grid justify-items-center gap-2 py-6">
                <span className="bg-muted flex size-10 items-center justify-center rounded-xl">
                  <EmptyIcon className="size-5" aria-hidden="true" />
                </span>
                <span className="text-foreground font-medium">
                  {props.filtered ? "没有匹配的记录" : "暂无日志记录"}
                </span>
                <span className="text-xs">
                  {props.filtered
                    ? "试试其他关键词或调整筛选条件"
                    : "任务执行和系统事件将显示在这里"}
                </span>
              </div>
            </TableEmptyState>
          )}
          {props.items.map((entry) => (
            <LogEntryRow key={entry.id} entry={entry} onSelect={props.onSelect} />
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function LogSkeletonRow() {
  return (
    <TableRow aria-label="正在加载日志">
      <TableCell overflowTooltip={false}>
        <div className="grid gap-2">
          <Skeleton className="h-3 w-10" />
          <Skeleton className="h-3 w-16" />
        </div>
      </TableCell>
      <TableCell overflowTooltip={false}>
        <Skeleton className="h-5 w-16 rounded-md" />
      </TableCell>
      <TableCell overflowTooltip={false}>
        <div className="grid gap-2">
          <Skeleton className="h-4 w-28" />
          <Skeleton className="h-3 w-4/5" />
        </div>
      </TableCell>
      <TableCell overflowTooltip={false}>
        <Skeleton className="h-3 w-20" />
      </TableCell>
      <TableCell overflowTooltip={false}>
        <Skeleton className="h-6 w-20 rounded-full" />
      </TableCell>
      <TableCell overflowTooltip={false}>
        <Skeleton className="ml-auto size-8 rounded-md" />
      </TableCell>
    </TableRow>
  );
}
