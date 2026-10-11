import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export type ConfirmActionDialogProps = {
  open: boolean;
  title: string;
  description: string;
  confirmLabel: string;
  pendingLabel?: string;
  pending?: boolean;
  pendingContent?: ReactNode;
  confirmDisabled?: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function ConfirmActionDialogContent(props: ConfirmActionDialogProps) {
  return (
    <div data-slot="confirm-action-dialog" className="contents">
      <DialogHeader>
        <DialogTitle>{props.title}</DialogTitle>
        <DialogDescription>{props.description}</DialogDescription>
      </DialogHeader>
      {props.pending && props.pendingContent}
      <DialogFooter>
        <Button
          variant="outline"
          disabled={props.pending}
          onClick={() => props.onOpenChange(false)}
        >
          取消
        </Button>
        <Button
          variant="destructive"
          disabled={props.pending || props.confirmDisabled}
          onClick={props.onConfirm}
        >
          {props.pending ? (props.pendingLabel ?? "处理中…") : props.confirmLabel}
        </Button>
      </DialogFooter>
    </div>
  );
}

export function ConfirmActionDialog(props: ConfirmActionDialogProps) {
  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!props.pending) props.onOpenChange(open);
      }}
    >
      <DialogContent>
        <ConfirmActionDialogContent {...props} />
      </DialogContent>
    </Dialog>
  );
}
