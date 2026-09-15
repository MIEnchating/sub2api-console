import { Activity, Eye } from "lucide-react";
import type { ReactElement } from "react";

import type { AutoInspectionStatus } from "@/api";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { StatusBadge } from "@/components/status-badge";
import { cn } from "@/lib/utils";

export type InspectionQueueRoundProps = {
  item: AutoInspectionStatus["queue"][number];
  state: { label: string; tone: "info" | "neutral" | "danger" };
  countdown: string;
  scheduledAt: string;
  operationKind: (operation: string) => string;
  onDetails: () => void;
};

export function InspectionQueueRound(props: InspectionQueueRoundProps): ReactElement {
  const operations = props.item.operations ?? [];
  let dueIndex = 0;

  return (
    <section data-slot="queue-round" className="min-w-0">
      <div className="grid min-w-0 gap-3 px-3 py-3">
        <div className="flex min-w-0 items-start gap-2.5">
          <span className="bg-muted text-muted-foreground flex size-8 shrink-0 items-center justify-center rounded-md">
            <Activity size={15} aria-hidden="true" />
          </span>
          <div className="min-w-0 flex-1">
            <strong className="block text-sm leading-5 font-medium [overflow-wrap:anywhere]">
              {props.item.label}
            </strong>
            <div className="mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
              <span className="text-primary border-primary/25 bg-primary/5 rounded-full border px-1.5 py-0.5 text-[11px] font-medium">
                主巡检任务
              </span>
              {props.item.target_count !== null ? (
                <span className="text-muted-foreground text-xs">
                  {props.item.target_count} 个目标
                </span>
              ) : null}
              <StatusBadge label={props.state.label} variant={props.state.tone} />
            </div>
          </div>
        </div>
        <div className="flex min-w-0 items-center justify-between gap-3">
          <div className="min-w-0">
            <div className="text-sm font-medium [overflow-wrap:anywhere]">
              {props.item.scheduled_for ? props.countdown : "等待排期"}
            </div>
            {props.item.scheduled_for ? (
              <div className="text-muted-foreground mt-0.5 font-mono text-xs [overflow-wrap:anywhere]">
                {props.scheduledAt}
              </div>
            ) : null}
          </div>
          {operations.length ? (
            <TableActionButton label="查看任务详情" onClick={props.onDetails} className="shrink-0">
              <Eye aria-hidden="true" />
            </TableActionButton>
          ) : null}
        </div>
      </div>

      {operations.length ? (
        <div data-slot="queue-operations" className="border-border/70 min-w-0 border-t">
          <div className="text-muted-foreground bg-muted/20 flex min-h-9 items-center justify-between gap-3 px-3 text-xs font-medium">
            <span>执行计划</span>
            <span>本轮安排</span>
          </div>
          <ol className="divide-border/60 min-w-0 divide-y">
            {operations.map((operation) => {
              const operationDue = operation.due ?? props.item.state === "ready";
              if (operationDue) dueIndex += 1;

              return (
                <li
                  key={operation.operation}
                  data-slot="queue-operation"
                  className="grid min-w-0 grid-cols-[1.5rem_minmax(0,1fr)_auto] items-start gap-x-2 gap-y-1 px-3 py-3"
                >
                  <span
                    data-sequence={operationDue ? dueIndex : undefined}
                    className={cn(
                      "flex size-6 items-center justify-center rounded-full font-mono text-xs",
                      operationDue
                        ? "bg-primary/10 text-primary font-medium"
                        : "bg-muted text-muted-foreground",
                    )}
                  >
                    {operationDue ? dueIndex : "—"}
                  </span>
                  <div className="min-w-0">
                    <span className="block text-sm leading-6 font-medium [overflow-wrap:anywhere]">
                      {operation.label}
                    </span>
                    <div className="text-muted-foreground mt-0.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-xs">
                      <span className="[overflow-wrap:anywhere]">
                        {props.operationKind(operation.operation)}
                      </span>
                      {operation.target_count !== null ? (
                        <span>{operation.target_count} 个账号</span>
                      ) : null}
                    </div>
                  </div>
                  <span
                    className={cn(
                      "text-right text-xs leading-6 font-medium whitespace-nowrap",
                      operationDue ? "text-primary" : "text-muted-foreground",
                    )}
                  >
                    {operationDue ? "本轮执行" : "本轮不执行"}
                  </span>
                  <p className="text-muted-foreground col-span-2 col-start-2 min-w-0 text-xs leading-5 [overflow-wrap:anywhere]">
                    <span>执行周期：</span>
                    {operation.cycle || "继承调度策略"}
                  </p>
                </li>
              );
            })}
          </ol>
        </div>
      ) : (
        <div className="border-border/70 text-muted-foreground border-t px-3 py-4 text-sm [overflow-wrap:anywhere]">
          {props.item.state === "disabled"
            ? "启用自动巡检后才会生成执行计划"
            : "当前没有可执行操作"}
        </div>
      )}
    </section>
  );
}
