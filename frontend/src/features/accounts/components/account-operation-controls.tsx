import {
  Activity,
  Ban,
  LoaderCircle,
  MoreHorizontal,
  Pause,
  Pencil,
  Pin,
  Play,
  RefreshCw,
  Trash2,
} from "lucide-react";

import type { AccountControlAction, AccountStatus } from "@/api";
import type { ReactElement } from "react";
import { TableActionButton } from "@/components/data-table/table-action-button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import { accountPoolState } from "@/features/accounts/lib/account-pool";
import { cn } from "@/lib/utils";

const prominentDangerActionClassName =
  "border-destructive/40 bg-destructive/10 hover:border-destructive/60 hover:bg-destructive/20 focus-visible:border-destructive focus-visible:ring-destructive/30";

export type AccountOperationProps = {
  account: AccountStatus;
  pending: boolean;
  probePending: boolean;
  onProbe: () => void;
  onControl: (
    action: AccountControlAction,
    label: string,
    confirmationDescription?: string,
  ) => void;
  onRateSync: () => void;
  onManualPriority: () => void;
  onEdit: () => void;
  onDelete: () => void;
};

export function AccountOperationControls(
  props: AccountOperationProps & { expanded?: boolean },
): ReactElement {
  const state = accountPoolState(props.account).value;
  const paused = state === "paused";
  const fused = state === "fused";
  const policyStopped = state === "cost_blocked" || fused;
  const resumable = paused || (!policyStopped && props.account.schedulable === false);
  const excluded = state === "excluded";
  const manualControlled = props.account.manual_priority != null;
  const schedulingLabel = resumable ? "恢复调度" : "暂停调度";
  return (
    <div
      className={cn(
        "flex items-center gap-1",
        props.expanded ? "flex-wrap" : "ml-auto justify-end",
      )}
    >
      {excluded ? (
        <TableActionButton
          label="恢复管控"
          className={props.expanded ? "w-auto px-3" : undefined}
          tone="primary"
          disabled={props.pending || manualControlled}
          onClick={() => props.onControl("include", "恢复管控")}
        >
          <Play />
          {props.expanded ? <span>恢复管控</span> : null}
        </TableActionButton>
      ) : (
        <>
          <TableActionButton
            label={props.probePending ? "正在探活" : "探活测试"}
            className={props.expanded ? "w-auto px-3" : undefined}
            disabled={props.pending || props.probePending || manualControlled}
            onClick={props.onProbe}
          >
            {props.probePending ? <LoaderCircle className="animate-spin" /> : <Activity />}
            {props.expanded ? <span>{props.probePending ? "正在探活" : "探活测试"}</span> : null}
          </TableActionButton>
          {(props.expanded || resumable) && !policyStopped ? (
            <TableActionButton
              label={schedulingLabel}
              className={props.expanded ? "w-auto px-3" : undefined}
              tone={resumable ? "primary" : "default"}
              disabled={props.pending || policyStopped || manualControlled}
              onClick={() =>
                props.onControl(
                  resumable ? "resume" : "pause",
                  resumable ? "恢复调度" : "暂停调度",
                  resumable
                    ? undefined
                    : `暂停“${props.account.name}”后，该账号将停止接收流量，但仍会继续监控和计分。`,
                )
              }
            >
              {resumable ? <Play /> : <Pause />}
              {props.expanded ? <span>{schedulingLabel}</span> : null}
            </TableActionButton>
          ) : null}
          {!paused || props.expanded ? (
            <TableActionButton
              label={fused ? "解除熔断" : "手动熔断（停止调度）"}
              tone={fused ? "primary" : "danger"}
              className={cn(
                !fused && prominentDangerActionClassName,
                props.expanded && "w-auto px-3",
              )}
              disabled={props.pending || paused || manualControlled}
              onClick={() =>
                props.onControl(
                  fused ? "recover" : "fuse",
                  fused ? "解除熔断" : "手动熔断",
                  fused
                    ? `解除“${props.account.name}”的熔断后，该账号可重新接收流量，后续仍受调度策略约束。`
                    : `手动熔断“${props.account.name}”后，该账号会立即停止接收流量，并持续保持熔断状态，直到手动解除。`,
                )
              }
            >
              {fused ? <RefreshCw /> : <Ban />}
              {props.expanded ? <span>{fused ? "解除熔断" : "手动熔断"}</span> : null}
            </TableActionButton>
          ) : null}
        </>
      )}
      {props.expanded ? (
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                className="data-popup-open:bg-muted"
                aria-label="更多账号操作"
                disabled={props.pending}
              />
            }
          >
            <MoreHorizontal className="size-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            <DropdownMenuItem disabled={props.pending} onClick={props.onRateSync}>
              <RefreshCw />
              同步账号倍率
            </DropdownMenuItem>
            <DropdownMenuItem disabled={props.pending} onClick={props.onManualPriority}>
              <Pin />
              {props.account.manual_priority == null ? "设置人工优先位" : "调整人工优先位"}
            </DropdownMenuItem>
            <DropdownMenuItem disabled={props.pending || manualControlled} onClick={props.onEdit}>
              <Pencil />
              查看并编辑账号
            </DropdownMenuItem>
            <DropdownMenuItem
              className="text-destructive focus:text-destructive"
              disabled={props.pending}
              onClick={props.onDelete}
            >
              <Trash2 />
              删除账号及上游 Key
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
    </div>
  );
}
