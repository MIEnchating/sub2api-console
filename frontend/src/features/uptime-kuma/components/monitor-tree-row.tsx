import { ChevronDown, ChevronRight, Folder } from "lucide-react";
import type { KumaMonitor } from "@/api";
import { Button } from "@/components/ui/button";
import { TableOverflowTooltip } from "@/components/ui/table-overflow-tooltip";
import { TableCell, TableRow } from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { actionLabels, monitorTypeLabels } from "../constants";
import type { MonitorTreeRow as TreeRow } from "../lib/monitor-tree";
import { MonitorStatusBadge } from "./monitor-detail";
import { MonitorActions } from "./monitor-actions";

export function MonitorTreeRow(props: {
  row: TreeRow;
  expanded: boolean;
  filtering: boolean;
  management: boolean;
  disabled: boolean;
  onToggle: (key: string) => void;
  onDetail: (key: string) => void;
  onEdit: (monitor: KumaMonitor) => void;
  onAction: (monitor: KumaMonitor, action: keyof typeof actionLabels) => void;
}) {
  const monitor = props.row.monitor;
  const expandable = props.row.hasChildren;
  return (
    <TableRow>
      <TableCell overflowTooltip={false}>
        <div className="flex min-w-0 items-center gap-1">
          {props.row.ancestors.slice(0, 4).map((item) => (
            <span key={item.key} aria-hidden="true" className="w-3 shrink-0" />
          ))}
          {expandable ? (
            <Tooltip disabled={!props.filtering}>
              <TooltipTrigger render={<span className="inline-flex shrink-0" />}>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-expanded={props.expanded}
                  aria-label={`${props.expanded ? "收起" : "展开"}分组 ${monitor.name}`}
                  disabled={props.filtering}
                  aria-description={props.filtering ? "筛选时自动展开匹配分组" : undefined}
                  onClick={() => props.onToggle(monitor.key)}
                >
                  {props.expanded ? (
                    <ChevronDown aria-hidden="true" />
                  ) : (
                    <ChevronRight aria-hidden="true" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>筛选时自动展开匹配分组</TooltipContent>
            </Tooltip>
          ) : (
            <span aria-hidden="true" className="size-8 shrink-0" />
          )}
          {monitor.type === "group" ? (
            <Folder aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
          ) : (
            <span aria-hidden="true" className="size-4 shrink-0" />
          )}
          <Button
            variant="link"
            aria-label={`查看 ${monitor.name} 详情`}
            className="min-w-0 flex-1 justify-start px-0"
            onClick={() => props.onDetail(monitor.key)}
          >
            <TableOverflowTooltip content={monitor.name}>{monitor.name}</TableOverflowTooltip>
          </Button>
        </div>
      </TableCell>
      <TableCell>{props.management ? props.row.parentLabel : "—"}</TableCell>
      <TableCell>
        <MonitorStatusBadge monitor={monitor} management={props.management} />
      </TableCell>
      <TableCell>{monitorTypeLabels[monitor.type] ?? monitor.type}</TableCell>
      <TableCell>{monitor.target || monitor.url || "—"}</TableCell>
      <TableCell>{monitor.interval > 0 ? `${monitor.interval} 秒` : "—"}</TableCell>
      <TableCell>
        {monitor.response_time === null ? "—" : `${monitor.response_time.toFixed(0)} ms`}
      </TableCell>
      <TableCell>
        {monitor.uptime === null ? "—" : `${(monitor.uptime * 100).toFixed(2)}%`}
      </TableCell>
      {props.management && (
        <TableCell overflowTooltip={false}>
          <MonitorActions
            monitor={monitor}
            disabled={props.disabled}
            onEdit={props.onEdit}
            onAction={props.onAction}
          />
        </TableCell>
      )}
    </TableRow>
  );
}
