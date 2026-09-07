import { useRef, useState } from "react";
import type { ReactElement } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { AccountOperationControls, type AccountOperationProps } from "./account-operation-controls";
import { AccountStateCell } from "./account-pool-cells";

export function AccountOperationButtons(props: AccountOperationProps): ReactElement {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const titleRef = useRef<HTMLHeadingElement>(null);
  function closeAndRun(action: () => void): void {
    setOpen(false);
    action();
  }
  return (
    <div className="ml-auto flex shrink-0 items-center justify-end gap-1">
      <AccountOperationControls {...props} />
      <Dialog open={open} onOpenChange={setOpen}>
        <Button
          ref={triggerRef}
          type="button"
          variant="outline"
          size="sm"
          aria-haspopup="dialog"
          aria-expanded={open}
          onClick={() => setOpen(true)}
        >
          状态与处置
        </Button>
        <DialogContent
          width="progress"
          height="adaptive"
          initialFocus={titleRef}
          finalFocus={triggerRef}
          className="grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden"
        >
          <DialogHeader>
            <DialogTitle ref={titleRef} tabIndex={-1} className="outline-none">
              状态与处置
            </DialogTitle>
            <DialogDescription className="break-words [overflow-wrap:anywhere]">
              {props.account.name}（#{props.account.id}）
            </DialogDescription>
          </DialogHeader>
          <DialogBody role="region" aria-label="账号状态详情" className="grid gap-4 text-left">
            <AccountStateCell account={props.account} expanded />
            {props.account.manual_priority != null ? (
              <p className="text-sm text-muted-foreground">
                账号处于人工优先位，自动探活与调度处置已禁用。可在更多账号操作中调整人工优先位。
              </p>
            ) : null}
          </DialogBody>
          <DialogFooter>
            <div role="group" aria-label="账号常用处置" className="min-w-0">
              <AccountOperationControls
                {...props}
                expanded
                onProbe={() => closeAndRun(props.onProbe)}
                onControl={(action, label, description) =>
                  closeAndRun(() => props.onControl(action, label, description))
                }
                onRateSync={() => closeAndRun(props.onRateSync)}
                onManualPriority={() => closeAndRun(props.onManualPriority)}
                onEdit={() => closeAndRun(props.onEdit)}
                onDelete={() => closeAndRun(props.onDelete)}
              />
            </div>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
