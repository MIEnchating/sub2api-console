import type { ReactElement } from "react";
import { Ban, RefreshCw } from "lucide-react";
import type { AccountStatus } from "@/api";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  detectionControlLabels,
  detectionControlState,
  type DetectionControlAction,
} from "../lib/account-control";
import { useDetectionAccountControl } from "./detection-account-control-provider";

export function DetectionAccountControls(props: { account: AccountStatus }): ReactElement | null {
  const control = useDetectionAccountControl();
  if (!control) return null;
  const state = detectionControlState(props.account);
  const actions: Array<{ action: DetectionControlAction; reason: string }> = [
    { action: "fuse", reason: state.fuseReason },
    { action: state.recovery, reason: state.recoveryReason },
  ];
  return (
    <>
      {actions.map((item) => (
        <Tooltip key={item.action}>
          <TooltipTrigger render={<span className="inline-flex" />}>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label={detectionControlLabels[item.action]}
              aria-busy={control.pending}
              disabled={control.pending || Boolean(item.reason)}
              onClick={() => control.select({ accountID: props.account.id, action: item.action })}
            >
              {item.action === "fuse" ? (
                <Ban aria-hidden="true" />
              ) : (
                <RefreshCw aria-hidden="true" />
              )}
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {control.pending
              ? "账号处置执行中"
              : item.reason || detectionControlLabels[item.action]}
          </TooltipContent>
        </Tooltip>
      ))}
    </>
  );
}
