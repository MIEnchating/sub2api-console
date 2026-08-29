import { Activity, Ban, LoaderCircle, Pause, Pencil, Pin, Play, RefreshCw } from "lucide-react";

import type { AccountControlAction, AccountStatus } from "@/api";
import { TableActionButton } from "@/components/data-table/table-action-button";
import { accountPoolState } from "@/features/accounts/lib/account-pool";
import { cn } from "@/lib/utils";

export function AccountOperationButtons(props: {
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
}) {
  const state = accountPoolState(props.account).value;
  const paused = state === "paused";
  const fused = state === "fused";
  const policyStopped = state === "cost_blocked" || fused;
  const resumable = paused || (!policyStopped && props.account.schedulable === false);
  const excluded = state === "excluded";
  return (
    <div className="ml-auto grid w-[7rem] grid-cols-3 gap-1">
      {excluded ? (
        <TableActionButton
          label="恢复管控"
          tone="primary"
          className="col-span-2 w-full"
          disabled={props.pending}
          onClick={() => props.onControl("include", "恢复管控")}
        >
          <Play />
        </TableActionButton>
      ) : (
        <>
          <TableActionButton
            label={props.probePending ? "正在探活" : "探活测试"}
            disabled={props.pending || props.probePending}
            onClick={props.onProbe}
          >
            {props.probePending ? <LoaderCircle className="animate-spin" /> : <Activity />}
          </TableActionButton>
          <TableActionButton
            label={resumable ? "恢复调度" : policyStopped ? "已停止调度" : "暂停调度"}
            tone={resumable ? "primary" : "default"}
            disabled={props.pending || policyStopped}
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
          </TableActionButton>
          <TableActionButton
            label={fused ? "解除熔断" : "手动熔断"}
            tone={fused ? "primary" : "danger"}
            disabled={props.pending || paused}
            onClick={() =>
              props.onControl(
                fused ? "recover" : "fuse",
                fused ? "解除熔断" : "手动熔断",
                fused ? undefined : `手动熔断“${props.account.name}”后，该账号将立即停止参与调度。`,
              )
            }
          >
            {fused ? <RefreshCw /> : <Ban />}
          </TableActionButton>
        </>
      )}
      <TableActionButton label="同步账号倍率" disabled={props.pending} onClick={props.onRateSync}>
        <RefreshCw />
      </TableActionButton>
      <TableActionButton
        label={props.account.manual_priority == null ? "设置人工优先位" : "调整人工优先位"}
        tone={props.account.manual_priority == null ? "default" : "primary"}
        disabled={props.pending}
        onClick={props.onManualPriority}
      >
        <Pin />
      </TableActionButton>
      <TableActionButton
        label="查看并编辑账号"
        className={cn(excluded && "col-span-3 w-full")}
        onClick={props.onEdit}
      >
        <Pencil />
      </TableActionButton>
    </div>
  );
}
