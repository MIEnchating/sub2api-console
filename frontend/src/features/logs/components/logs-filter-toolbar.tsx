import { useDictionaryOrder } from "@/hooks/use-dictionary-order";
import { Info } from "lucide-react";

import type { GroupStatus, UnifiedLogEventLevel, UnifiedLogKind, UnifiedLogState } from "@/api";
import { FilterMenu } from "@/components/data-table/filter-menu";
import { TableFilterToolbar } from "@/components/data-table/filter-toolbar";
import { SearchField } from "@/components/data-table/search-field";
import { SegmentedControl, SegmentedControlItem } from "@/components/ui/segmented-control";
import { logEventLevelLabel, logKindLabel, logStateLabel } from "../lib/log-display";

export const logKinds: UnifiedLogKind[] = ["all", "task", "event", "change"];
const states: UnifiedLogState[] = ["all", "active", "failed", "warning", "succeeded"];
const eventLevels: UnifiedLogEventLevel[] = ["all", "info", "warning", "error"];

export function LogKindFilter(props: {
  value: UnifiedLogKind;
  onChange: (value: UnifiedLogKind) => void;
}) {
  return (
    <SegmentedControl
      role="tablist"
      aria-label="记录类型"
      className="grid w-full grid-cols-4 sm:inline-flex sm:w-fit"
    >
      {logKinds.map((option) => {
        const selected = props.value === option;
        return (
          <SegmentedControlItem
            key={option}
            id={`logs-kind-tab-${option}`}
            type="button"
            role="tab"
            selected={selected}
            className="min-w-0 px-1 sm:px-3"
            aria-controls="logs-results-panel"
            onClick={() => props.onChange(option)}
          >
            {logKindLabel(option)}
          </SegmentedControlItem>
        );
      })}
    </SegmentedControl>
  );
}

function LogStateFilter(props: {
  value: UnifiedLogState;
  onChange: (value: UnifiedLogState) => void;
}) {
  return (
    <FilterMenu
      label="执行结果"
      options={states.filter((option) => option !== "all")}
      value={props.value === "all" ? null : props.value}
      onValueChange={(value) => props.onChange(value ?? "all")}
      optionLabel={logStateLabel}
    />
  );
}

function EventLevelFilter(props: {
  value: UnifiedLogEventLevel;
  onChange: (value: UnifiedLogEventLevel) => void;
}) {
  return (
    <FilterMenu
      label="事件级别"
      options={eventLevels.filter((option) => option !== "all")}
      value={props.value === "all" ? null : props.value}
      onValueChange={(value) => props.onChange(value ?? "all")}
      optionLabel={logEventLevelLabel}
    />
  );
}

function EventGroupFilter(props: {
  value: string;
  groups: GroupStatus[];
  onChange: (value: string) => void;
}) {
  const groups = useDictionaryOrder("group", props.groups, (group) => group.id ?? "");
  return (
    <FilterMenu
      label="事件分组"
      options={groups.map((group) => group.name)}
      value={props.value === "all" ? null : props.value}
      onValueChange={(value) => props.onChange(value ?? "all")}
    />
  );
}

export function LogsFilterToolbar(props: {
  search: string;
  kind: UnifiedLogKind;
  state: UnifiedLogState;
  eventLevel: UnifiedLogEventLevel;
  eventGroup: string;
  groups: GroupStatus[];
  truncated: boolean;
  onSearchChange: (value: string) => void;
  onKindChange: (value: UnifiedLogKind) => void;
  onStateChange: (value: UnifiedLogState) => void;
  onEventLevelChange: (value: UnifiedLogEventLevel) => void;
  onEventGroupChange: (value: string) => void;
}) {
  return (
    <div className="flex min-w-0 shrink-0 flex-col gap-3">
      <div className="flex min-w-0 flex-wrap items-center justify-between gap-2">
        <LogKindFilter value={props.kind} onChange={props.onKindChange} />
        {props.truncated && (
          <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
            <Info className="size-3.5 shrink-0" aria-hidden="true" />
            仅显示最近记录
          </span>
        )}
      </div>
      <TableFilterToolbar data-testid="logs-filter-toolbar" aria-label="日志筛选">
        <SearchField
          value={props.search}
          onChange={props.onSearchChange}
          placeholder="搜索任务、对象或原因"
        />
        {props.kind === "event" ? (
          <>
            <EventLevelFilter value={props.eventLevel} onChange={props.onEventLevelChange} />
            <EventGroupFilter
              value={props.eventGroup}
              groups={props.groups}
              onChange={props.onEventGroupChange}
            />
          </>
        ) : (
          <LogStateFilter value={props.state} onChange={props.onStateChange} />
        )}
      </TableFilterToolbar>
    </div>
  );
}
