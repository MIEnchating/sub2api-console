import type { ReactNode } from "react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export const accountDetailDialogLayout = {
  width: "progress",
  content: "grid gap-3 overflow-visible",
  body: "min-w-0 overflow-visible pr-1 text-sm",
} as const;

export function AccountDetailDialog(props: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  accountName: string;
  accountId: string;
  children: ReactNode;
}) {
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent
        width={accountDetailDialogLayout.width}
        height="content"
        className={accountDetailDialogLayout.content}
      >
        <DialogHeader>
          <DialogTitle>账号设置</DialogTitle>
          <DialogDescription>
            {props.accountName} · 稳定账号 ID {props.accountId}
          </DialogDescription>
        </DialogHeader>
        {props.children}
      </DialogContent>
    </Dialog>
  );
}
