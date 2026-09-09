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
import type { ReactElement } from "react";

import type { AccountControlAction, AccountStatus } from "@/api";
import { TableActionButton } from "@/components/data-table/table-action-button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import { accountPoolState } from "@/features/accounts/lib/account-pool";

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

type AccountAction = {
  label: string;
  icon: ReactElement;
  disabled: boolean;
  tone?: "primary" | "danger";
  prominent?: boolean;
  onClick: () => void;
};

function accountActions(props: AccountOperationProps): {
  controls: AccountAction[];
  maintenance: AccountAction[];
} {
  const state = accountPoolState(props.account).value;
  const paused = state === "paused";
  const fused = state === "fused";
  const policyStopped = state === "cost_blocked" || fused;
  const resumable = paused || (!policyStopped && props.account.schedulable === false);
  const manualControlled = props.account.manual_priority != null;
  const controls: AccountAction[] = [];
  if (state === "excluded") {
    controls.push({
      label: "恢复管控",
      icon: <Play />,
      tone: "primary",
      disabled: props.pending || manualControlled,
      onClick: () => props.onControl("include", "恢复管控"),
    });
  } else {
    controls.push({
      label: props.probePending ? "正在探活" : "探活测试",
      icon: props.probePending ? <LoaderCircle className="animate-spin" /> : <Activity />,
      disabled: props.pending || props.probePending || manualControlled,
      onClick: props.onProbe,
    });
    if (!policyStopped) {
      controls.push({
        label: resumable ? "恢复调度" : "暂停调度",
        icon: resumable ? <Play /> : <Pause />,
        tone: resumable ? "primary" : undefined,
        disabled: props.pending || manualControlled,
        onClick: () =>
          props.onControl(
            resumable ? "resume" : "pause",
            resumable ? "恢复调度" : "暂停调度",
            resumable
              ? undefined
              : `暂停“${props.account.name}”后，该账号将停止接收流量，但仍会继续监控和计分。`,
          ),
      });
    }
    controls.push({
      label: fused ? "解除熔断" : "手动熔断（停止调度）",
      icon: fused ? <RefreshCw /> : <Ban />,
      tone: fused ? "primary" : "danger",
      prominent: !fused,
      disabled: props.pending || paused || manualControlled,
      onClick: () =>
        props.onControl(
          fused ? "recover" : "fuse",
          fused ? "解除熔断" : "手动熔断",
          fused
            ? `解除“${props.account.name}”的熔断后，该账号可重新接收流量，后续仍受调度策略约束。`
            : `手动熔断“${props.account.name}”后，该账号会立即停止接收流量，并持续保持熔断状态，直到手动解除。`,
        ),
    });
  }
  return {
    controls,
    maintenance: [
      {
        label: "同步账号倍率",
        icon: <RefreshCw />,
        disabled: props.pending,
        onClick: props.onRateSync,
      },
      {
        label: manualControlled ? "调整人工优先位" : "设置人工优先位",
        icon: <Pin />,
        disabled: props.pending,
        onClick: props.onManualPriority,
      },
      {
        label: "查看并编辑账号",
        icon: <Pencil />,
        disabled: props.pending || manualControlled,
        onClick: props.onEdit,
      },
      {
        label: "删除账号及上游 Key",
        icon: <Trash2 />,
        tone: "danger",
        disabled: props.pending,
        onClick: props.onDelete,
      },
    ],
  };
}

export function AccountOperationControls(props: AccountOperationProps): ReactElement {
  const actions = accountActions(props);
  const allActions = [...actions.controls, ...actions.maintenance];
  const visibleCount = Math.min(5, allActions.length - 1);
  const visibleActions = allActions.slice(0, visibleCount);
  const moreActions = allActions.slice(visibleCount);
  return (
    <div
      role="group"
      aria-label="账号操作"
      className="ml-auto grid w-fit shrink-0 grid-cols-3 items-center gap-1"
    >
      {visibleActions.map((action) => (
        <TableActionButton
          key={action.label}
          label={action.label}
          tone={action.tone}
          disabled={action.disabled}
          onClick={action.onClick}
          className={action.prominent ? prominentDangerActionClassName : undefined}
        >
          {action.icon}
        </TableActionButton>
      ))}
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              type="button"
              variant="outline"
              size="icon-sm"
              className="col-start-3 row-start-2 data-popup-open:bg-muted"
              aria-label="更多账号操作"
              disabled={props.pending}
            />
          }
        >
          <MoreHorizontal className="size-4" aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-48">
          {moreActions.map((action) => (
            <DropdownMenuItem
              key={action.label}
              className={
                action.tone === "danger" ? "text-destructive focus:text-destructive" : undefined
              }
              disabled={action.disabled}
              onClick={action.onClick}
            >
              {action.icon}
              {action.label}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
