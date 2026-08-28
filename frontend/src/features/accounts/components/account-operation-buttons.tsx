import { Activity, Ban, LoaderCircle, Pause, Pencil, Play, RefreshCw } from "lucide-react";

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
  onEdit: () => void;
}) {
  const state = accountPoolState(props.account).value;
  const paused = state === "paused";
  const fused = state === "fused";
  const excluded = state === "excluded";
  return (
    <div className="ml-auto grid w-[4.5rem] grid-cols-2 gap-1">
      {excluded ? (
        <TableActionButton
          label={`恢复管控 ${props.account.name}`}
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
            label={`${props.probePending ? "正在探活" : "探活测试"} ${props.account.name}`}
            disabled={props.pending || props.probePending}
            onClick={props.onProbe}
          >
            {props.probePending ? <LoaderCircle className="animate-spin" /> : <Activity />}
          </TableActionButton>
          <TableActionButton
            label={`${paused ? "恢复" : "暂停"}调度 ${props.account.name}`}
            tone={paused ? "primary" : "default"}
            disabled={props.pending}
            onClick={() =>
              props.onControl(
                paused ? "resume" : "pause",
                paused ? "恢复调度" : "暂停调度",
                paused
                  ? undefined
                  : `暂停“${props.account.name}”后，该账号将停止接收流量，但仍会继续监控和计分。`,
              )
            }
          >
            {paused ? <Play /> : <Pause />}
          </TableActionButton>
          <TableActionButton
            label={`${fused ? "解除熔断" : "手动熔断"} ${props.account.name}`}
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
          {props.account.upstream_host && (
            <TableActionButton
              label={`同步上游倍率 ${props.account.name}`}
              disabled={props.pending}
              onClick={props.onRateSync}
            >
              <RefreshCw />
            </TableActionButton>
          )}
        </>
      )}
      <TableActionButton
        label={`查看并编辑账号 ${props.account.name}`}
        className={cn(props.account.upstream_host && !excluded && "col-span-2 w-full")}
        onClick={props.onEdit}
      >
        <Pencil />
      </TableActionButton>
    </div>
  );
}
