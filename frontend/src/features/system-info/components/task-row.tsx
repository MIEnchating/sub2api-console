import { Eye } from "lucide-react";
import type { ReactElement } from "react";

import type { TaskSummary } from "@/api";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { StatusBadge } from "@/components/status-badge";
import { Progress } from "@/components/ui/progress";
import { TableCell, TableRow } from "@/components/ui/table";
import { TableOverflowTooltip } from "@/components/ui/table-overflow-tooltip";
import { taskOperationLabel, taskStatusLabel, taskStatusVariant } from "../constants";
import { formatTaskDate } from "../lib/format-task-date";

export function TaskRow(props: {
  task: TaskSummary;
  onSelect: (id: string) => void;
}): ReactElement {
  const operation = taskOperationLabel(props.task.operation);
  return (
    <TableRow>
      <TableCell overflowTooltip={false}>
        <div className="grid min-w-0 gap-1">
          <TableOverflowTooltip className="font-medium" content={operation}>
            {operation}
          </TableOverflowTooltip>
          <TableOverflowTooltip
            className="text-muted-foreground font-mono text-xs"
            content={props.task.id}
          >
            {props.task.id}
          </TableOverflowTooltip>
        </div>
      </TableCell>
      <TableCell overflowTooltip={false}>
        <StatusBadge
          label={taskStatusLabel(props.task.status)}
          variant={taskStatusVariant(props.task.status)}
        />
      </TableCell>
      <TableCell overflowTooltip={false}>
        <div className="grid min-w-0 gap-2">
          <div className="flex min-w-0 items-start gap-3">
            <TableOverflowTooltip
              className="min-w-0 flex-1 line-clamp-2 text-sm leading-5 wrap-anywhere whitespace-normal"
              content={props.task.message}
            >
              {props.task.message || "—"}
            </TableOverflowTooltip>
            <span className="text-muted-foreground shrink-0 text-xs leading-5 tabular-nums">
              {props.task.progress}%
            </span>
          </div>
          <Progress value={props.task.progress} aria-label={`${props.task.id} 任务进度`} />
        </div>
      </TableCell>
      <TableCell overflowTooltip={false}>
        <time
          dateTime={props.task.updated_at}
          className="text-muted-foreground block text-xs leading-5 whitespace-normal tabular-nums"
        >
          {formatTaskDate(props.task.updated_at)}
        </time>
      </TableCell>
      <TableCell className="text-right" overflowTooltip={false}>
        <TableActionButton label="查看任务" onClick={() => props.onSelect(props.task.id)}>
          <Eye aria-hidden="true" />
        </TableActionButton>
      </TableCell>
    </TableRow>
  );
}
