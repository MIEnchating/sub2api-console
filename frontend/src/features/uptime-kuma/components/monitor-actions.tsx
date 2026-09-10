import { Pause, Pencil, Play, Trash2 } from "lucide-react";
import type { KumaMonitor } from "@/api";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { actionLabels } from "../constants";

export function MonitorActions(props: {
  monitor: KumaMonitor;
  disabled: boolean;
  onEdit: (monitor: KumaMonitor) => void;
  onAction: (monitor: KumaMonitor, action: keyof typeof actionLabels) => void;
}) {
  const monitor = props.monitor;
  return (
    <div
      className="flex items-center justify-end gap-1"
      role="group"
      aria-label={`${monitor.name} 的操作`}
    >
      <TableActionButton
        label={monitor.active ? actionLabels.pause : actionLabels.resume}
        disabled={props.disabled}
        onClick={() => props.onAction(monitor, monitor.active ? "pause" : "resume")}
      >
        {monitor.active ? <Pause aria-hidden="true" /> : <Play aria-hidden="true" />}
      </TableActionButton>
      <TableActionButton
        label="编辑"
        disabled={props.disabled}
        onClick={() => props.onEdit(monitor)}
      >
        <Pencil aria-hidden="true" />
      </TableActionButton>
      <TableActionButton
        label="删除"
        tone="danger"
        disabled={props.disabled}
        onClick={() => props.onAction(monitor, "delete")}
      >
        <Trash2 aria-hidden="true" />
      </TableActionButton>
    </div>
  );
}
