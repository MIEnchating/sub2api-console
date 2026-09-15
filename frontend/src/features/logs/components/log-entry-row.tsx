import {
  Ban,
  CircleAlert,
  CircleCheck,
  CircleX,
  Clock3,
  Eye,
  Info,
  LoaderCircle,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import type { UnifiedLogEntry } from "@/api";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { StatusBadge, type StatusVariant } from "@/components/status-badge";
import { Badge } from "@/components/ui/badge";
import { TableCell, TableRow } from "@/components/ui/table";
import { TableOverflowTooltip } from "@/components/ui/table-overflow-tooltip";
import { cn } from "@/lib/utils";
import {
  formatLogDate,
  logEventLevel,
  logEventLevelLabel,
  logKindLabel,
  logSourceLabel,
  logStatusLabel,
  logStatusVariant,
  logTitleLabel,
} from "../lib/log-display";

const statusStyles: Record<StatusVariant, { icon: LucideIcon; className: string }> = {
  success: { icon: CircleCheck, className: "bg-success/10" },
  warning: { icon: CircleAlert, className: "bg-warning/10" },
  danger: { icon: CircleX, className: "bg-destructive/10" },
  info: { icon: Info, className: "bg-info/10" },
  purple: { icon: Info, className: "bg-purple-500/10" },
  neutral: { icon: Ban, className: "bg-muted" },
};

function LogEntryStatus(props: { entry: UnifiedLogEntry }) {
  let variant = logStatusVariant(props.entry.status);
  let label = logStatusLabel(props.entry.status);
  if (props.entry.kind === "event") {
    const level = logEventLevel(props.entry.status);
    label = logEventLevelLabel(level);
    variant = level === "error" ? "danger" : level;
  }
  const presentation = statusStyles[variant];
  let icon = presentation.icon;
  if (props.entry.kind !== "event") {
    if (props.entry.status === "running") icon = LoaderCircle;
    if (["queued", "pending", "waiting_input"].includes(props.entry.status)) icon = Clock3;
  }

  return (
    <StatusBadge
      label={label}
      variant={variant}
      icon={icon}
      className={cn(presentation.className, "h-6 px-2 text-xs [&>span]:text-xs")}
    />
  );
}

export function LogEntryRow(props: {
  entry: UnifiedLogEntry;
  onSelect: (entry: UnifiedLogEntry) => void;
}) {
  const title = logTitleLabel(props.entry.title);
  const source = logSourceLabel(props.entry.source);
  const object = props.entry.object_label?.trim();
  const actor = props.entry.actor?.trim();
  let secondary = "";
  if (actor) secondary = `执行人：${actor}`;
  else if (object) secondary = source;

  return (
    <TableRow>
      <TableCell overflowTooltip={false}>
        <time
          dateTime={props.entry.occurred_at}
          className="text-muted-foreground block text-xs leading-5 whitespace-normal tabular-nums"
        >
          {formatLogDate(props.entry.occurred_at)}
        </time>
      </TableCell>
      <TableCell overflowTooltip={false}>
        <Badge variant="secondary" className="text-muted-foreground rounded-md font-normal">
          {logKindLabel(props.entry.kind)}
        </Badge>
      </TableCell>
      <TableCell overflowTooltip={false}>
        <div className="grid min-w-0 gap-1">
          <div className="flex min-w-0 items-center gap-2">
            <TableOverflowTooltip className="text-sm font-medium" content={title}>
              {title}
            </TableOverflowTooltip>
            {props.entry.related_count > 0 && (
              <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
                关联 {props.entry.related_count} 条
              </span>
            )}
          </div>
          {props.entry.summary && (
            <TableOverflowTooltip
              className="text-muted-foreground line-clamp-2 text-xs leading-5 break-all whitespace-normal"
              content={props.entry.summary}
            >
              {props.entry.summary}
            </TableOverflowTooltip>
          )}
        </div>
      </TableCell>
      <TableCell overflowTooltip={false}>
        <div className="grid min-w-0 gap-1">
          <TableOverflowTooltip
            className={object ? "truncate text-sm" : "text-muted-foreground truncate text-xs"}
            content={object || source}
          >
            {object || source}
          </TableOverflowTooltip>
          {secondary && (
            <TableOverflowTooltip className="text-muted-foreground text-xs" content={secondary}>
              {secondary}
            </TableOverflowTooltip>
          )}
        </div>
      </TableCell>
      <TableCell overflowTooltip={false}>
        <LogEntryStatus entry={props.entry} />
      </TableCell>
      <TableCell className="text-right" overflowTooltip={false}>
        <TableActionButton label="查看日志详情" onClick={() => props.onSelect(props.entry)}>
          <Eye aria-hidden="true" />
        </TableActionButton>
      </TableCell>
    </TableRow>
  );
}
