import type { MouseEventHandler, ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

type TableActionTone = "default" | "primary" | "danger";

export function TableActionButton(props: {
  label: string;
  ariaLabel?: string;
  children: ReactNode;
  onClick?: MouseEventHandler<HTMLButtonElement>;
  disabled?: boolean;
  tone?: TableActionTone;
  className?: string;
}) {
  let variant: "outline" | "default" | "destructive" = "outline";
  if (props.tone === "primary") variant = "default";
  if (props.tone === "danger") variant = "destructive";
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            type="button"
            variant={variant}
            size="icon"
            aria-label={props.ariaLabel ?? props.label}
            disabled={props.disabled}
            onClick={props.onClick}
            className={props.className}
          />
        }
      >
        {props.children}
      </TooltipTrigger>
      <TooltipContent>{props.label}</TooltipContent>
    </Tooltip>
  );
}
