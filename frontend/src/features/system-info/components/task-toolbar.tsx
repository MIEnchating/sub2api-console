import type { ReactElement } from "react";

import type { TaskSummary } from "@/api";
import { FilterMenu } from "@/components/data-table/filter-menu";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { useDictionaryOrder } from "@/hooks/use-dictionary-order";
import { taskStatusDictionary } from "@/lib/domain-dictionaries";
import { activeTaskStatuses, taskStatusLabel } from "../constants";

export type TaskListGroup = "active" | "history";

export function TaskToolbar(props: {
  group: TaskListGroup;
  counts: Record<TaskListGroup, number> | null;
  statusFilter: TaskSummary["status"] | null;
  onGroupChange: (group: TaskListGroup) => void;
  onStatusChange: (status: TaskSummary["status"] | null) => void;
}): ReactElement {
  const statusOptions = useDictionaryOrder(
    "task_status",
    Object.keys(taskStatusDictionary) as TaskSummary["status"][],
    (value) => value,
  );
  return (
    <div className="flex min-w-0 shrink-0 flex-wrap items-center justify-between gap-3 border-b p-3">
      <SegmentedControl
        role="tablist"
        aria-label="任务分类"
        className="grid w-full grid-cols-2 sm:inline-flex sm:w-fit"
      >
        <SegmentedControlItem
          id="system-task-tab-active"
          type="button"
          role="tab"
          aria-controls="system-task-panel"
          selected={props.group === "active"}
          onClick={() => props.onGroupChange("active")}
        >
          进行中任务{" "}
          <span className="bg-muted text-muted-foreground min-w-5 rounded px-1 text-xs tabular-nums">
            {props.counts?.active ?? "—"}
          </span>
        </SegmentedControlItem>
        <SegmentedControlItem
          id="system-task-tab-history"
          type="button"
          role="tab"
          aria-controls="system-task-panel"
          selected={props.group === "history"}
          onClick={() => props.onGroupChange("history")}
        >
          历史任务{" "}
          <span className="bg-muted text-muted-foreground min-w-5 rounded px-1 text-xs tabular-nums">
            {props.counts?.history ?? "—"}
          </span>
        </SegmentedControlItem>
      </SegmentedControl>
      <div className="flex min-w-0 flex-1 flex-wrap items-center justify-between gap-2 sm:justify-end">
        <span className="text-muted-foreground text-xs">最近 20 条任务</span>
        <FilterMenu
          label="任务状态"
          options={statusOptions.filter(
            (status) => activeTaskStatuses.has(status) === (props.group === "active"),
          )}
          value={props.statusFilter}
          onValueChange={props.onStatusChange}
          optionLabel={taskStatusLabel}
        />
      </div>
    </div>
  );
}
