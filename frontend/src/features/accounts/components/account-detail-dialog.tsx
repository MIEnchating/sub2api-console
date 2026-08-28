import type { ReactNode } from "react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export const accountDetailDialogLayout = {
  content:
    "grid max-h-[min(42rem,calc(100svh-2rem))] min-w-0 grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden sm:max-w-2xl",
  body: "min-h-0 overflow-x-hidden overflow-y-auto pr-1 text-sm",
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
      <DialogContent className={accountDetailDialogLayout.content}>
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
