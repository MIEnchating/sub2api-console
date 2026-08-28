import type { MouseEventHandler, ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

type TableActionTone = "default" | "primary" | "danger";

export function TableActionButton(props: {
  label: string;
  children: ReactNode;
  onClick?: MouseEventHandler<HTMLButtonElement>;
  disabled?: boolean;
  tone?: TableActionTone;
  className?: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            aria-label={props.label}
            disabled={props.disabled}
            onClick={props.onClick}
            className={cn(
              props.tone === "primary" && "text-primary hover:text-primary",
              props.tone === "danger" &&
                "text-destructive hover:bg-destructive/10 hover:text-destructive",
              props.className,
            )}
          />
        }
      >
        {props.children}
      </TooltipTrigger>
      <TooltipContent>{props.label}</TooltipContent>
    </Tooltip>
  );
}
