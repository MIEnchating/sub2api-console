import { ListTodo, SearchX } from "lucide-react";
import type { ReactElement } from "react";

import type { TaskSummary } from "@/api";
import { ContentRetry } from "@/components/content-retry";
import { TableEmptyState } from "@/components/data-table/empty-state";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { TaskRow } from "./task-row";

export function TaskTable(props: {
  tasks: TaskSummary[];
  group: "active" | "history";
  filtered: boolean;
  loading: boolean;
  unavailable: boolean;
  refreshing: boolean;
  onRetry: () => void;
  onClearFilter: () => void;
  onSelect: (id: string) => void;
}): ReactElement {
  const EmptyIcon = props.filtered ? SearchX : ListTodo;
  let emptyMessage = "当前没有进行中的任务";
  let emptyHint = "后台任务启动后会显示在这里";
  if (props.group === "history") {
    emptyMessage = "当前没有历史任务";
    emptyHint = "已结束的任务会显示在这里";
  }
  if (props.filtered) {
    emptyMessage = "没有符合筛选条件的任务";
    emptyHint = "调整任务状态，或清除筛选查看当前分类";
  }

  return (
    <div className="min-h-0 flex-1 overflow-hidden">
      <Table
        actionColumn
        aria-label="后台任务"
        aria-busy={props.loading}
        uniformTextSize={false}
        containerClassName="h-full min-h-0 overflow-auto overscroll-contain"
        className="min-w-[760px] [&_td]:px-3 [&_td]:py-3 [&_th]:px-3 sm:[&_td]:px-4 sm:[&_th]:px-4"
      >
        <TableHeader>
          <TableRow>
            <TableHead className="w-44">任务类型</TableHead>
            <TableHead className="w-28">状态</TableHead>
            <TableHead>进度与消息</TableHead>
            <TableHead className="w-32">更新时间</TableHead>
            <TableHead className="w-16 text-right">操作</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {props.loading &&
            Array.from({ length: 5 }, (_, index) => (
              <TableRow key={index} aria-label="正在加载任务">
                <TableCell overflowTooltip={false}>
                  <div className="grid gap-2">
                    <Skeleton className="h-4 w-20" />
                    <Skeleton className="h-3 w-28" />
                  </div>
                </TableCell>
                <TableCell overflowTooltip={false}>
                  <Skeleton className="h-5 w-16 rounded-full" />
                </TableCell>
                <TableCell overflowTooltip={false}>
                  <div className="grid gap-2">
                    <Skeleton className="h-4 w-4/5" />
                    <Skeleton className="h-1 w-full" />
                  </div>
                </TableCell>
                <TableCell overflowTooltip={false}>
                  <Skeleton className="h-4 w-20" />
                </TableCell>
                <TableCell overflowTooltip={false}>
                  <Skeleton className="ml-auto size-8 rounded-md" />
                </TableCell>
              </TableRow>
            ))}
          {props.unavailable && (
            <TableEmptyState columns={5}>
              <ContentRetry pending={props.refreshing} onRetry={props.onRetry} />
            </TableEmptyState>
          )}
          {!props.loading && !props.unavailable && props.tasks.length === 0 && (
            <TableEmptyState columns={5}>
              <div className="grid justify-items-center gap-2">
                <span className="bg-muted flex size-10 items-center justify-center rounded-xl">
                  <EmptyIcon className="size-5" aria-hidden="true" />
                </span>
                <span className="text-foreground font-medium">{emptyMessage}</span>
                <span className="text-xs">{emptyHint}</span>
                {props.filtered && (
                  <Button type="button" variant="outline" onClick={props.onClearFilter}>
                    清除筛选
                  </Button>
                )}
              </div>
            </TableEmptyState>
          )}
          {props.tasks.map((task) => (
            <TaskRow key={task.id} task={task} onSelect={props.onSelect} />
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
