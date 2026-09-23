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

export function AnimationScheduleConfirmation(props: {
  open: boolean;
  enabled: boolean;
  description: string;
  pending: boolean;
  targets: { accountID: string; accountName: string }[];
  onClose: () => void;
  onConfirm: () => void;
}): ReactElement {
  let confirmLabel = props.enabled ? "确认保存并开启" : "确认关闭";
  if (props.pending) confirmLabel = "正在保存…";
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open && !props.pending) props.onClose();
      }}
    >
      <DialogContent showCloseButton={!props.pending}>
        <DialogHeader>
          <DialogTitle>{props.enabled ? "确认开启自动检测" : "确认关闭自动检测"}</DialogTitle>
          <DialogDescription>
            {props.description}。已选择 {props.targets.length} 个账号。
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          <ul
            aria-label="自动检测影响账号"
            className="max-h-48 space-y-1 overflow-y-auto text-sm [overflow-wrap:anywhere]"
          >
            {props.targets.map((target) => (
              <li key={target.accountID}>
                {target.accountName}（ID {target.accountID}）
              </li>
            ))}
          </ul>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" disabled={props.pending} onClick={props.onClose}>
            取消
          </Button>
          <Button disabled={props.pending} onClick={props.onConfirm}>
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
