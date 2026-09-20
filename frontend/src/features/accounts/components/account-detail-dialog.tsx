import type { ReactNode } from "react";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

export const accountDetailDialogLayout = {
  width: "progress",
  content: "grid h-[min(40rem,calc(100svh-2rem))] w-[min(48rem,calc(100vw-2rem))] gap-5",
  body: "min-w-0 pr-1 text-sm",
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
          <DialogDescription className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
            <Tooltip>
              <TooltipTrigger
                render={
                  <span
                    tabIndex={0}
                    className="line-clamp-2 min-w-0 rounded-sm break-all outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  />
                }
              >
                {props.accountName}
              </TooltipTrigger>
              <TooltipContent role="tooltip">{props.accountName}</TooltipContent>
            </Tooltip>
            <span className="bg-muted text-muted-foreground shrink-0 rounded px-1.5 py-0.5 text-xs tabular-nums">
              账号 ID {props.accountId}
            </span>
          </DialogDescription>
        </DialogHeader>
        {props.children}
      </DialogContent>
    </Dialog>
  );
}
